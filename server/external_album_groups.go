package main

import (
	"context"
	"log"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
)

var xrwAlbumSuffix = regexp.MustCompile(`(?i)(?:-(?:0[1-9]|1[0-2])-(?:0[1-9]|[12][0-9]|3[01])(?:-[0-9]{1,2})?|-(?:19|20)[0-9]{6}[a-z]?|[- _]+(?:set|part|pt)[- _]*[0-9]{1,3}|(?:[- ]*[0-9]+(?:[.,][0-9]+)?[- ]*(?:gb|mb|kb|p|v|g|m))+)$`)

func xrwAlbumStem(title, model string) string {
	stem := xrwLeadingNumber.ReplaceAllString(strings.TrimSpace(title), "")
	for {
		shorter := strings.Trim(xrwAlbumSuffix.ReplaceAllString(stem, ""), "- _")
		if shorter == stem {
			break
		}
		stem = shorter
	}
	parts := strings.FieldsFunc(stem, func(r rune) bool { return r == '-' || r == '_' })
	if len(parts) > 0 {
		switch strings.ToLower(parts[0]) {
		case "cosplay", "coser", "cos", "萝莉cos", "福利cos":
			parts = parts[1:]
		}
	}
	if len(parts) > 0 && strings.EqualFold(cleanXRWModel(parts[0]), model) {
		parts = parts[1:]
	}
	stem = strings.Join(strings.Fields(strings.Join(parts, " ")), " ")
	if !strings.ContainsFunc(stem, unicode.IsLetter) || utf8.RuneCountInString(stem) < 2 {
		return ""
	}
	switch strings.ToLower(stem) {
	case "写真", "合集", "套图", "预览", "日常", "自拍", "女仆", "泳装", "黑丝", "白丝", "婚纱", "cosplay", "photos", "photobook":
		return ""
	}
	return stem
}

func groupXRWAlbums(ctx context.Context, tx pgx.Tx) error {
	rows, err := tx.Query(ctx, `SELECT id,title,COALESCE(model_names[1],''),created_at FROM wallpapers WHERE channel_id='xrw-album' ORDER BY id`)
	if err != nil {
		return err
	}
	defer rows.Close()
	type album struct{ id, key string }
	type group struct {
		id, title string
		count     int
		firstSeen time.Time
	}
	albums := []album{}
	groups := map[string]group{}
	for rows.Next() {
		var id, title, model string
		var createdAt time.Time
		if err = rows.Scan(&id, &title, &model, &createdAt); err != nil {
			return err
		}
		a := album{id: id}
		if model != "" {
			if stem := xrwAlbumStem(title, model); stem != "" {
				a.key = model + "\x00" + strings.ToLower(stem)
				g := groups[a.key]
				if g.count == 0 {
					g.id, g.title = id, stem
				}
				if g.firstSeen.IsZero() || createdAt.Before(g.firstSeen) {
					g.firstSeen = createdAt
				}
				g.count++
				groups[a.key] = g
			}
		}
		albums = append(albums, a)
	}
	if err = rows.Err(); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `CREATE TEMP TABLE xrw_album_update(id text, album_group text, album_title text, archived_at timestamptz) ON COMMIT DROP`); err != nil {
		return err
	}
	merged := 0
	_, err = tx.CopyFrom(ctx, pgx.Identifier{"xrw_album_update"}, []string{"id", "album_group", "album_title", "archived_at"}, pgx.CopyFromSlice(len(albums), func(i int) ([]any, error) {
		a := albums[i]
		if g := groups[a.key]; g.count > 1 {
			if a.id != g.id {
				merged++
			}
			return []any{a.id, g.id, g.title, g.firstSeen}, nil
		}
		return []any{a.id, "", "", nil}, nil
	}))
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE wallpapers w SET album_group=m.album_group, album_title=m.album_title, album_archived_at=m.archived_at
		FROM xrw_album_update m WHERE w.id=m.id AND (w.album_group,w.album_title,w.album_archived_at) IS DISTINCT FROM (m.album_group,m.album_title,m.archived_at)`)
	if err == nil {
		log.Printf("xrw-album grouping: %d upstream entries reorganized into existing albums", merged)
	}
	return err
}
