package main

import (
	"errors"
	"github.com/gin-gonic/gin"
	"regexp"
	"strings"
	"time"
)

var xHandle = regexp.MustCompile(`^[A-Za-z0-9_]{1,15}$`)

func normalizeSourceQuery(kind, query string) (string, error) {
	query = strings.TrimSpace(query)
	if kind == "keyword" {
		if query == "" || len(query) > 500 {
			return "", errors.New("关键词需为 1–500 个字节")
		}
		return query, nil
	}
	if kind != "accounts" {
		return "", errors.New("请将单账号来源合并为作者组，或选择关键词批量搜索")
	}
	parts := strings.Fields(strings.NewReplacer(",", " ", "，", " ", ";", " ", "；", " ").Replace(query))
	seen := map[string]bool{}
	handles := []string{}
	for _, part := range parts {
		handle := strings.TrimPrefix(part, "@")
		if !xHandle.MatchString(handle) {
			return "", errors.New("作者请填写 X 用户名，不包含网址或搜索表达式；每个用户名最多 15 个字符")
		}
		key := strings.ToLower(handle)
		if !seen[key] {
			seen[key] = true
			handles = append(handles, handle)
		}
	}
	if len(handles) < 2 || len(handles) > 100 {
		return "", errors.New("每个作者组需要 2–100 个不同作者，请批量填写")
	}
	return strings.Join(handles, "\n"), nil
}

func (a *App) previewSource(c *gin.Context) {
	s := Source{MaxItems: 100}
	if c.ShouldBindJSON(&s) != nil || s.MaxItems < 1 || s.MaxItems > 1000 {
		fail(c, 400, "请检查批次总条数（1–1000）")
		return
	}
	query, err := normalizeSourceQuery(s.Kind, s.Query)
	if err != nil {
		fail(c, 400, err.Error())
		return
	}
	s.Query = query
	// Existing source watermarks are server-owned; do not trust browser-supplied dates.
	s.Watermark = nil
	if s.ID != "" {
		var oldKind, oldQuery string
		var mark *time.Time
		if !check(c, a.db.QueryRow(c, "SELECT kind,query,watermark FROM sources WHERE id=$1", s.ID).Scan(&oldKind, &oldQuery, &mark)) {
			return
		}
		if oldKind == s.Kind && oldQuery == s.Query {
			s.Watermark = mark
		}
	}
	input := buildInput(s, time.Now())
	authors := 0
	if s.Kind == "accounts" {
		authors = len(strings.Fields(s.Query))
	}
	c.JSON(200, M{"actor": actorName, "input": input, "authorCount": authors, "queryCount": len(input["searchTerms"].([]string)), "runs": 1})
}
