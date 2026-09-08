package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	"net/url"
	"strings"
	"time"
)

func (a *App) adminOverview(c *gin.Context) {
	var raw []byte
	e := a.db.QueryRow(c, `SELECT jsonb_build_object('users',(SELECT count(*) FROM users),'wallpapers',(SELECT count(*) FROM wallpapers),'sources',(SELECT count(*) FROM sources),'activeJobs',(SELECT count(*) FROM jobs WHERE status IN ('queued','starting','running','importing','retrying')),'failedItems',(SELECT count(*) FROM job_items WHERE NOT resolved))`).Scan(&raw)
	if check(c, e) {
		c.Data(200, "application/json", raw)
	}
}
func (a *App) adminSettings(c *gin.Context) {
	var configured bool
	var at *time.Time
	e := a.db.QueryRow(c, "SELECT EXISTS(SELECT 1 FROM settings WHERE key='apify_token'),(SELECT updated_at FROM settings WHERE key='apify_token')").Scan(&configured, &at)
	if check(c, e) {
		c.JSON(200, M{"tokenConfigured": configured, "tokenMask": map[bool]string{true: "••••••••", false: ""}[configured], "updatedAt": at, "actor": actorName, "demo": a.demo})
	}
}
func (a *App) saveSettings(c *gin.Context) {
	if a.demo {
		fail(c, 400, "演示环境不保存真实 Apify 配置")
		return
	}
	var in struct {
		Token string `json:"token"`
	}
	if c.ShouldBindJSON(&in) != nil || len(in.Token) < 10 || len(in.Token) > 512 {
		fail(c, 400, "请输入有效的 Apify Token")
		return
	}
	cipher, e := encrypt(a.key, strings.TrimSpace(in.Token))
	if !check(c, e) {
		return
	}
	_, e = a.db.Exec(c, "INSERT INTO settings(key,value) VALUES('apify_token',$1) ON CONFLICT(key) DO UPDATE SET value=excluded.value,updated_at=now()", encode(M{"ciphertext": cipher}))
	if check(c, e) {
		a.adminSettings(c)
	}
}

type Source struct {
	ID            string     `json:"id"`
	Name          string     `json:"name"`
	Kind          string     `json:"kind"`
	Query         string     `json:"query"`
	Enabled       bool       `json:"enabled"`
	IntervalHours int        `json:"interval_hours"`
	MaxItems      int        `json:"max_items"`
	MinShort      int        `json:"min_short"`
	MinLong       int        `json:"min_long"`
	TopicIDs      []string   `json:"topic_ids"`
	Tags          []string   `json:"tags"`
	Watermark     *time.Time `json:"watermark"`
	NextRun       time.Time  `json:"next_run"`
}

func (a *App) listSources(c *gin.Context) {
	items, e := rowsJSON(c, a.db, "SELECT to_jsonb(s) FROM sources s ORDER BY s.created_at DESC")
	if check(c, e) {
		c.JSON(200, M{"items": items})
	}
}
func (a *App) saveSource(c *gin.Context) {
	in := Source{IntervalHours: 168, MaxItems: 100, MinShort: 720, MinLong: 1280, TopicIDs: []string{}, Tags: []string{}}
	if c.ShouldBindJSON(&in) != nil {
		fail(c, 400, "采集配置格式不正确")
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	in.Query = strings.TrimSpace(in.Query)
	if in.Name == "" || len(in.Name) > 100 || in.Query == "" || len(in.Query) > 5000 || (in.Kind != "accounts" && in.Kind != "keyword") || in.IntervalHours < 1 || in.IntervalHours > 720 || in.MaxItems < 1 || in.MaxItems > 1000 || in.MinShort < 1 || in.MinLong < in.MinShort {
		fail(c, 400, "请检查来源名称、查询、频率、条数及分辨率")
		return
	}
	query, err := normalizeSourceQuery(in.Kind, in.Query)
	if err != nil {
		fail(c, 400, err.Error())
		return
	}
	in.Query = query
	if len(in.TopicIDs) > 20 || len(in.Tags) > 30 {
		fail(c, 400, "分类或标签数量过多")
		return
	}
	for _, tid := range in.TopicIDs {
		var ok bool
		if !check(c, a.db.QueryRow(c, "SELECT EXISTS(SELECT 1 FROM entities WHERE id=$1 AND kind='topic')", tid).Scan(&ok)) {
			return
		}
		if !ok {
			fail(c, 400, "主题不存在")
			return
		}
	}
	in.ID = c.Param("id")
	if in.ID == "" {
		in.ID = id()
		_, e := a.db.Exec(c, "INSERT INTO sources(id,name,kind,query,enabled,interval_hours,max_items,min_short,min_long,topic_ids,tags) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)", in.ID, in.Name, in.Kind, in.Query, in.Enabled, in.IntervalHours, in.MaxItems, in.MinShort, in.MinLong, in.TopicIDs, in.Tags)
		if !check(c, e) {
			return
		}
	} else {
		tag, e := a.db.Exec(c, "UPDATE sources SET name=$2,kind=$3,query=$4,enabled=$5,interval_hours=$6,max_items=$7,min_short=$8,min_long=$9,topic_ids=$10,tags=$11,watermark=CASE WHEN kind<>$3 OR query<>$4 THEN NULL ELSE watermark END,next_run=CASE WHEN kind<>$3 OR query<>$4 THEN now() ELSE next_run END WHERE id=$1", in.ID, in.Name, in.Kind, in.Query, in.Enabled, in.IntervalHours, in.MaxItems, in.MinShort, in.MinLong, in.TopicIDs, in.Tags)
		if !check(c, e) {
			return
		}
		if tag.RowsAffected() == 0 {
			fail(c, 404, "来源不存在")
			return
		}
	}
	c.JSON(200, in)
}
func (a *App) enqueue(c *gin.Context) {
	if a.demo {
		fail(c, 400, "演示环境不启动付费采集")
		return
	}
	var ready bool
	if !check(c, a.db.QueryRow(c, "SELECT EXISTS(SELECT 1 FROM settings WHERE key='apify_token')").Scan(&ready)) {
		return
	}
	if !ready {
		fail(c, 400, "请先保存 Apify Token")
		return
	}
	var exists bool
	if !check(c, a.db.QueryRow(c, "SELECT EXISTS(SELECT 1 FROM sources WHERE id=$1 AND kind<>'account')", c.Param("id")).Scan(&exists)) {
		return
	}
	if !exists {
		fail(c, 404, "来源不存在")
		return
	}
	jid := id()
	tag, e := a.db.Exec(c, "INSERT INTO jobs(id,source_id,source_snapshot) SELECT $1,id,to_jsonb(s) FROM sources s WHERE id=$2 ON CONFLICT DO NOTHING", jid, c.Param("id"))
	if !check(c, e) {
		return
	}
	if tag.RowsAffected() == 0 {
		fail(c, 409, "该来源已有未完成任务")
		return
	}
	c.JSON(202, M{"id": jid})
}
func (a *App) listJobs(c *gin.Context) {
	cur, e := parseCursor(c.Query("cursor"))
	if e != nil {
		fail(c, 400, "分页参数无效")
		return
	}
	items, e := rowsJSON(c, a.db, "SELECT to_jsonb(j)||jsonb_build_object('source_name',s.name,'failed_items',(SELECT count(*) FROM job_items i WHERE i.job_id=j.id AND NOT i.resolved)) FROM jobs j JOIN sources s ON s.id=j.source_id WHERE (j.created_at,j.id)<($1,$2) ORDER BY j.created_at DESC,j.id DESC LIMIT 51", cur.At, cur.ID)
	if !check(c, e) {
		return
	}
	next := ""
	if len(items) > 50 {
		items = items[:50]
		at, _ := time.Parse(time.RFC3339, items[49]["created_at"].(string))
		next = base64.RawURLEncoding.EncodeToString(encode(Cursor{At: at, ID: items[49]["id"].(string)}))
	}
	c.JSON(200, M{"items": items, "nextCursor": next})
}
func (a *App) jobItems(c *gin.Context) {
	items, e := rowsJSON(c, a.db, "SELECT jsonb_build_object('id',id,'source_key',source_key,'error',error,'attempts',attempts,'resolved',resolved) FROM job_items WHERE job_id=$1 ORDER BY id LIMIT 100", c.Param("id"))
	if check(c, e) {
		c.JSON(200, M{"items": items})
	}
}
func (a *App) retryJob(c *gin.Context) {
	tag, e := a.db.Exec(c, "UPDATE jobs SET status='retrying',error='',retry_item_id='' WHERE id=$1 AND status IN ('partial','failed') AND dataset_id IS NOT NULL", c.Param("id"))
	if !check(c, e) {
		return
	}
	if tag.RowsAffected() == 0 {
		fail(c, 409, "此任务不能重试；启动状态不明请先核对运行记录")
		return
	}
	c.JSON(202, M{"ok": true})
}

func (a *App) retryJobItem(c *gin.Context) {
	tag, e := a.db.Exec(c, "UPDATE jobs SET status='retrying',error='',retry_item_id=$2 WHERE id=$1 AND status IN ('partial','failed') AND dataset_id IS NOT NULL AND EXISTS(SELECT 1 FROM job_items WHERE id=$2 AND job_id=$1 AND NOT resolved)", c.Param("id"), c.Param("item"))
	if !check(c, e) {
		return
	}
	if tag.RowsAffected() == 0 {
		fail(c, 409, "只能重试已结束任务中的失败条目")
		return
	}
	c.JSON(202, M{"ok": true})
}
func (a *App) reconcileJob(c *gin.Context) {
	var in struct {
		RunID         string `json:"runId"`
		ConfirmAbsent bool   `json:"confirmAbsent"`
	}
	if c.ShouldBindJSON(&in) != nil {
		fail(c, 400, "参数不正确")
		return
	}
	if in.RunID != "" {
		if len(in.RunID) > 64 {
			fail(c, 400, "运行 ID 无效")
			return
		}
		token, e := a.apifyToken(c)
		if !check(c, e) {
			return
		}
		run, e := a.getRun(c, token, in.RunID)
		if e != nil {
			fail(c, 400, "无法验证运行 ID")
			return
		}
		var inputRaw []byte
		var started *time.Time
		if !check(c, a.db.QueryRow(c, "SELECT input,started_at FROM jobs WHERE id=$1 AND status='uncertain'", c.Param("id")).Scan(&inputRaw, &started)) {
			return
		}
		if e = a.verifyRunInput(c, token, run, inputRaw, started); e != nil {
			fail(c, 400, "运行时间或输入与此任务不匹配")
			return
		}
		tag, e := a.db.Exec(c, "UPDATE jobs SET run_id=$2,status='running',error='' WHERE id=$1 AND status='uncertain'", c.Param("id"), in.RunID)
		if !check(c, e) {
			return
		}
		if tag.RowsAffected() == 0 {
			fail(c, 409, "任务状态已改变")
			return
		}
	} else if in.ConfirmAbsent {
		tag, e := a.db.Exec(c, "UPDATE jobs SET status='failed',error='管理员确认 Apify 未创建运行',finished_at=now() WHERE id=$1 AND status='uncertain'", c.Param("id"))
		if !check(c, e) {
			return
		}
		if tag.RowsAffected() == 0 {
			fail(c, 409, "任务状态已改变")
			return
		}
	} else {
		fail(c, 400, "请提供 Apify run ID，或确认控制台中不存在此次运行")
		return
	}
	c.JSON(200, M{"ok": true})
}
func (a *App) adminWallpapers(c *gin.Context) {
	items, e := rowsJSON(c, a.db, "SELECT "+wallpaperJSON+" FROM wallpapers w WHERE ($2='' OR w.title ILIKE '%'||$2||'%') AND ($3='' OR w.id<$3) ORDER BY w.id DESC LIMIT 51", userID(c), c.Query("q"), c.Query("cursor"))
	if !check(c, e) {
		return
	}
	next := ""
	if len(items) > 50 {
		items = items[:50]
		next = items[49]["id"].(string)
	}
	c.JSON(200, M{"items": items, "nextCursor": next})
}
func (a *App) editWallpaper(c *gin.Context) {
	var in struct {
		Title       string   `json:"title"`
		Description string   `json:"description"`
		Tags        []string `json:"tags"`
		TopicIDs    []string `json:"topicIds"`
		Status      string   `json:"status"`
		Featured    bool     `json:"featured"`
	}
	if c.ShouldBindJSON(&in) != nil || strings.TrimSpace(in.Title) == "" || len(in.Title) > 500 || len(in.Description) > 20000 || len(in.Tags) > 30 || len(in.TopicIDs) > 20 || (in.Status != "published" && in.Status != "hidden") {
		fail(c, 400, "作品信息不正确")
		return
	}
	tag, e := a.db.Exec(c, "UPDATE wallpapers SET title=$2,description=$3,tags=$4,topic_ids=$5,status=$6,featured=$7 WHERE id=$1", c.Param("id"), in.Title, in.Description, in.Tags, in.TopicIDs, in.Status, in.Featured)
	if !check(c, e) {
		return
	}
	if tag.RowsAffected() == 0 {
		fail(c, 404, "作品不存在")
		return
	}
	c.JSON(200, M{"ok": true})
}
func validImageRef(s string) bool {
	return !strings.ContainsAny(s, "\\\r\n") && (s == "" || strings.HasPrefix(s, "/assets/") || strings.HasPrefix(s, "/media/"))
}
func (a *App) saveEntity(c *gin.Context) {
	var in struct {
		Kind        string `json:"kind"`
		Name        string `json:"name"`
		Description string `json:"description"`
		Cover       string `json:"cover"`
		Avatar      string `json:"avatar"`
		Featured    bool   `json:"featured"`
	}
	if c.ShouldBindJSON(&in) != nil || strings.TrimSpace(in.Name) == "" || len(in.Name) > 100 || len(in.Description) > 500 || !validCoverRef(in.Cover) || (!validImageRef(in.Avatar) && allowedMediaURL(in.Avatar) != nil) || (in.Kind != "topic" && in.Kind != "author" && in.Kind != "channel") {
		fail(c, 400, "名称、类型或图片地址不正确")
		return
	}
	eid := c.Param("id")
	if in.Kind == "channel" && eid != "x" {
		fail(c, 400, "首版仅支持 X 渠道")
		return
	}
	data := encode(M{"name": in.Name, "description": in.Description, "cover": in.Cover, "avatar": in.Avatar, "featured": in.Featured})
	if eid == "" {
		eid = id()
		_, e := a.db.Exec(c, "INSERT INTO entities(id,kind,data) VALUES($1,$2,$3)", eid, in.Kind, data)
		if !check(c, e) {
			return
		}
	} else {
		tag, e := a.db.Exec(c, "UPDATE entities SET data=data||$2::jsonb WHERE id=$1 AND kind=$3", eid, data, in.Kind)
		if !check(c, e) {
			return
		}
		if tag.RowsAffected() == 0 {
			fail(c, 404, "对象不存在或类型不一致")
			return
		}
	}
	c.JSON(200, M{"id": eid})
}
func (a *App) saveHomepage(c *gin.Context) {
	var in struct {
		Banners []struct {
			Title    string `json:"title"`
			Subtitle string `json:"subtitle"`
			Image    string `json:"image"`
			Href     string `json:"href"`
		} `json:"banners"`
	}
	if c.ShouldBindJSON(&in) != nil || len(in.Banners) > 6 {
		fail(c, 400, "最多配置 6 张横幅")
		return
	}
	for _, b := range in.Banners {
		u, e := url.Parse(b.Href)
		if len(b.Title) > 100 || len(b.Subtitle) > 200 || !validImageRef(b.Image) || b.Image == "" || e != nil || u.IsAbs() || !strings.HasPrefix(b.Href, "/") || strings.HasPrefix(b.Href, "//") {
			fail(c, 400, "横幅需使用站内图片和站内跳转链接")
			return
		}
	}
	_, e := a.db.Exec(c, "INSERT INTO settings(key,value) VALUES('homepage',$1) ON CONFLICT(key) DO UPDATE SET value=excluded.value,updated_at=now()", encode(in))
	if check(c, e) {
		c.JSON(200, M{"ok": true})
	}
}
func (a *App) listUsers(c *gin.Context) {
	items, e := rowsJSON(c, a.db, "SELECT jsonb_build_object('id',id,'username',username,'role',role,'disabled',disabled,'profile',profile,'created_at',created_at) FROM users WHERE ($1='' OR id>$1) ORDER BY id LIMIT 51", c.Query("cursor"))
	if !check(c, e) {
		return
	}
	next := ""
	if len(items) > 50 {
		items = items[:50]
		next = items[49]["id"].(string)
	}
	c.JSON(200, M{"items": items, "nextCursor": next})
}
func (a *App) editUser(c *gin.Context) {
	var in struct {
		Disabled bool `json:"disabled"`
	}
	if c.ShouldBindJSON(&in) != nil {
		fail(c, 400, "参数错误")
		return
	}
	tag, e := a.db.Exec(c, "UPDATE users SET disabled=$2 WHERE id=$1 AND role='user'", c.Param("id"), in.Disabled)
	if !check(c, e) {
		return
	}
	if tag.RowsAffected() == 0 {
		fail(c, 400, "只能修改普通用户状态")
		return
	}
	c.JSON(200, M{"ok": true})
}
func sourceFromJSON(b []byte) (Source, error) {
	var s Source
	e := json.Unmarshal(b, &s)
	if s.ID == "" {
		return s, fmt.Errorf("source missing")
	}
	return s, e
}
