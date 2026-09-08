package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const wallpaperJSON = `jsonb_build_object(
 'id',w.id,'title',w.title,'description',w.description,'tags',w.tags,'topicIds',w.topic_ids,
 'sourceUrl',w.source_url,'sourceMetrics',w.source_metrics,'publishedAt',w.published_at,'status',w.status,'featured',w.featured,
 'author',COALESCE((SELECT e.data || jsonb_build_object('id',e.id) FROM entities e WHERE e.id=w.author_id),'{}'),
 'channel',COALESCE((SELECT e.data || jsonb_build_object('id',e.id) FROM entities e WHERE e.id=w.channel_id),'{}'),
 'images',COALESCE((SELECT jsonb_agg(jsonb_build_object('id',m.id,'url','/media/'||m.storage_key,'thumbnail','/media/'||m.thumbnail_key,'width',m.width,'height',m.height,'bytes',m.bytes,'mime',m.mime) ORDER BY m.position) FROM media m WHERE m.wallpaper_id=w.id),'[]'),
 'likes',(SELECT count(*) FROM interactions i WHERE i.wallpaper_id=w.id AND i.kind='like'),
 'favorites',(SELECT count(*) FROM interactions i WHERE i.wallpaper_id=w.id AND i.kind='favorite'),
 'downloads',(SELECT count(*) FROM downloads d WHERE d.wallpaper_id=w.id),
 'liked',EXISTS(SELECT 1 FROM interactions i WHERE i.wallpaper_id=w.id AND i.kind='like' AND i.user_id=$1),
 'favorited',EXISTS(SELECT 1 FROM interactions i WHERE i.wallpaper_id=w.id AND i.kind='favorite' AND i.user_id=$1))`

type Cursor struct {
	At time.Time `json:"at"`
	ID string    `json:"id"`
}

func parseCursor(s string) (Cursor, error) {
	var c Cursor
	if s == "" {
		return Cursor{At: time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC), ID: "~"}, nil
	}
	b, e := base64.RawURLEncoding.DecodeString(s)
	if e != nil {
		return c, e
	}
	e = json.Unmarshal(b, &c)
	if c.At.IsZero() || c.ID == "" {
		return c, fmt.Errorf("invalid cursor")
	}
	return c, e
}
func (a *App) wallpapers(c *gin.Context) {
	cursor, e := parseCursor(c.Query("cursor"))
	if e != nil {
		fail(c, 400, "分页参数无效")
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "24"))
	if limit < 1 || limit > 60 {
		limit = 24
	}
	scope := c.Query("scope")
	if scope != "" && userID(c) == "" {
		fail(c, 401, "请先登录")
		return
	}
	filters := []string{"w.status='published'", "EXISTS(SELECT 1 FROM media m WHERE m.wallpaper_id=w.id)"}
	if c.Query("featured") == "prefer" && c.Query("topic") == "" {
		filters = append(filters, "(w.featured OR NOT EXISTS(SELECT 1 FROM wallpapers f WHERE f.featured AND f.status='published' AND EXISTS(SELECT 1 FROM media fm WHERE fm.wallpaper_id=f.id)))")
	}
	if c.Query("featured") == "true" {
		filters = append(filters, "w.featured")
	}
	args := []any{userID(c)}
	add := func(expr string, value any) {
		args = append(args, value)
		filters = append(filters, fmt.Sprintf(expr, len(args)))
	}
	if q := strings.TrimSpace(c.Query("q")); q != "" {
		add("(w.title ILIKE $%[1]d OR w.description ILIKE $%[1]d OR array_to_string(w.tags,' ') ILIKE $%[1]d OR EXISTS(SELECT 1 FROM entities e WHERE e.id=w.author_id AND e.data->>'name' ILIKE $%[1]d))", "%"+q+"%")
	}
	if s := c.Query("topic"); s != "" {
		add("$%d=ANY(w.topic_ids)", s)
	}
	if s := c.Query("author"); s != "" {
		add("w.author_id=$%d", s)
	}
	if s := c.Query("channel"); s != "" {
		add("w.channel_id=$%d", s)
	}
	if s := c.Query("tag"); s != "" {
		add("$%d=ANY(w.tags)", s)
	}
	switch c.Query("orientation") {
	case "landscape":
		filters = append(filters, "EXISTS(SELECT 1 FROM media m WHERE m.wallpaper_id=w.id AND m.width>m.height)")
	case "portrait":
		filters = append(filters, "EXISTS(SELECT 1 FROM media m WHERE m.wallpaper_id=w.id AND m.height>m.width)")
	}
	sortAt := "w.published_at"
	switch scope {
	case "favorites":
		filters = append(filters, "EXISTS(SELECT 1 FROM interactions i WHERE i.wallpaper_id=w.id AND i.user_id=$1 AND i.kind='favorite')")
		sortAt = "(SELECT created_at FROM interactions i WHERE i.wallpaper_id=w.id AND i.user_id=$1 AND i.kind='favorite')"
	case "downloads":
		filters = append(filters, "EXISTS(SELECT 1 FROM downloads d WHERE d.wallpaper_id=w.id AND d.user_id=$1)")
		sortAt = "(SELECT max(created_at) FROM downloads d WHERE d.wallpaper_id=w.id AND d.user_id=$1)"
	case "history":
		filters = append(filters, "EXISTS(SELECT 1 FROM history h WHERE h.wallpaper_id=w.id AND h.user_id=$1)")
		sortAt = "(SELECT viewed_at FROM history h WHERE h.wallpaper_id=w.id AND h.user_id=$1)"
	case "subscriptions":
		filters = append(filters, "EXISTS(SELECT 1 FROM subscriptions s WHERE s.user_id=$1 AND (s.entity_id=w.author_id OR s.entity_id=w.channel_id OR s.entity_id=ANY(w.topic_ids)))")
	case "":
	default:
		fail(c, 400, "未知列表类型")
		return
	}
	args = append(args, cursor.At, cursor.ID, limit+1)
	n := len(args)
	query := fmt.Sprintf("SELECT item,sort_at,id FROM (SELECT %s AS item,%s AS sort_at,w.id FROM wallpapers w WHERE %s) x WHERE (sort_at,id)<($%d,$%d) ORDER BY sort_at DESC,id DESC LIMIT $%d", wallpaperJSON, sortAt, strings.Join(filters, " AND "), n-2, n-1, n)
	rows, e := a.db.Query(c, query, args...)
	if !check(c, e) {
		return
	}
	defer rows.Close()
	items := []M{}
	cursors := []Cursor{}
	for rows.Next() {
		var b []byte
		var cur Cursor
		if !check(c, rows.Scan(&b, &cur.At, &cur.ID)) {
			return
		}
		var item M
		_ = json.Unmarshal(b, &item)
		items = append(items, item)
		cursors = append(cursors, cur)
	}
	if !check(c, rows.Err()) {
		return
	}
	next := ""
	if len(items) > limit {
		items = items[:limit]
		next = base64.RawURLEncoding.EncodeToString(encode(cursors[limit-1]))
	}
	c.JSON(200, M{"items": items, "nextCursor": next})
}
func (a *App) wallpaper(c *gin.Context) {
	var raw []byte
	e := a.db.QueryRow(c, "SELECT "+wallpaperJSON+" FROM wallpapers w WHERE w.id=$2 AND w.status='published' AND EXISTS(SELECT 1 FROM media m WHERE m.wallpaper_id=w.id)", userID(c), c.Param("id")).Scan(&raw)
	if check(c, e) {
		c.Data(200, "application/json", raw)
	}
}
func (a *App) history(c *gin.Context) {
	_, e := a.db.Exec(c, `INSERT INTO history(user_id,wallpaper_id) SELECT $1,id FROM wallpapers WHERE id=$2 AND status='published' ON CONFLICT(user_id,wallpaper_id) DO UPDATE SET viewed_at=now()`, userID(c), c.Param("id"))
	if !check(c, e) {
		return
	}
	_, e = a.db.Exec(c, `DELETE FROM history WHERE user_id=$1 AND wallpaper_id NOT IN (SELECT wallpaper_id FROM history WHERE user_id=$1 ORDER BY viewed_at DESC LIMIT 50)`, userID(c))
	if check(c, e) {
		c.JSON(200, M{"ok": true})
	}
}
func (a *App) interaction(c *gin.Context) {
	kind := c.Param("kind")
	if kind != "like" && kind != "favorite" {
		fail(c, 400, "未知操作")
		return
	}
	var exists bool
	if !check(c, a.db.QueryRow(c, "SELECT EXISTS(SELECT 1 FROM wallpapers WHERE id=$1 AND status='published')", c.Param("id")).Scan(&exists)) {
		return
	}
	if !exists {
		fail(c, 404, "内容不存在")
		return
	}
	var e error
	if c.Request.Method == "PUT" {
		_, e = a.db.Exec(c, "INSERT INTO interactions(user_id,wallpaper_id,kind) VALUES($1,$2,$3) ON CONFLICT DO NOTHING", userID(c), c.Param("id"), kind)
	} else {
		_, e = a.db.Exec(c, "DELETE FROM interactions WHERE user_id=$1 AND wallpaper_id=$2 AND kind=$3", userID(c), c.Param("id"), kind)
	}
	if check(c, e) {
		c.JSON(200, M{"ok": true})
	}
}

const entityJSON = `e.data || jsonb_build_object('cover',COALESCE(NULLIF(e.data->>'cover',''),(SELECT '/media/'||m.thumbnail_key FROM wallpapers w JOIN media m ON m.wallpaper_id=w.id WHERE w.status='published' AND (w.author_id=e.id OR w.channel_id=e.id OR e.id=ANY(w.topic_ids)) ORDER BY w.published_at DESC,w.id DESC,m.position LIMIT 1),''),'id',e.id,'kind',e.kind,'subscribed',EXISTS(SELECT 1 FROM subscriptions s WHERE s.entity_id=e.id AND s.user_id=$1),'subscribers',(SELECT count(*) FROM subscriptions s WHERE s.entity_id=e.id),'count',(SELECT count(*) FROM wallpapers w WHERE w.status='published' AND (w.author_id=e.id OR w.channel_id=e.id OR e.id=ANY(w.topic_ids))))`

func (a *App) entities(c *gin.Context) {
	kind := c.Query("kind")
	cursor := c.Query("cursor")
	q := "SELECT " + entityJSON + " FROM entities e WHERE ($2='' OR e.kind=$2) AND e.id>$3 AND ($4=false OR EXISTS(SELECT 1 FROM subscriptions s WHERE s.entity_id=e.id AND s.user_id=$1)) ORDER BY e.id LIMIT 61"
	items, e := rowsJSON(c, a.db, q, userID(c), kind, cursor, c.Query("subscribed") == "true")
	if !check(c, e) {
		return
	}
	next := ""
	if len(items) > 60 {
		items = items[:60]
		next = items[59]["id"].(string)
	}
	c.JSON(200, M{"items": items, "nextCursor": next})
}
func (a *App) entity(c *gin.Context) {
	var raw []byte
	e := a.db.QueryRow(c, "SELECT "+entityJSON+" FROM entities e WHERE e.id=$2", userID(c), c.Param("id")).Scan(&raw)
	if check(c, e) {
		c.Data(200, "application/json", raw)
	}
}
func (a *App) subscription(c *gin.Context) {
	var exists bool
	if !check(c, a.db.QueryRow(c, "SELECT EXISTS(SELECT 1 FROM entities WHERE id=$1)", c.Param("id")).Scan(&exists)) {
		return
	}
	if !exists {
		fail(c, 404, "订阅对象不存在")
		return
	}
	var e error
	if c.Request.Method == "PUT" {
		_, e = a.db.Exec(c, "INSERT INTO subscriptions(user_id,entity_id) VALUES($1,$2) ON CONFLICT DO NOTHING", userID(c), c.Param("id"))
	} else {
		_, e = a.db.Exec(c, "DELETE FROM subscriptions WHERE user_id=$1 AND entity_id=$2", userID(c), c.Param("id"))
	}
	if check(c, e) {
		c.JSON(200, M{"ok": true})
	}
}
func (a *App) ranks(c *gin.Context) {
	period := c.DefaultQuery("period", "7d")
	if period != "7d" && period != "30d" && period != "all" {
		fail(c, 400, "未知统计周期")
		return
	}
	kind := c.DefaultQuery("kind", "wallpaper")
	var items []M
	var e error
	if kind == "wallpaper" {
		items, e = rowsJSON(c, a.db, "SELECT "+wallpaperJSON+" || jsonb_build_object('score',COALESCE(r.score,0)) FROM wallpapers w LEFT JOIN rank_snapshots r ON r.wallpaper_id=w.id AND r.period=$2 WHERE w.status='published' AND EXISTS(SELECT 1 FROM media m WHERE m.wallpaper_id=w.id) ORDER BY COALESCE(r.score,0) DESC,w.published_at DESC,w.id DESC LIMIT 50", userID(c), period)
	} else if kind == "author" || kind == "channel" {
		items, e = rowsJSON(c, a.db, "SELECT "+entityJSON+" || jsonb_build_object('score',COALESCE((SELECT sum(r.score) FROM rank_snapshots r JOIN wallpapers w ON w.id=r.wallpaper_id WHERE r.period=$2 AND w.status='published' AND (w.author_id=e.id OR w.channel_id=e.id)),0)) FROM entities e WHERE e.kind=$3 ORDER BY COALESCE((SELECT sum(r.score) FROM rank_snapshots r JOIN wallpapers w ON w.id=r.wallpaper_id WHERE r.period=$2 AND w.status='published' AND (w.author_id=e.id OR w.channel_id=e.id)),0) DESC,e.created_at DESC,e.id LIMIT 50", userID(c), period, kind)
	} else {
		fail(c, 400, "未知排行类型")
		return
	}
	if check(c, e) {
		c.JSON(200, M{"items": items, "period": period})
	}
}
func (a *App) home(c *gin.Context) {
	var value []byte
	e := a.db.QueryRow(c, "SELECT value FROM settings WHERE key='homepage'").Scan(&value)
	if e != nil {
		value = []byte(`{"banners":[]}`)
	}
	c.Data(200, "application/json", value)
}
