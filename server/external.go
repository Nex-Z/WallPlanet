package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const xrwChannel = "xrw-album"
const xrwRepo = "https://github.com/hg3386628/xrw-album"
const xrwIndex = "https://raw.githubusercontent.com/hg3386628/xrw-album/main/data/photos/"

var xrwAlbumID = regexp.MustCompile(`^[a-z0-9]{4}-[a-f0-9]{10}$`)
var xrwImagePath = regexp.MustCompile(`^/file/[a-zA-Z0-9_-]+\.(?:jpe?g|png|webp)$`)

func validXRWImage(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Scheme == "https" && u.Host == "telegra.ph" && u.User == nil &&
		u.RawQuery == "" && u.Fragment == "" && xrwImagePath.MatchString(u.EscapedPath())
}

type xrwAlbum struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Photos []struct {
		ID  int    `json:"id"`
		URL string `json:"url"`
	} `json:"photos"`
}

func syncXRW(ctx context.Context, db *pgxpool.Pool, ids []string) error {
	if len(ids) == 0 {
		return errors.New("usage: wallplanet sync-xrw ALBUM_ID [ALBUM_ID ...]")
	}
	client := http.Client{Timeout: 20 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	albums := make([]xrwAlbum, 0, len(ids))
	for _, albumID := range ids {
		if !xrwAlbumID.MatchString(albumID) {
			return fmt.Errorf("invalid xrw-album ID: %q", albumID)
		}
		album, err := readXRWAlbum(ctx, &client, albumID)
		if err != nil {
			return fmt.Errorf("xrw-album %s: %w", albumID, err)
		}
		albums = append(albums, album)
	}
	return writeXRWAlbums(ctx, db, albums, time.Now())
}

func writeXRWAlbums(ctx context.Context, db *pgxpool.Pool, albums []xrwAlbum, publishedAt time.Time) error {
	tx, err := db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(723019)`); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO entities(id,kind,data) VALUES($1,'channel',$2) ON CONFLICT DO NOTHING`, xrwChannel,
		encode(M{"name": "xrw-album", "description": "来自 xrw-album 的外部相册索引"}))
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `CREATE TEMP TABLE xrw_works(id text, title text, source_url text) ON COMMIT DROP;
		CREATE TEMP TABLE xrw_photos(id text, wallpaper_id text, position integer, external_url text) ON COMMIT DROP`)
	if err != nil {
		return err
	}
	_, err = tx.CopyFrom(ctx, pgx.Identifier{"xrw_works"}, []string{"id", "title", "source_url"}, pgx.CopyFromSlice(len(albums), func(i int) ([]any, error) {
		album := albums[i]
		return []any{xrwChannel + "-" + album.ID, album.Title, xrwRepo + "/blob/main/data/photos/" + album.ID + ".json"}, nil
	}))
	if err != nil {
		return err
	}
	albumIndex, position := 0, 0
	photos, err := tx.CopyFrom(ctx, pgx.Identifier{"xrw_photos"}, []string{"id", "wallpaper_id", "position", "external_url"}, pgx.CopyFromFunc(func() ([]any, error) {
		if albumIndex == len(albums) {
			return nil, nil
		}
		album := albums[albumIndex]
		workID := xrwChannel + "-" + album.ID
		photo := album.Photos[position]
		row := []any{fmt.Sprintf("%s-%d", workID, photo.ID), workID, position, photo.URL}
		position++
		if position == len(album.Photos) {
			albumIndex++
			position = 0
		}
		return row, nil
	}))
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO wallpapers(id,source_id,channel_id,title,source_url,published_at)
		SELECT id,id,$1,title,source_url,$2 FROM xrw_works
		ON CONFLICT(id) DO UPDATE SET title=excluded.title,source_url=excluded.source_url`, xrwChannel, publishedAt)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO media(id,wallpaper_id,source_key,storage_key,thumbnail_key,width,height,bytes,mime,position,external_url)
		SELECT id,wallpaper_id,id,'','',0,0,0,'',position,external_url FROM xrw_photos
		ON CONFLICT(id) DO UPDATE SET position=excluded.position,external_url=excluded.external_url`)
	if err != nil {
		return err
	}
	if err = applyXRWModels(ctx, tx); err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return err
	}
	log.Printf("xrw-album index synced: %d albums, %d photos", len(albums), photos)
	return nil
}

func readXRWAlbum(ctx context.Context, client *http.Client, albumID string) (xrwAlbum, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, xrwIndex+albumID+".json", nil)
	if err != nil {
		return xrwAlbum{}, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return xrwAlbum{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return xrwAlbum{}, fmt.Errorf("index HTTP %d", resp.StatusCode)
	}
	return decodeXRWAlbum(resp.Body, albumID)
}

var xrwURLFixes = strings.NewReplacer(
	"https://telegra.phhttps://legra.ph/file/", "https://telegra.ph/file/",
	"https://telegra.phhttps//legra.ph/file/", "https://telegra.ph/file/",
)

func decodeXRWAlbum(reader io.Reader, albumID string) (xrwAlbum, error) {
	var album xrwAlbum
	if err := json.NewDecoder(io.LimitReader(reader, 1<<20)).Decode(&album); err != nil {
		return album, err
	}
	if album.ID != albumID || strings.TrimSpace(album.Title) == "" || len(album.Photos) == 0 {
		return album, errors.New("invalid album index")
	}
	seen := make(map[int]bool, len(album.Photos))
	for i := range album.Photos {
		photo := &album.Photos[i]
		photo.URL = xrwURLFixes.Replace(photo.URL)
		if photo.ID < 1 || seen[photo.ID] || !validXRWImage(photo.URL) {
			return album, fmt.Errorf("invalid image index entry %d", photo.ID)
		}
		seen[photo.ID] = true
	}
	return album, nil
}

func fetchXRWImage(ctx context.Context, raw string) ([]byte, string, error) {
	if !validXRWImage(raw) {
		return nil, "", errors.New("invalid external image URL")
	}
	transport, err := mediaTransport(env("MEDIA_PROXY_URL", ""))
	if err != nil {
		return nil, "", err
	}
	defer transport.CloseIdleConnections()
	client := http.Client{Timeout: 30 * time.Second, Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return nil, "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("image HTTP %d", resp.StatusCode)
	}
	const maxBytes = 25 << 20
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil || len(data) > maxBytes {
		return nil, "", errors.New("external image is unreadable or too large")
	}
	_, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || (format != "jpeg" && format != "png" && format != "webp") {
		return nil, "", errors.New("unsupported external image format")
	}
	return data, format, nil
}

func (a *App) downloadXRW(c *gin.Context, raw string) {
	data, format, err := fetchXRWImage(c.Request.Context(), raw)
	if err != nil {
		fail(c, http.StatusBadGateway, "外部原图暂不可下载")
		return
	}
	if !a.recordDownload(c) {
		return
	}
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Content-Disposition", `attachment; filename="wallplanet-`+c.Param("id")+`.`+format+`"`)
	c.Data(http.StatusOK, "image/"+format, data)
}
