package main

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func testPNG(t *testing.T, w, h int) []byte {
	t.Helper()
	im := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			im.Set(x, y, color.RGBA{uint8(x % 256), uint8(y % 256), 100, 255})
		}
	}
	var b bytes.Buffer
	if e := png.Encode(&b, im); e != nil {
		t.Fatal(e)
	}
	return b.Bytes()
}
func TestPasswordsAndEncryption(t *testing.T) {
	h := hashPassword("long-password-123")
	if !verifyPassword("long-password-123", h) || verifyPassword("wrong-password", h) || verifyPassword("anything", "invalid") {
		t.Fatal("password verification failed")
	}
	key := bytes.Repeat([]byte{7}, 32)
	a, e := encrypt(key, "private-token")
	if e != nil {
		t.Fatal(e)
	}
	b, _ := encrypt(key, "private-token")
	if a == b {
		t.Fatal("encryption must use random nonce")
	}
	plain, e := decrypt(key, a)
	if e != nil || plain != "private-token" {
		t.Fatal("round trip failed")
	}
	if _, e = decrypt(bytes.Repeat([]byte{8}, 32), a); e == nil {
		t.Fatal("wrong key accepted")
	}
	if _, e = decrypt(key, "invalid"); e == nil {
		t.Fatal("malformed ciphertext accepted")
	}
}
func TestStorageValidationAndDedup(t *testing.T) {
	s := &LocalStorage{Root: t.TempDir()}
	data := testPNG(t, 1000, 750)
	a, e := s.Save(context.Background(), data)
	if e != nil {
		t.Fatal(e)
	}
	b, e := s.Save(context.Background(), data)
	if e != nil || a.Key != b.Key || a.Width != 1000 || a.Height != 750 {
		t.Fatal("bad image metadata/dedup")
	}
	if _, e = s.Save(context.Background(), []byte("not an image")); e == nil {
		t.Fatal("invalid image accepted")
	}
	for _, key := range []string{"../secret", "a/b", "a\\b", "C:secret"} {
		if _, e = s.Path(key); e == nil {
			t.Fatal("traversal accepted")
		}
	}
	for _, u := range []string{"http://pbs.twimg.com/a.jpg", "https://127.0.0.1/a", "https://pbs.twimg.com.evil.test/a", "https://pbs.twimg.com:444/a", "https://user@pbs.twimg.com/a"} {
		if allowedMediaURL(u) == nil {
			t.Fatalf("unsafe URL accepted: %s", u)
		}
	}
	if allowedMediaURL("https://pbs.twimg.com/media/a.jpg") != nil {
		t.Fatal("valid image URL rejected")
	}
}
func sampleTweet() M {
	return M{"id": "12345", "full_text": "雪山与湖泊 https://t.co/abc", "url": "https://x.com/artist/status/12345", "created_at": "Tue Sep 08 02:00:00 +0000 2026", "favorite_count": float64(123), "author": M{"id_str": "a1", "screen_name": "artist", "name": "作者", "profile_image_url_https": "https://pbs.twimg.com/profile_images/a.jpg"}, "entities": M{"hashtags": []any{M{"text": "自然"}}}, "extended_entities": M{"media": []any{M{"id_str": "photo1", "type": "photo", "media_url_https": "https://pbs.twimg.com/media/one.jpg"}, M{"id_str": "photo2", "type": "photo", "media_url_https": "https://pbs.twimg.com/media/two.jpg"}, M{"id_str": "photo1", "type": "photo", "media_url_https": "https://pbs.twimg.com/media/one.jpg"}, M{"id_str": "video1", "type": "video", "media_url_https": "https://pbs.twimg.com/media/video.jpg"}}}}
}
func TestActorInputAndMultiImageParser(t *testing.T) {
	now := time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)
	s := Source{Kind: "account", Query: "@artist", MaxItems: 100}
	input := buildInput(s, now)
	if input["searchTerms"].([]string)[0] != "from:artist filter:images since:2026-09-01" || input["maxItems"] != 100 {
		t.Fatalf("unexpected input: %v", input)
	}
	s.Kind = "keyword"
	s.Query = "#wallpaper"
	s.Watermark = &now
	if !strings.Contains(buildInput(s, now)["searchTerms"].([]string)[0], "since:2026-09-07") {
		t.Fatal("watermark overlap missing")
	}
	tw, e := parseTweet(sampleTweet())
	if e != nil || len(tw.Images) != 2 || tw.Title != "雪山与湖泊" || tw.AuthorID != "x-author-a1" || len(tw.Tags) != 1 {
		t.Fatalf("invalid parse: %+v %v", tw, e)
	}
	bad := sampleTweet()
	bad["url"] = "javascript:alert(1)"
	tw, _ = parseTweet(bad)
	if !strings.HasPrefix(tw.URL, "https://x.com/") {
		t.Fatal("unsafe source URL")
	}
	if _, e = parseTweet(M{"error": "no result"}); e == nil {
		t.Fatal("invalid dataset row accepted")
	}
}
func TestApifyHTTPAndRunVerification(t *testing.T) {
	now := time.Now().UTC()
	input := M{"maxItems": float64(100), "onlyImage": true}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer secret" {
			t.Error("missing bearer token")
		}
		switch r.URL.Path {
		case "/actor-runs/run1":
			w.Write([]byte(`{"data":{"id":"run1","status":"SUCCEEDED","defaultDatasetId":"ds","defaultKeyValueStoreId":"store"}}`))
		case "/key-value-stores/store/records/INPUT":
			w.Write(encode(input))
		case "/limited":
			w.WriteHeader(429)
			w.Write([]byte(`{"error":"must not expose secret"}`))
		default:
			w.WriteHeader(500)
		}
	}))
	defer srv.Close()
	old := apifyBase
	apifyBase = srv.URL
	defer func() { apifyBase = old }()
	a := &App{http: srv.Client()}
	run, e := a.getRun(context.Background(), "secret", "run1")
	if e != nil || run.DatasetID != "ds" {
		t.Fatal("run API failed")
	}
	run.StartedAt = now
	if e = a.verifyRunInput(context.Background(), "secret", run, encode(input), &now); e != nil {
		t.Fatal(e)
	}
	if e = a.verifyRunInput(context.Background(), "secret", run, encode(M{"maxItems": 50}), &now); e == nil {
		t.Fatal("mismatched run accepted")
	}
	e = a.apify(context.Background(), "secret", "GET", "/limited", nil, nil)
	if e == nil || e.Error() != "Apify HTTP 429" {
		t.Fatal("rate limit handling failed")
	}
}
func TestCursorAndLimiter(t *testing.T) {
	if _, e := parseCursor("malformed!"); e == nil {
		t.Fatal("bad cursor accepted")
	}
	if _, e := parseCursor(""); e != nil {
		t.Fatal(e)
	}
	l := NewLimiter()
	if !l.Allow("one", 1, time.Minute) || l.Allow("one", 1, time.Minute) || !l.Allow("two", 1, time.Minute) {
		t.Fatal("rate limit partitioning failed")
	}
}
