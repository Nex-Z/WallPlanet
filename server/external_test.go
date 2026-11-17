package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestIntegrationXRWGroupedAlbums(t *testing.T) {
	a := integrationApp(t)
	albums := []xrwAlbum{}
	for i, title := range []string{
		"Cosplay-过期米线线喵-狐狸小姐-Set01-04-06", "过期米线线喵-狐狸小姐55P-04-06",
		"Cosplay-过期米线线喵-金丝雀-04-06", "小律-Ritsu-Platinum-06-01",
	} {
		id := fmt.Sprintf("000%d-aaaaaaaaaa", i)
		payload := fmt.Sprintf(`{"id":%q,"title":%q,"photos":[{"id":1,"url":"https://telegra.ph/file/common.jpg"},{"id":2,"url":"https://telegra.ph/file/%d.jpg"}]}`, id, title, i)
		album, err := decodeXRWAlbum(strings.NewReader(payload), id)
		if err != nil {
			t.Fatal(err)
		}
		albums = append(albums, album)
	}
	if err := writeXRWAlbums(context.Background(), a.db, albums, time.Now()); err != nil {
		t.Fatal(err)
	}
	firstArchive := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	if _, err := a.db.Exec(context.Background(), `UPDATE wallpapers SET created_at=$1 WHERE id=$2`, firstArchive, "xrw-album-"+albums[1].ID); err != nil {
		t.Fatal(err)
	}
	if err := classifyXRW(context.Background(), a.db); err != nil {
		t.Fatal(err)
	}
	dir := request(t, a, testSession{}, "GET", "/api/v1/models", nil)
	var models struct {
		Items []struct {
			Name  string
			Count int
			Cover string
		}
	}
	if err := json.Unmarshal(dir.Body.Bytes(), &models); err != nil || dir.Code != 200 || len(models.Items) != 2 || models.Items[0].Name != "过期米线线喵" || models.Items[0].Count != 2 || models.Items[0].Cover == "" {
		t.Fatal("incorrect model count, cover or sort order", dir.Code, dir.Body.String())
	}
	for _, album := range albums[:2] {
		detail := request(t, a, testSession{}, "GET", "/api/v1/wallpapers/xrw-album-"+album.ID, nil)
		var item struct {
			Title      string
			Images     []struct{ URL string }
			ArchivedAt time.Time
		}
		if err := json.Unmarshal(detail.Body.Bytes(), &item); err != nil || detail.Code != 200 || item.Title != "狐狸小姐" || len(item.Images) != 3 {
			t.Fatal("merged detail did not preserve both sources and deduplicate common photos", detail.Code, detail.Body.String())
		}
		if !item.ArchivedAt.Equal(firstArchive) {
			t.Fatal("group did not retain the earliest archive time", item.ArchivedAt)
		}
	}
}

func TestIntegrationArchiveSortPagination(t *testing.T) {
	a := integrationApp(t)
	ctx := context.Background()
	for _, row := range []struct{ id, created string }{
		{"a", "2026-01-03"}, {"b", "2026-01-03"}, {"c", "2026-01-01"},
	} {
		_, err := a.db.Exec(ctx, `INSERT INTO wallpapers(id,source_id,channel_id,title,published_at,created_at)
			VALUES($1,$1,'x',$1,'2025-01-01',$2::text::timestamptz)`, row.id, row.created)
		if err != nil {
			t.Fatal(err)
		}
		_, err = a.db.Exec(ctx, `INSERT INTO media(id,wallpaper_id,source_key,storage_key,thumbnail_key,width,height,bytes,mime,position)
			VALUES($1,$1,$1,'a.jpg','a-thumb.jpg',720,1280,5,'image/jpeg',0)`, row.id)
		if err != nil {
			t.Fatal(err)
		}
	}
	for order, expected := range map[string][]string{"newest": {"a", "b", "c"}, "oldest": {"c", "a", "b"}} {
		cursor := ""
		for _, want := range expected {
			r := request(t, a, testSession{}, "GET", "/api/v1/wallpapers?limit=1&sort="+order+"&cursor="+url.QueryEscape(cursor), nil)
			var page struct {
				Items      []struct{ ID string }
				NextCursor string
			}
			if err := json.Unmarshal(r.Body.Bytes(), &page); err != nil || r.Code != 200 || len(page.Items) != 1 || page.Items[0].ID != want {
				t.Fatalf("%s expected %s: %s", order, want, r.Body.String())
			}
			cursor = page.NextCursor
		}
		if cursor != "" {
			t.Fatal("last page had a cursor", order)
		}
	}
}

func TestXRWAlbumStem(t *testing.T) {
	for _, tc := range []struct{ title, model, stem string }{
		{"过期米线线喵-狐狸小姐55P-04-06", "过期米线线喵", "狐狸小姐"},
		{"过期米线线喵-狐狸小姐-04-06", "过期米线线喵", "狐狸小姐"},
		{"Cosplay-蠢沫沫Chunmomo-白精灵-婚纱-Set02-11-13", "蠢沫沫", "白精灵 婚纱"},
		{"蠢沫沫---白精灵-原设-04-25", "蠢沫沫", "白精灵 原设"},
		{"蠢沫沫---婚纱-04-22", "蠢沫沫", ""},
		{"菌烨tako---NO010-八重神子COS12P-236M-07-24", "菌烨tako", "NO010 八重神子COS"},
	} {
		if got := xrwAlbumStem(tc.title, tc.model); got != tc.stem {
			t.Errorf("%q: got %q, want %q", tc.title, got, tc.stem)
		}
	}
}

func TestXRWModelCandidate(t *testing.T) {
	for _, tc := range []struct {
		title, name string
		known       bool
	}{
		{"Cosplay-蠢沫沫Chunmomo-Set01-11-13", "蠢沫沫", true},
		{"000-菌烨tako---原神-甘雨-08-15", "菌烨tako", true},
		{"XiuRen秀人网-NO6278-陆萱萱LuXuanXuan-11-13", "陆萱萱LuXuanXuan", true},
		{"Son-Yeeun-손예은-LOOZY-Set02-11-14", "Son Yeeun 손예은", true},
		{"小律-Ritsu-Platinum-明日方舟-Arknights-06-01", "小律", true},
		{"柒柒不可爱-Mewo-02-17", "柒柒要乖哦", false},
		{"微密圈-模特合集-11-12", "", false},
	} {
		if name, known := xrwModelCandidate(tc.title); name != tc.name || known != tc.known {
			t.Errorf("%q: got (%q, %t), want (%q, %t)", tc.title, name, known, tc.name, tc.known)
		}
	}
}

func TestValidXRWImage(t *testing.T) {
	for _, raw := range []string{
		"http://telegra.ph/file/photo.jpg",
		"https://telegra.ph.example.com/file/photo.jpg",
		"https://telegra.ph:8443/file/photo.jpg",
		"https://user@telegra.ph/file/photo.jpg",
		"https://telegra.ph/file/%2e%2e/photo.jpg",
		"https://telegra.ph/file/photo.svg",
		"https://telegra.ph/file/photo.jpg?url=http://localhost/",
	} {
		if validXRWImage(raw) {
			t.Errorf("unexpectedly accepted %q", raw)
		}
	}
	if !validXRWImage("https://telegra.ph/file/photo.jpg") {
		t.Fatal("a valid upstream image was rejected")
	}
}
