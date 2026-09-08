package main

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestXquikBatchInput(t *testing.T) {
	handles := []string{}
	for i := 0; i < 100; i++ {
		handles = append(handles, fmt.Sprintf("artist%d", i))
	}
	normalized, e := normalizeSourceQuery("accounts", strings.Join(handles, ","))
	if e != nil {
		t.Fatal(e)
	}
	input := buildInput(Source{Kind: "accounts", Query: normalized, MaxItems: 100}, time.Date(2026, 9, 8, 0, 0, 0, 0, time.UTC))
	queries := input["searchTerms"].([]string)
	if actorPath != "xquik~x-tweet-scraper" || len(queries) != 5 || input["maxItems"] != 100 || input["queryType"] != "Latest" || input["mode"] != "search" {
		t.Fatal(input)
	}
	for _, q := range queries {
		if strings.Count(q, "from:") != 20 || !strings.HasSuffix(q, "filter:images since:2026-09-01") {
			t.Fatal(q)
		}
	}
	for _, key := range []string{"twitterHandles", "sort", "onlyImage", "maxItemsPerQuery"} {
		if _, ok := input[key]; ok {
			t.Fatal("unexpected per-author/obsolete input", key)
		}
	}
	got, e := normalizeSourceQuery("accounts", "@NASA,nasa；NASAWebb")
	if e != nil || got != "NASA\nNASAWebb" {
		t.Fatal(got, e)
	}
	for _, q := range []string{"NASA", "NASA, from:evil", strings.Join(append(handles, "extra"), ",")} {
		if _, e := normalizeSourceQuery("accounts", q); e == nil {
			t.Fatal("accepted invalid group")
		}
	}
}

func TestXquikOutputAndPartialReport(t *testing.T) {
	raw := M{"id": "123", "text": "Mountains", "createdAt": "2026-09-08T00:00:00Z", "author": map[string]any{"id": "a1", "username": "artist", "name": "Artist"}, "media": []any{map[string]any{"type": "photo", "url": "https://pbs.twimg.com/media/one.jpg"}, map[string]any{"type": "photo", "url": "https://pbs.twimg.com/media/two.jpg"}, map[string]any{"type": "video", "url": "https://pbs.twimg.com/media/video.jpg"}}, "likeCount": float64(42)}
	tw, e := parseTweet(raw)
	if e != nil || len(tw.Images) != 2 || tw.AuthorID != "x-author-a1" || tw.Metrics["likes"] != float64(42) {
		t.Fatal(tw, e)
	}
	for _, reason := range []string{"partial_failure", "deadline_reached", "pagination_safety_limit", ""} {
		if reportWarning(M{"completionReason": reason}) == "" {
			t.Fatal(reason)
		}
	}
	if reportWarning(M{"completionReason": "completed", "failedSubtargets": float64(0)}) != "" {
		t.Fatal("complete report rejected")
	}
}
