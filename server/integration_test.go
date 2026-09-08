package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func integrationApp(t *testing.T) *App {
	t.Helper()
	gin.SetMode(gin.TestMode)
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to test against an isolated PostgreSQL schema")
	}
	ctx := context.Background()
	admin, e := pgxpool.New(ctx, dsn)
	if e != nil {
		t.Fatal("invalid test database configuration")
	}
	schema := "test_" + id()
	if _, e = admin.Exec(ctx, "CREATE SCHEMA "+pgx.Identifier{schema}.Sanitize()); e != nil {
		admin.Close()
		t.Fatal("cannot create isolated test schema")
	}
	cfg, e := pgxpool.ParseConfig(dsn)
	if e != nil {
		t.Fatal(e)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	db, e := pgxpool.NewWithConfig(ctx, cfg)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		db.Close()
		if !strings.HasPrefix(schema, "test_") {
			panic("unsafe test schema")
		}
		_, e = admin.Exec(ctx, "DROP SCHEMA "+pgx.Identifier{schema}.Sanitize()+" CASCADE")
		if e != nil {
			t.Error("failed to remove isolated test schema")
		}
		admin.Close()
	})
	if e = migrate(ctx, db); e != nil {
		t.Fatal(e)
	}
	if e = migrate(ctx, db); e != nil {
		t.Fatal("migration is not idempotent", e)
	}
	return &App{db: db, key: bytes.Repeat([]byte{1}, 32), origin: "http://localhost:5173", limiter: NewLimiter(), storage: &LocalStorage{Root: t.TempDir()}, http: &http.Client{Timeout: time.Second}}
}

type testSession struct {
	cookie *http.Cookie
	csrf   string
	uid    string
}

func request(t *testing.T, a *App, session testSession, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(encode(body))
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", a.origin)
	if session.cookie != nil {
		req.AddCookie(session.cookie)
	}
	if session.csrf != "" {
		req.Header.Set("X-CSRF-Token", session.csrf)
	}
	rec := httptest.NewRecorder()
	a.routes().ServeHTTP(rec, req)
	return rec
}
func register(t *testing.T, a *App, name string) testSession {
	t.Helper()
	r := request(t, a, testSession{}, "POST", "/api/v1/auth/register", M{"username": name, "password": "test-password-1234"})
	if r.Code != 200 {
		t.Fatal("register failed", r.Code, r.Body.String())
	}
	var b M
	json.Unmarshal(r.Body.Bytes(), &b)
	s := testSession{cookie: r.Result().Cookies()[0], csrf: b["csrf"].(string)}
	if e := a.db.QueryRow(context.Background(), "SELECT id FROM users WHERE username=$1", name).Scan(&s.uid); e != nil {
		t.Fatal(e)
	}
	return s
}
func TestIntegrationUserLifecycleAndRanks(t *testing.T) {
	a := integrationApp(t)
	ctx := context.Background()
	s := register(t, a, "test-person")
	if r := request(t, a, s, "GET", "/api/v1/admin/settings", nil); r.Code != 403 {
		t.Fatal("regular user accessed admin")
	}
	bad := s
	bad.csrf = "wrong"
	if r := request(t, a, bad, "PATCH", "/api/v1/me/profile", M{}); r.Code != 403 {
		t.Fatal("CSRF bypass")
	}
	stored, e := a.storage.Save(ctx, testPNG(t, 1280, 800))
	if e != nil {
		t.Fatal(e)
	}
	a.fetchOverride = func(context.Context, string) (StoredImage, error) { return stored, nil }
	src := Source{ID: "test-source", MinShort: 720, MinLong: 1280, TopicIDs: []string{"nature"}, Tags: []string{"测试"}}
	created, e := a.importTweet(ctx, src, sampleTweet())
	if e != nil || !created {
		t.Fatal("import failed", e)
	}
	created, e = a.importTweet(ctx, src, sampleTweet())
	if e != nil || created {
		t.Fatal("duplicate import created a second work")
	}
	var wid, mid string
	var count int
	if e = a.db.QueryRow(ctx, "SELECT id FROM wallpapers").Scan(&wid); e != nil {
		t.Fatal(e)
	}
	a.db.QueryRow(ctx, "SELECT count(*) FROM media").Scan(&count)
	if count != 2 {
		t.Fatal("multi-photo dedup failed", count)
	}
	a.db.QueryRow(ctx, "SELECT id FROM media LIMIT 1").Scan(&mid)
	for _, kind := range []string{"like", "favorite"} {
		for i := 0; i < 2; i++ {
			r := request(t, a, s, "PUT", "/api/v1/wallpapers/"+wid+"/interactions/"+kind, nil)
			if r.Code != 200 {
				t.Fatal(r.Body.String())
			}
		}
	}
	a.db.QueryRow(ctx, "SELECT count(*) FROM interactions").Scan(&count)
	if count != 2 {
		t.Fatal("interactions not idempotent")
	}
	for i := 0; i < 2; i++ {
		r := request(t, a, s, "POST", "/api/v1/wallpapers/"+wid+"/download/"+mid, nil)
		if r.Code != 200 || r.Body.Len() < 1000 {
			t.Fatal("original download failed")
		}
	}
	if e = a.refreshRanks(ctx); e != nil {
		t.Fatal(e)
	}
	var score int
	if e = a.db.QueryRow(ctx, "SELECT score FROM rank_snapshots WHERE period='7d' AND wallpaper_id=$1", wid).Scan(&score); e != nil || score != 7 {
		t.Fatal("download ranking dedup failed", score, e)
	}
	for i := 0; i < 2; i++ {
		if r := request(t, a, s, "PUT", "/api/v1/entities/nature/subscription", nil); r.Code != 200 {
			t.Fatal("subscribe failed")
		}
	}
	for _, path := range []string{"/api/v1/wallpapers?scope=subscriptions", "/api/v1/wallpapers?scope=favorites", "/api/v1/wallpapers?scope=downloads", "/api/v1/wallpapers?q=雪山", "/api/v1/wallpapers?tag=自然"} {
		r := request(t, a, s, "GET", path, nil)
		if r.Code != 200 || !strings.Contains(r.Body.String(), wid) {
			t.Fatal("list filtering failed", path, r.Body.String())
		}
	}
	req := httptest.NewRequest("PUT", "/api/v1/entities/nature/subscription", nil)
	req.Header.Set("Origin", "https://evil.example")
	req.AddCookie(s.cookie)
	req.Header.Set("X-CSRF-Token", s.csrf)
	rec := httptest.NewRecorder()
	a.routes().ServeHTTP(rec, req)
	if rec.Code != 403 {
		t.Fatal("cross-origin mutation accepted")
	}
	if _, e = a.db.Exec(ctx, "UPDATE wallpapers SET status='hidden' WHERE id=$1", wid); e != nil {
		t.Fatal(e)
	}
	for _, path := range []string{"/api/v1/wallpapers/" + wid, "/media/" + stored.Key} {
		if r := request(t, a, s, "GET", path, nil); r.Code != 404 {
			t.Fatal("hidden content exposed", path)
		}
	}
	admin := register(t, a, "administrator")
	a.db.Exec(ctx, "UPDATE users SET role='admin' WHERE id=$1", admin.uid)
	if r := request(t, a, admin, "PUT", "/api/v1/admin/settings", M{"token": "apify-secret-never-return-this"}); r.Code != 200 {
		t.Fatal(r.Body.String())
	}
	r := request(t, a, admin, "GET", "/api/v1/admin/settings", nil)
	if r.Code != 200 || strings.Contains(r.Body.String(), "never-return") {
		t.Fatal("secret exposed")
	}
	token, e := a.apifyToken(ctx)
	if e != nil || token != "apify-secret-never-return-this" {
		t.Fatal("stored token decryption failed")
	}
	a.db.Exec(ctx, "UPDATE users SET disabled=true WHERE id=$1", s.uid)
	if r = request(t, a, s, "PATCH", "/api/v1/me/profile", M{}); r.Code != 401 {
		t.Fatal("disabled session remains authorized")
	}
}
func TestIntegrationPartialImportPaginationAndRecovery(t *testing.T) {
	a := integrationApp(t)
	ctx := context.Background()
	src := Source{ID: "source", Name: "test", Kind: "account", Query: "artist", MaxItems: 100, MinShort: 1, MinLong: 1, TopicIDs: []string{"nature"}, Tags: []string{}}
	_, e := a.db.Exec(ctx, "INSERT INTO sources(id,name,kind,query,min_short,min_long) VALUES('source','test','account','artist',1,1)")
	if e != nil {
		t.Fatal(e)
	}
	stored, e := a.storage.Save(ctx, testPNG(t, 50, 40))
	if e != nil {
		t.Fatal(e)
	}
	badImage := true
	a.fetchOverride = func(_ context.Context, u string) (StoredImage, error) {
		if badImage && strings.Contains(u, "two") {
			return StoredImage{}, errors.New("temporary media failure")
		}
		return stored, nil
	}
	var offsets []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/datasets/") {
			offset := r.URL.Query().Get("offset")
			offsets = append(offsets, offset)
			if offset == "0" {
				items := make([]M, 50)
				for i := range items {
					items[i] = sampleTweet()
				}
				w.Write(encode(items))
			} else {
				w.Write([]byte(`[]`))
			}
			return
		}
		w.WriteHeader(500)
	}))
	defer srv.Close()
	old := apifyBase
	apifyBase = srv.URL
	defer func() { apifyBase = old }()
	a.http = srv.Client()
	started := time.Now().UTC()
	_, e = a.db.Exec(ctx, "INSERT INTO jobs(id,source_id,status,dataset_id,started_at) VALUES('job','source','importing','dataset',$1)", started)
	if e != nil {
		t.Fatal(e)
	}
	if e = a.importDatasetPage(ctx, "token", "job", "dataset", src, 0, &started); e != nil {
		t.Fatal(e)
	}
	var offset, count int
	var status string
	a.db.QueryRow(ctx, "SELECT dataset_offset FROM jobs WHERE id='job'").Scan(&offset)
	if offset != 50 {
		t.Fatal("pagination not persisted", offset)
	}
	a.db.QueryRow(ctx, "SELECT count(*) FROM wallpapers").Scan(&count)
	if count != 1 {
		t.Fatal("dataset duplicates created extra works", count)
	}
	a.db.QueryRow(ctx, "SELECT count(*) FROM media").Scan(&count)
	if count != 1 {
		t.Fatal("partial success not retained")
	}
	if e = a.importDatasetPage(ctx, "token", "job", "dataset", src, offset, &started); e != nil {
		t.Fatal(e)
	}
	a.db.QueryRow(ctx, "SELECT status FROM jobs WHERE id='job'").Scan(&status)
	if status != "partial" {
		t.Fatal("missing partial status", status)
	}
	badImage = false
	if e = a.retryItems(ctx, "job", src); e != nil {
		t.Fatal(e)
	}
	if e = a.importDatasetPage(ctx, "token", "job", "dataset", src, 50, &started); e != nil {
		t.Fatal(e)
	}
	a.db.QueryRow(ctx, "SELECT status FROM jobs WHERE id='job'").Scan(&status)
	a.db.QueryRow(ctx, "SELECT count(*) FROM media").Scan(&count)
	if status != "succeeded" || count != 2 {
		t.Fatal("retry failed", status, count)
	}
	if len(offsets) != 3 || offsets[1] != "50" {
		t.Fatal("dataset offset resume failed", offsets)
	}
	c1, _ := a.db.Acquire(ctx)
	defer c1.Release()
	c2, _ := a.db.Acquire(ctx)
	defer c2.Release()
	var locked bool
	if e = c1.QueryRow(ctx, "SELECT pg_try_advisory_lock(723019)").Scan(&locked); e != nil || !locked {
		t.Fatal("first worker lock failed")
	}
	if e = c2.QueryRow(ctx, "SELECT pg_try_advisory_lock(723019)").Scan(&locked); e != nil || locked {
		t.Fatal("concurrent worker obtained lock")
	}
	c1.Exec(ctx, "SELECT pg_advisory_unlock(723019)")
	_, e = a.db.Exec(ctx, "INSERT INTO jobs(id,source_id,status) VALUES('j1','source','queued')")
	if e != nil {
		t.Fatal(e)
	}
	_, e = a.db.Exec(ctx, "INSERT INTO jobs(id,source_id,status) VALUES('j2','source','retrying')")
	if e == nil {
		t.Fatal("duplicate source active job accepted")
	}
}
func TestIntegrationCursorPagination(t *testing.T) {
	a := integrationApp(t)
	ctx := context.Background()
	stored, e := a.storage.Save(ctx, testPNG(t, 10, 10))
	if e != nil {
		t.Fatal(e)
	}
	a.fetchOverride = func(context.Context, string) (StoredImage, error) { return stored, nil }
	for i := 0; i < 3; i++ {
		raw := sampleTweet()
		raw["id"] = fmt.Sprint(i)
		if _, e = a.importTweet(ctx, Source{MinShort: 1, MinLong: 1, Tags: []string{}, TopicIDs: []string{}}, raw); e != nil {
			t.Fatal(e)
		}
	}
	cursor := ""
	ids := map[string]bool{}
	for i := 0; i < 3; i++ {
		r := request(t, a, testSession{}, "GET", "/api/v1/wallpapers?limit=1&cursor="+cursor, nil)
		if r.Code != 200 {
			t.Fatal(r.Body.String())
		}
		var page struct {
			Items []M    `json:"items"`
			Next  string `json:"nextCursor"`
		}
		json.Unmarshal(r.Body.Bytes(), &page)
		if len(page.Items) != 1 {
			t.Fatal("unexpected page size")
		}
		wid := page.Items[0]["id"].(string)
		if ids[wid] {
			t.Fatal("duplicate cursor result")
		}
		ids[wid] = true
		cursor = page.Next
	}
	if cursor != "" {
		t.Fatal("last page returned cursor")
	}
}

func TestIntegrationWorkerRestartAndUncertainStart(t *testing.T) {
	a := integrationApp(t)
	ctx := context.Background()
	encrypted, _ := encrypt(a.key, "test-token")
	if _, e := a.db.Exec(ctx, "INSERT INTO settings(key,value) VALUES('apify_token',$1)", encode(M{"ciphertext": encrypted})); e != nil {
		t.Fatal(e)
	}
	if _, e := a.db.Exec(ctx, "INSERT INTO sources(id,name,kind,query,enabled) VALUES('source','test','accounts','artist other',true)"); e != nil {
		t.Fatal(e)
	}
	var input M
	var started time.Time
	posts := 0
	rateLimit := false
	completed := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "POST" && r.URL.Path == "/acts/"+actorPath+"/runs":
			posts++
			json.NewDecoder(r.Body).Decode(&input)
			started = time.Now().UTC()
			w.Write([]byte(`{"data":{"id":"remote-run","status":"RUNNING"}}`))
		case r.URL.Path == "/acts/"+actorPath+"/runs":
			w.Write(encode(M{"data": M{"items": []ActorRun{{ID: "remote-run", Status: "RUNNING", StoreID: "store", StartedAt: started}}}}))
		case r.URL.Path == "/key-value-stores/store/records/run-report":
			w.Write([]byte(`{"completionReason":"completed","failedSubtargets":0}`))
		case r.URL.Path == "/key-value-stores/store/records/INPUT":
			w.Write(encode(input))
		case r.URL.Path == "/actor-runs/remote-run":
			if rateLimit {
				w.WriteHeader(429)
				return
			}
			status := "RUNNING"
			if completed {
				status = "SUCCEEDED"
			}
			w.Write(encode(M{"data": ActorRun{ID: "remote-run", Status: status, DatasetID: "dataset", StoreID: "store", StartedAt: started}}))
		case r.URL.Path == "/datasets/dataset/items":
			w.Write([]byte(`[]`))
		default:
			t.Errorf("unexpected mock path %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()
	old := apifyBase
	apifyBase = srv.URL
	defer func() { apifyBase = old }()
	a.http = srv.Client()
	if e := a.tick(ctx); e != nil {
		t.Fatal(e)
	}
	if posts != 1 {
		t.Fatal("scheduler did not submit exactly once", posts)
	}
	// Simulate restart after a run was created remotely but its ID was not persisted locally.
	if _, e := a.db.Exec(ctx, "UPDATE jobs SET status='starting',run_id=NULL"); e != nil {
		t.Fatal(e)
	}
	restarted := &App{db: a.db, key: a.key, http: srv.Client(), storage: a.storage}
	if e := restarted.tick(ctx); e != nil {
		t.Fatal(e)
	}
	var status, run string
	if e := a.db.QueryRow(ctx, "SELECT status,run_id FROM jobs").Scan(&status, &run); e != nil {
		t.Fatal(e)
	}
	if status != "running" || run != "remote-run" || posts != 1 {
		t.Fatal("restart caused duplicate submission or lost run", status, run, posts)
	}
	rateLimit = true
	if e := restarted.tick(ctx); e != nil {
		t.Fatal(e)
	}
	var msg string
	a.db.QueryRow(ctx, "SELECT status,error FROM jobs").Scan(&status, &msg)
	if status != "running" || msg != "Apify HTTP 429" {
		t.Fatal("polling rate limit lost resumable state", status, msg)
	}
	rateLimit = false
	completed = true
	if e := restarted.tick(ctx); e != nil {
		t.Fatal(e)
	}
	if e := restarted.tick(ctx); e != nil {
		t.Fatal(e)
	}
	a.db.QueryRow(ctx, "SELECT status FROM jobs").Scan(&status)
	if status != "succeeded" {
		t.Fatal("worker lifecycle incomplete", status)
	}
	if _, e := a.db.Exec(ctx, "INSERT INTO jobs(id,source_id) VALUES('second','source')"); e != nil {
		t.Fatal(e)
	}
	if e := restarted.tick(ctx); e != nil {
		t.Fatal(e)
	}
	if posts != 1 {
		t.Fatal("5-minute cooldown ignored")
	}
}
