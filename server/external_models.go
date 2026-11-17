package main

import (
	"context"
	"log"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var xrwLeadingNumber = regexp.MustCompile(`^[0-9]{1,4}-+`)
var xrwKoreanName = regexp.MustCompile(`^([A-Za-z][A-Za-z0-9]*(?:-[A-Za-z][A-Za-z0-9]*){0,3})-([\p{Hangul}]{2,6})(?:-|$)`)
var xrwIssueNumber = regexp.MustCompile(`(?i)^(?:NO\.?|VOL\.?)?[0-9]{2,8}[a-z]?$`)

var xrwModelAliases = map[string]string{
	"蠢沫沫chunmomo": "蠢沫沫", "星之迟迟hoshilily": "星之迟迟", "半半子banbanko": "半半子",
	"日奈娇rinaijiao": "日奈娇", "rinaijiao日奈娇": "日奈娇", "nyako喵子": "喵子",
	"雨波haneame": "雨波", "haneame雨波": "雨波", "柒柒不可爱": "柒柒要乖哦",
	"cos过期米线线喵": "过期米线线喵", "水淼": "水淼Aqua", "水淼aqua": "水淼Aqua",
	"日本性感萝莉byoru": "Byoru", "萝莉byoru": "Byoru", "byoru": "Byoru",
	"菌烨tako": "菌烨tako", "yuuhui玉汇": "Yuuhui玉汇",
}

func xrwPublisher(name string) bool {
	switch strings.ToLower(name) {
	case "xiuren", "xiuren秀人网", "秀人网", "youmi", "youmi尤蜜荟", "you wu", "youwu尤物馆", "尤物馆", "尤蜜荟", "imiss爱蜜社", "imiss", "爱蜜社", "miitao蜜桃社", "蜜桃社", "mygirl美媛馆", "美媛馆", "mfstar模范学院", "模范学院", "ugirls尤果网", "尤果网", "xiao yu", "xiaoyu", "语画界", "模物恋", "tuigirl推女郎", "tuigirl":
		return true
	}
	return false
}

func cleanXRWModel(name string) string {
	name = strings.Trim(strings.TrimSpace(name), "@[]【】()（）")
	if n := utf8.RuneCountInString(name); n < 2 || n > 26 {
		return ""
	}
	if r, _ := utf8.DecodeRuneInString(name); !unicode.IsLetter(r) {
		return ""
	}
	for _, part := range []string{"合集", "写真", "模特", "资源", "福利", "会员", "财团", "写真馆", "微密圈", "OnlyFans", "Cosplay", "ArtGravia", "PURE MEDIA", "Bimilstory", "DJAWA", "Espacia", "JVID", "LOOZY"} {
		if strings.Contains(strings.ToLower(name), strings.ToLower(part)) {
			return ""
		}
	}
	if xrwPublisher(name) {
		return ""
	}
	if alias := xrwModelAliases[strings.ToLower(name)]; alias != "" {
		return alias
	}
	return name
}

func xrwHasEastAsian(name string) bool {
	return strings.ContainsFunc(name, func(r rune) bool {
		return unicode.In(r, unicode.Han, unicode.Hangul, unicode.Hiragana, unicode.Katakana)
	})
}

func xrwModelCandidate(title string) (string, bool) {
	title = xrwLeadingNumber.ReplaceAllString(strings.TrimSpace(title), "")
	first, rest, found := strings.Cut(title, "-")
	if !found {
		return "", false
	}
	if xrwPublisher(first) {
		fields := strings.FieldsFunc(rest, func(r rune) bool { return r == '-' })
		for i := 0; i+1 < len(fields) && i < 2; i++ {
			if !xrwIssueNumber.MatchString(fields[i]) {
				break
			}
			if name := cleanXRWModel(fields[i+1]); xrwHasEastAsian(name) {
				return name, true
			}
		}
		return "", false
	}
	switch strings.ToLower(first) {
	case "cosplay", "coser", "cos", "萝莉cos", "福利cos":
		name, _, _ := strings.Cut(strings.TrimLeft(rest, "-"), "-")
		return cleanXRWModel(name), true
	}
	if match := xrwKoreanName.FindStringSubmatch(title); match != nil {
		return cleanXRWModel(strings.ReplaceAll(match[1], "-", " ") + " " + match[2]), true
	}
	name := cleanXRWModel(first)
	return name, xrwHasEastAsian(name) && (strings.HasPrefix(rest, "-") || strings.HasPrefix(title, "小律-Ritsu-"))
}

type xrwModelAlbum struct {
	id    string
	title string
	name  string
	known bool
}

func applyXRWModels(ctx context.Context, tx pgx.Tx) error {
	rows, err := tx.Query(ctx, `SELECT id,title FROM wallpapers WHERE channel_id=$1 ORDER BY id`, xrwChannel)
	if err != nil {
		return err
	}
	defer rows.Close()
	albums := []xrwModelAlbum{}
	counts := map[string]int{}
	confirmed := map[string]string{}
	for rows.Next() {
		var album xrwModelAlbum
		if err := rows.Scan(&album.id, &album.title); err != nil {
			return err
		}
		album.name, album.known = xrwModelCandidate(album.title)
		if album.name != "" {
			key := strings.ToLower(album.name)
			counts[key]++
			if album.known && confirmed[key] == "" {
				confirmed[key] = album.name
			}
		}
		albums = append(albums, album)
	}
	if err = rows.Err(); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `CREATE TEMP TABLE xrw_model_update(id text, model_names text[]) ON COMMIT DROP`); err != nil {
		return err
	}
	matched := 0
	_, err = tx.CopyFrom(ctx, pgx.Identifier{"xrw_model_update"}, []string{"id", "model_names"}, pgx.CopyFromSlice(len(albums), func(i int) ([]any, error) {
		album := albums[i]
		names := []string{}
		key := strings.ToLower(album.name)
		if name := confirmed[key]; name != "" {
			names = append(names, name)
		} else if counts[key] >= 3 && xrwHasEastAsian(album.name) {
			names = append(names, album.name)
		}
		if len(names) > 0 {
			matched++
		}
		return []any{album.id, names}, nil
	}))
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE wallpapers w SET model_names=m.model_names FROM xrw_model_update m
		WHERE w.id=m.id AND w.model_names IS DISTINCT FROM m.model_names`)
	if err == nil {
		log.Printf("xrw-album models: %d classified, %d unclassified", matched, len(albums)-matched)
		return groupXRWAlbums(ctx, tx)
	}
	return err
}

func classifyXRW(ctx context.Context, db *pgxpool.Pool) error {
	tx, err := db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(723019)`); err != nil {
		return err
	}
	if err = applyXRWModels(ctx, tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (a *App) xrwModels(c *gin.Context) {
	var count int64 = 1 << 62
	var after string
	if cursor := c.Query("cursor"); cursor != "" {
		value, name, ok := strings.Cut(cursor, ":")
		var err error
		count, err = strconv.ParseInt(value, 10, 64)
		if !ok || err != nil || count < 1 || name == "" {
			fail(c, 400, "分页参数无效")
			return
		}
		after = name
	}
	items, err := rowsJSON(c, a.db, `WITH models AS (
		SELECT name,count(DISTINCT COALESCE(NULLIF(w.album_group,''),w.id)) AS total,
		(array_agg(w.id ORDER BY w.published_at DESC,w.id DESC))[1] AS cover_album
		FROM wallpapers w CROSS JOIN LATERAL unnest(w.model_names) AS src(name)
		WHERE w.channel_id='xrw-album' AND w.status='published'
		AND ($3='' OR strpos(lower(name),lower($3))>0) AND ($4='' OR name=$4) GROUP BY name)
		SELECT jsonb_build_object('name',name,'count',total,'cover',COALESCE((SELECT m.external_url FROM media m
		WHERE m.wallpaper_id=cover_album ORDER BY m.position,m.id LIMIT 1),'')) FROM models
		WHERE total<$1 OR (total=$1 AND name COLLATE "C">$2)
		ORDER BY total DESC,name COLLATE "C" LIMIT 61`, count, after, strings.TrimSpace(c.Query("q")), c.Query("name"))
	if !check(c, err) {
		return
	}
	next := ""
	if len(items) > 60 {
		items = items[:60]
		next = strconv.FormatInt(int64(items[59]["count"].(float64)), 10) + ":" + items[59]["name"].(string)
	}
	c.JSON(200, M{"items": items, "nextCursor": next})
}
