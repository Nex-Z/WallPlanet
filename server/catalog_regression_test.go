package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestCoverReferences(t *testing.T) {
	for _, s := range []string{"", "/media/photo.jpg", "https://images.example.com/a.jpg?size=large", "http://images.example.com/a.jpg"} {
		if !validCoverRef(s) {
			t.Fatal(s)
		}
	}
	for _, s := range []string{"javascript:alert(1)", "data:text/html,test", "//example.com/a.jpg", "https://user:secret@example.com/a.jpg", "https://example.com/a\n"} {
		if validCoverRef(s) {
			t.Fatal(s)
		}
	}
}

func TestIntegrationTopicCoverAndFeaturedFilter(t *testing.T) {
	a := integrationApp(t)
	ctx := context.Background()
	stored, e := a.storage.Save(ctx, testPNG(t, 10, 10))
	if e != nil {
		t.Fatal(e)
	}
	a.fetchOverride = func(context.Context, string) (StoredImage, error) { return stored, nil }
	_, e = a.db.Exec(ctx, `INSERT INTO entities(id,kind,data) VALUES('new-topic','topic','{"name":"New topic","cover":""}')`)
	if e != nil {
		t.Fatal(e)
	}
	raw := sampleTweet()
	if _, e = a.importTweet(ctx, Source{MinShort: 1, MinLong: 1, TopicIDs: []string{"new-topic"}, Tags: []string{}}, raw); e != nil {
		t.Fatal(e)
	}
	raw["id"] = "another"
	if _, e = a.importTweet(ctx, Source{MinShort: 1, MinLong: 1, TopicIDs: []string{}, Tags: []string{}}, raw); e != nil {
		t.Fatal(e)
	}
	_, e = a.db.Exec(ctx, "UPDATE wallpapers SET featured=true WHERE source_id='x:another'")
	if e != nil {
		t.Fatal(e)
	}
	// Set every work outside this topic as featured regardless of source key format.
	_, e = a.db.Exec(ctx, "UPDATE wallpapers SET featured=true WHERE NOT ('new-topic'=ANY(topic_ids))")
	if e != nil {
		t.Fatal(e)
	}
	r := request(t, a, testSession{}, "GET", "/api/v1/wallpapers?featured=prefer&topic=new-topic", nil)
	var page struct {
		Items []M `json:"items"`
	}
	if r.Code != 200 || json.Unmarshal(r.Body.Bytes(), &page) != nil || len(page.Items) != 1 {
		t.Fatal(r.Code, r.Body.String())
	}
	r = request(t, a, testSession{}, "GET", "/api/v1/entities/new-topic", nil)
	var entity M
	if r.Code != 200 || json.Unmarshal(r.Body.Bytes(), &entity) != nil || !strings.HasPrefix(str(entity, "cover"), "/media/") {
		t.Fatal(r.Body.String())
	}
	_, e = a.db.Exec(ctx, `UPDATE entities SET data=data||'{"cover":"https://images.example.com/custom.jpg"}'::jsonb WHERE id='new-topic'`)
	if e != nil {
		t.Fatal(e)
	}
	r = request(t, a, testSession{}, "GET", "/api/v1/entities/new-topic", nil)
	if json.Unmarshal(r.Body.Bytes(), &entity) != nil || str(entity, "cover") != "https://images.example.com/custom.jpg" {
		t.Fatal(r.Body.String())
	}
}
