package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/gin-gonic/gin"
	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
	"image"
	"image/jpeg"
	_ "image/png"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type StoredImage struct {
	Key       string
	Thumbnail string
	Width     int
	Height    int
	Bytes     int64
	Mime      string
}
type Storage interface {
	Save(context.Context, []byte) (StoredImage, error)
	Path(string) (string, error)
}
type LocalStorage struct{ Root string }

func (s *LocalStorage) Path(key string) (string, error) {
	if key == "" || filepath.Base(key) != key || strings.ContainsAny(key, "/\\:") {
		return "", errors.New("invalid storage key")
	}
	return filepath.Join(s.Root, key), nil
}
func atomicFile(path string, b []byte) error {
	if _, e := os.Stat(path); e == nil {
		return nil
	}
	f, e := os.CreateTemp(filepath.Dir(path), ".write-*")
	if e != nil {
		return e
	}
	temp := f.Name()
	defer os.Remove(temp)
	if _, e = f.Write(b); e != nil {
		f.Close()
		return e
	}
	if e = f.Close(); e != nil {
		return e
	}
	return os.Rename(temp, path)
}
func (s *LocalStorage) Save(ctx context.Context, b []byte) (StoredImage, error) {
	var out StoredImage
	if len(b) > 25<<20 {
		return out, errors.New("图片超过 25 MB")
	}
	if e := ctx.Err(); e != nil {
		return out, e
	}
	cfg, format, e := image.DecodeConfig(bytes.NewReader(b))
	if e != nil {
		return out, errors.New("不支持的图片格式")
	}
	if format != "jpeg" && format != "png" && format != "webp" {
		return out, errors.New("仅支持静态 JPEG、PNG、WebP")
	}
	if format == "png" && bytes.Contains(b, []byte("acTL")) || format == "webp" && bytes.Contains(b, []byte("ANIM")) {
		return out, errors.New("跳过动态图")
	}
	if int64(cfg.Width)*int64(cfg.Height) > 80_000_000 || cfg.Width < 1 || cfg.Height < 1 {
		return out, errors.New("图片像素数异常")
	}
	h := sha256.Sum256(b)
	key := hex.EncodeToString(h[:])
	out = StoredImage{Key: key + "." + format, Thumbnail: key + "-thumb.jpg", Width: cfg.Width, Height: cfg.Height, Bytes: int64(len(b)), Mime: "image/" + format}
	if format == "jpeg" {
		out.Mime = "image/jpeg"
	}
	if e = os.MkdirAll(s.Root, 0700); e != nil {
		return out, e
	}
	p, _ := s.Path(out.Key)
	if e = atomicFile(p, b); e != nil {
		return out, e
	}
	thumbPath, _ := s.Path(out.Thumbnail)
	if _, e = os.Stat(thumbPath); e == nil {
		return out, nil
	}
	im, _, e := image.Decode(bytes.NewReader(b))
	if e != nil {
		return out, e
	}
	w, hg := cfg.Width, cfg.Height
	if w > 720 {
		hg = hg * 720 / w
		w = 720
	}
	if hg < 1 {
		hg = 1
	}
	dst := image.NewRGBA(image.Rect(0, 0, w, hg))
	draw.CatmullRom.Scale(dst, dst.Bounds(), im, im.Bounds(), draw.Over, nil)
	var buf bytes.Buffer
	if e = jpeg.Encode(&buf, dst, &jpeg.Options{Quality: 85}); e != nil {
		return out, e
	}
	return out, atomicFile(thumbPath, buf.Bytes())
}
func allowedMediaURL(raw string) error {
	u, e := url.Parse(raw)
	if e != nil || u.Scheme != "https" || u.Hostname() != "pbs.twimg.com" || u.User != nil || (u.Port() != "" && u.Port() != "443") {
		return errors.New("图片来源必须是 pbs.twimg.com 的 HTTPS 地址")
	}
	return nil
}
func publicDial(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, e := net.SplitHostPort(address)
	if e != nil {
		return nil, e
	}
	ips, e := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	if e != nil {
		return nil, e
	}
	for _, ip := range ips {
		if !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
			return nil, errors.New("private media destination blocked")
		}
	}
	d := net.Dialer{Timeout: 15 * time.Second}
	for _, ip := range ips {
		conn, e := d.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
		if e == nil {
			return conn, nil
		}
	}
	return nil, errors.New("cannot connect to image host")
}
func mediaTransport(proxyRaw string) (*http.Transport, error) {
	transport := &http.Transport{DialContext: publicDial, TLSHandshakeTimeout: 15 * time.Second}
	if proxyRaw == "" {
		return transport, nil
	}
	proxy, err := url.Parse(proxyRaw)
	if err != nil || (proxy.Scheme != "http" && proxy.Scheme != "https") || proxy.Hostname() == "" || proxy.RawQuery != "" || proxy.Fragment != "" || (proxy.Path != "" && proxy.Path != "/") {
		return nil, errors.New("MEDIA_PROXY_URL 配置无效")
	}
	// This is an operator-configured trusted proxy, never a URL from a post.
	// The proxy resolves the allowlisted image host; direct requests retain
	// public-IP pinning. Every destination and redirect is still validated.
	transport.Proxy = http.ProxyURL(proxy)
	transport.DialContext = (&net.Dialer{Timeout: 15 * time.Second}).DialContext
	return transport, nil
}

func (a *App) fetchImage(ctx context.Context, raw string) (StoredImage, error) {
	if e := allowedMediaURL(raw); e != nil {
		return StoredImage{}, e
	}
	u, _ := url.Parse(raw)
	if strings.HasPrefix(u.Path, "/media/") {
		q := u.Query()
		q.Set("name", "orig")
		u.RawQuery = q.Encode()
	}
	transport, e := mediaTransport(env("MEDIA_PROXY_URL", ""))
	if e != nil {
		return StoredImage{}, e
	}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 45 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 3 {
			return errors.New("too many redirects")
		}
		return allowedMediaURL(req.URL.String())
	}}
	req, _ := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	resp, e := client.Do(req)
	if e != nil {
		return StoredImage{}, errors.New("图片连接失败")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return StoredImage{}, fmt.Errorf("图片 HTTP %d", resp.StatusCode)
	}
	b, e := io.ReadAll(io.LimitReader(resp.Body, (25<<20)+1))
	if e != nil {
		return StoredImage{}, errors.New("图片读取失败")
	}
	return a.storage.Save(ctx, b)
}
func (a *App) media(c *gin.Context) {
	key := c.Param("key")
	path, e := a.storage.Path(key)
	if e != nil {
		fail(c, 404, "图片不存在")
		return
	}
	var exists bool
	if !check(c, a.db.QueryRow(c, "SELECT EXISTS(SELECT 1 FROM media m JOIN wallpapers w ON w.id=m.wallpaper_id WHERE (m.storage_key=$1 OR m.thumbnail_key=$1) AND w.status='published')", key).Scan(&exists)) {
		return
	}
	if !exists {
		fail(c, 404, "图片不存在或已下架")
		return
	}
	if _, e = os.Stat(path); e != nil {
		fail(c, 404, "图片文件暂不可用")
		return
	}
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Cache-Control", "public, max-age=300")
	c.File(path)
}
func (a *App) download(c *gin.Context) {
	if !a.limiter.Allow("download:"+userID(c), 30, time.Minute) {
		fail(c, 429, "下载过于频繁，请稍后重试")
		return
	}
	var key, mime string
	e := a.db.QueryRow(c, "SELECT m.storage_key,m.mime FROM media m JOIN wallpapers w ON w.id=m.wallpaper_id WHERE m.id=$1 AND w.id=$2 AND w.status='published'", c.Param("media"), c.Param("id")).Scan(&key, &mime)
	if !check(c, e) {
		return
	}
	path, e := a.storage.Path(key)
	if !check(c, e) {
		return
	}
	file, e := os.Open(path)
	if e != nil {
		fail(c, 404, "原图文件暂不可用")
		return
	}
	defer file.Close()
	st, e := file.Stat()
	if !check(c, e) {
		return
	}
	_, e = a.db.Exec(c, "INSERT INTO downloads(id,user_id,wallpaper_id,media_id) VALUES($1,$2,$3,$4)", id(), userID(c), c.Param("id"), c.Param("media"))
	if !check(c, e) {
		return
	}
	c.Header("Content-Disposition", `attachment; filename="wallplanet-`+c.Param("id")+filepath.Ext(key)+`"`)
	c.DataFromReader(200, st.Size(), mime, file, nil)
}
