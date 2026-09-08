package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"regexp"
	"strings"
	"time"
)

const actorName = "xquik/x-tweet-scraper"
const actorPath = "xquik~x-tweet-scraper"

var apifyBase = "https://api.apify.com/v2"

type ActorRun struct {
	ID        string    `json:"id"`
	Status    string    `json:"status"`
	DatasetID string    `json:"defaultDatasetId"`
	StoreID   string    `json:"defaultKeyValueStoreId"`
	StartedAt time.Time `json:"startedAt"`
}

func (a *App) apifyToken(ctx context.Context) (string, error) {
	var raw []byte
	if e := a.db.QueryRow(ctx, "SELECT value FROM settings WHERE key='apify_token'").Scan(&raw); e != nil {
		return "", errors.New("Apify Token 未配置")
	}
	var v struct {
		Ciphertext string `json:"ciphertext"`
	}
	if e := json.Unmarshal(raw, &v); e != nil {
		return "", e
	}
	return decrypt(a.key, v.Ciphertext)
}
func (a *App) apify(ctx context.Context, token, method, path string, input any, out any) error {
	var body io.Reader
	if input != nil {
		body = bytes.NewReader(encode(input))
	}
	req, e := http.NewRequestWithContext(ctx, method, apifyBase+path, body)
	if e != nil {
		return e
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, e := a.http.Do(req)
	if e != nil {
		return errors.New("Apify 网络请求失败")
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("Apify HTTP %d", resp.StatusCode)
	}
	if out != nil {
		return json.NewDecoder(io.LimitReader(resp.Body, 32<<20)).Decode(out)
	}
	return nil
}
func buildInput(s Source, now time.Time) M {
	since := now.AddDate(0, 0, -7)
	if s.Watermark != nil {
		since = s.Watermark.AddDate(0, 0, -1)
	}
	query := s.Query
	queries := []string{}
	suffix := " filter:images since:" + since.UTC().Format("2006-01-02")
	if s.Kind == "accounts" {
		handles := strings.Fields(s.Query)
		// At most 100 authors => at most five OR queries in ONE Actor run.
		for start := 0; start < len(handles); start += 20 {
			end := min(start+20, len(handles))
			terms := make([]string, 0, end-start)
			for _, handle := range handles[start:end] {
				terms = append(terms, "from:"+handle)
			}
			queries = append(queries, "("+strings.Join(terms, " OR ")+")"+suffix)
		}
		return M{"searchTerms": queries, "mode": "search", "queryType": "Latest", "outputVariant": "rich", "fieldStyle": "camelCase", "maxItems": s.MaxItems, "includeSearchTerms": true}
	}
	if s.Kind == "account" {
		query = "from:" + strings.TrimPrefix(query, "@")
	}
	query += suffix
	return M{"searchTerms": []string{query}, "mode": "search", "queryType": "Latest", "outputVariant": "rich", "fieldStyle": "camelCase", "maxItems": s.MaxItems, "includeSearchTerms": true}
}
func (a *App) getRun(ctx context.Context, token, runID string) (ActorRun, error) {
	var res struct {
		Data ActorRun `json:"data"`
	}
	e := a.apify(ctx, token, "GET", "/actor-runs/"+url.PathEscape(runID), nil, &res)
	return res.Data, e
}
func (a *App) verifyRunInput(ctx context.Context, token string, run ActorRun, raw []byte, started *time.Time) error {
	if started == nil || run.StartedAt.Before(started.Add(-time.Minute)) || run.StartedAt.After(started.Add(10*time.Minute)) {
		return errors.New("run outside start window")
	}
	var expected, actual M
	if e := json.Unmarshal(raw, &expected); e != nil {
		return e
	}
	if e := a.apify(ctx, token, "GET", "/key-value-stores/"+url.PathEscape(run.StoreID)+"/records/INPUT", nil, &actual); e != nil {
		return e
	}
	for k, v := range expected {
		if !reflect.DeepEqual(v, actual[k]) {
			return errors.New("input mismatch")
		}
	}
	return nil
}
func str(m M, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k].(string); ok && v != "" {
			return v
		}
	}
	return ""
}
func obj(m M, keys ...string) M {
	for _, k := range keys {
		if v, ok := m[k].(map[string]any); ok {
			return v
		}
	}
	return M{}
}
func arr(m M, k string) []any { v, _ := m[k].([]any); return v }

var urlPattern = regexp.MustCompile(`https?://\S+`)

type TweetImage struct {
	Key      string
	URL      string
	Position int
}
type Tweet struct {
	ID        string
	URL       string
	Text      string
	Title     string
	AuthorID  string
	Author    M
	Tags      []string
	Images    []TweetImage
	Metrics   M
	Published time.Time
}

func parseTweet(raw M) (Tweet, error) {
	t := Tweet{ID: str(raw, "id", "id_str"), URL: str(raw, "url", "tweetUrl", "twitterUrl"), Text: str(raw, "full_text", "text"), Tags: []string{}, Images: []TweetImage{}, Metrics: M{}, Published: time.Now().UTC()}
	if t.ID == "" {
		return t, errors.New("结果缺少帖子 ID")
	}
	au := obj(raw, "author", "user")
	handle := str(au, "screen_name", "userName", "username")
	aid := str(au, "id_str", "id")
	if aid == "" {
		aid = handle
	}
	if aid == "" {
		aid = "unknown-" + t.ID
	}
	t.AuthorID = "x-author-" + aid
	t.Author = M{"name": str(au, "name", "screen_name", "userName"), "handle": handle, "avatar": str(au, "profile_image_url_https", "profilePicture"), "description": str(au, "description"), "url": "https://x.com/" + handle}
	if t.Author["name"] == "" {
		t.Author["name"] = "未署名作者"
	}
	u, urlErr := url.Parse(t.URL)
	if urlErr != nil || u.Scheme != "https" || (u.Hostname() != "x.com" && u.Hostname() != "twitter.com") || u.User != nil {
		t.URL = "https://x.com/" + url.PathEscape(handle) + "/status/" + url.PathEscape(t.ID)
	}
	if avatar, ok := t.Author["avatar"].(string); ok && avatar != "" {
		if allowedMediaURL(avatar) != nil {
			t.Author["avatar"] = ""
		}
	}
	for _, layout := range []string{time.RubyDate, time.RFC3339, "Mon Jan 02 15:04:05 -0700 2006"} {
		if ts, e := time.Parse(layout, str(raw, "created_at", "createdAt")); e == nil {
			t.Published = ts
			break
		}
	}
	title := strings.TrimSpace(urlPattern.ReplaceAllString(t.Text, ""))
	r := []rune(title)
	if len(r) > 60 {
		title = string(r[:60]) + "…"
	}
	if title == "" {
		title = "来自 @" + handle + " 的图片"
	}
	t.Title = title
	for _, v := range arr(obj(raw, "entities"), "hashtags") {
		if m, ok := v.(map[string]any); ok {
			if tag := str(m, "text", "tag"); tag != "" {
				t.Tags = append(t.Tags, tag)
			}
		}
	}
	extended := obj(raw, "extended_entities", "extendedEntities")
	if len(extended) == 0 {
		extended = obj(raw, "entities")
	}
	media := arr(extended, "media")
	if len(media) == 0 {
		media = arr(raw, "media")
	}
	seen := map[string]bool{}
	for i, v := range media {
		m, ok := v.(map[string]any)
		if !ok {
			continue
		}
		typ := str(m, "type")
		if typ != "photo" && typ != "image" {
			continue
		}
		u := str(m, "media_url_https", "mediaUrlHttps", "media_url", "mediaUrl", "url")
		if u == "" {
			continue
		}
		key := str(m, "id_str", "id", "media_key")
		if key == "" {
			key = tokenHash(strings.Split(u, "?")[0])
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		t.Images = append(t.Images, TweetImage{key, u, i})
	}
	for dst, keys := range map[string][]string{"likes": {"favorite_count", "likeCount"}, "reposts": {"retweet_count", "retweetCount"}, "replies": {"reply_count", "replyCount"}} {
		for _, k := range keys {
			if v, ok := raw[k]; ok {
				t.Metrics[dst] = v
				break
			}
		}
	}
	return t, nil
}
func (a *App) importTweet(ctx context.Context, s Source, raw M) (bool, error) {
	t, e := parseTweet(raw)
	if e != nil {
		return false, e
	}
	if len(t.Images) == 0 {
		return false, nil
	}
	type ready struct {
		m      TweetImage
		stored StoredImage
	}
	readyImages := []ready{}
	failed := []string{}
	fetch := a.fetchImage
	if a.fetchOverride != nil {
		fetch = a.fetchOverride
	}
	for _, m := range t.Images {
		stored, e := fetch(ctx, m.URL)
		if e != nil {
			failed = append(failed, m.Key+": "+e.Error())
			continue
		}
		short, long := stored.Width, stored.Height
		if short > long {
			short, long = long, short
		}
		if short < s.MinShort || long < s.MinLong {
			continue
		}
		readyImages = append(readyImages, ready{m, stored})
	}
	if len(readyImages) == 0 {
		if len(failed) > 0 {
			return false, errors.New(strings.Join(failed, "; "))
		}
		return false, nil
	}
	tx, e := a.db.Begin(ctx)
	if e != nil {
		return false, e
	}
	defer tx.Rollback(ctx)
	_, e = tx.Exec(ctx, "INSERT INTO entities(id,kind,data) VALUES($1,'author',$2) ON CONFLICT(id) DO UPDATE SET data=entities.data||excluded.data", t.AuthorID, encode(t.Author))
	if e != nil {
		return false, e
	}
	wid := id()
	tags := append(t.Tags, s.Tags...)
	tag, e := tx.Exec(ctx, "INSERT INTO wallpapers(id,source_id,author_id,channel_id,title,description,tags,topic_ids,source_url,source_metrics,published_at) VALUES($1,$2,$3,'x',$4,$5,$6,$7,$8,$9,$10) ON CONFLICT(source_id) DO NOTHING", wid, "x:"+t.ID, t.AuthorID, t.Title, t.Text, tags, s.TopicIDs, t.URL, encode(t.Metrics), t.Published)
	if e != nil {
		return false, e
	}
	created := tag.RowsAffected() > 0
	if !created {
		if e = tx.QueryRow(ctx, "SELECT id FROM wallpapers WHERE source_id=$1", "x:"+t.ID).Scan(&wid); e != nil {
			return false, e
		}
		_, e = tx.Exec(ctx, "UPDATE wallpapers SET source_metrics=$2,topic_ids=ARRAY(SELECT DISTINCT unnest(topic_ids||$3::text[])) WHERE id=$1", wid, encode(t.Metrics), s.TopicIDs)
		if e != nil {
			return false, e
		}
	}
	for _, r := range readyImages {
		v := r.stored
		_, e = tx.Exec(ctx, "INSERT INTO media(id,wallpaper_id,source_key,storage_key,thumbnail_key,width,height,bytes,mime,position) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) ON CONFLICT(wallpaper_id,source_key) DO NOTHING", id(), wid, r.m.Key, v.Key, v.Thumbnail, v.Width, v.Height, v.Bytes, v.Mime, r.m.Position)
		if e != nil {
			return false, e
		}
	}
	if e = tx.Commit(ctx); e != nil {
		return false, e
	}
	if len(failed) > 0 {
		return created, errors.New(strings.Join(failed, "; "))
	}
	return created, nil
}
