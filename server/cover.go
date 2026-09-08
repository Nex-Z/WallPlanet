package main

import (
	"net/url"
	"strings"
)

// Covers are rendered by the browser, never downloaded by the server.
func validCoverRef(s string) bool {
	if len(s) > 4096 || strings.ContainsAny(s, "\\\r\n\t") {
		return false
	}
	if validImageRef(s) {
		return true
	}
	u, e := url.Parse(s)
	return e == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Hostname() != "" && u.User == nil
}
