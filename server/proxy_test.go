package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMediaProxyIsExplicitAndPreservesAllowlist(t *testing.T) {
	direct, err := mediaTransport("")
	if err != nil || direct.Proxy != nil {
		t.Fatal("direct transport must not use ambient proxies")
	}
	for _, raw := range []string{"file:///tmp/proxy", "socks5://localhost:1080", "http://", "http://localhost:7897/path", "http://localhost:7897?target=host"} {
		if _, err := mediaTransport(raw); err == nil {
			t.Fatalf("accepted invalid proxy %q", raw)
		}
	}
	connects := make(chan string, 2)
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		connects <- r.Method + " " + r.Host
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer proxy.Close()
	t.Setenv("MEDIA_PROXY_URL", proxy.URL)
	a := &App{}
	if _, err := a.fetchImage(context.Background(), "https://127.0.0.1/private.jpg"); err == nil {
		t.Fatal("proxy bypassed host allowlist")
	}
	select {
	case <-connects:
		t.Fatal("blocked destination reached proxy")
	default:
	}
	if _, err := a.fetchImage(context.Background(), "https://pbs.twimg.com/media/test.jpg"); err == nil {
		t.Fatal("proxy error should fail without direct fallback")
	}
	select {
	case target := <-connects:
		if target != "CONNECT pbs.twimg.com:443" {
			t.Fatalf("unexpected target: %s", target)
		}
	default:
		t.Fatal("allowlisted image did not use configured proxy")
	}
}
