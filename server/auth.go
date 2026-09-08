package main

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/argon2"
)

func hashPassword(p string) string {
	salt := make([]byte, 16)
	_, _ = rand.Read(salt)
	h := argon2.IDKey([]byte(p), salt, 2, 64*1024, 2, 32)
	return base64.RawStdEncoding.EncodeToString(salt) + "." + base64.RawStdEncoding.EncodeToString(h)
}
func verifyPassword(p, h string) bool {
	parts := strings.Split(h, ".")
	if len(parts) != 2 {
		return false
	}
	salt, e := base64.RawStdEncoding.DecodeString(parts[0])
	if e != nil || len(salt) != 16 {
		return false
	}
	expected, e := base64.RawStdEncoding.DecodeString(parts[1])
	if e != nil || len(expected) != 32 {
		return false
	}
	actual := argon2.IDKey([]byte(p), salt, 2, 64*1024, 2, 32)
	return subtle.ConstantTimeCompare(actual, expected) == 1
}
func tokenHash(s string) string { h := sha256.Sum256([]byte(s)); return hex.EncodeToString(h[:]) }
func encrypt(key []byte, s string) (string, error) {
	block, e := aes.NewCipher(key)
	if e != nil {
		return "", e
	}
	g, e := cipher.NewGCM(block)
	if e != nil {
		return "", e
	}
	n := make([]byte, g.NonceSize())
	if _, e = rand.Read(n); e != nil {
		return "", e
	}
	return base64.StdEncoding.EncodeToString(g.Seal(n, n, []byte(s), nil)), nil
}
func decrypt(key []byte, s string) (string, error) {
	b, e := base64.StdEncoding.DecodeString(s)
	if e != nil {
		return "", e
	}
	block, e := aes.NewCipher(key)
	if e != nil {
		return "", e
	}
	g, e := cipher.NewGCM(block)
	if e != nil || len(b) < g.NonceSize() {
		return "", errors.New("invalid ciphertext")
	}
	plain, e := g.Open(nil, b[:g.NonceSize()], b[g.NonceSize():], nil)
	return string(plain), e
}

type limitEntry struct {
	count int
	until time.Time
}
type Limiter struct {
	mu      sync.Mutex
	entries map[string]limitEntry
}

func NewLimiter() *Limiter { return &Limiter{entries: map[string]limitEntry{}} }
func (l *Limiter) Allow(key string, max int, d time.Duration) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	if len(l.entries) > 10000 {
		for k, v := range l.entries {
			if now.After(v.until) {
				delete(l.entries, k)
			}
		}
	}
	v := l.entries[key]
	if now.After(v.until) {
		v = limitEntry{until: now.Add(d)}
	}
	v.count++
	l.entries[key] = v
	return v.count <= max
}
func createAdmin(ctx context.Context, db *pgxpool.Pool) error {
	u, p := os.Getenv("ADMIN_USERNAME"), os.Getenv("ADMIN_PASSWORD")
	if len(u) < 3 || len(u) > 32 || len(p) < 10 || len(p) > 128 {
		return errors.New("set ADMIN_USERNAME (3-32 characters) and ADMIN_PASSWORD (10-128 characters)")
	}
	_, e := db.Exec(ctx, "INSERT INTO users(id,username,password_hash,role,profile) VALUES($1,$2,$3,'admin',$4)", id(), u, hashPassword(p), encode(M{"name": u}))
	return e
}
func (a *App) session(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1<<20)
	if cookie, e := c.Cookie(a.cookieName()); e == nil {
		var uid, role, csrf string
		e = a.db.QueryRow(c, "SELECT u.id,u.role,s.csrf FROM sessions s JOIN users u ON u.id=s.user_id WHERE s.token_hash=$1 AND s.expires_at>now() AND NOT u.disabled", tokenHash(cookie)).Scan(&uid, &role, &csrf)
		if e == nil {
			c.Set("user_id", uid)
			c.Set("role", role)
			c.Set("csrf", csrf)
		}
	}
	if c.Request.Method != "GET" && c.Request.Method != "HEAD" && c.Request.Method != "OPTIONS" {
		scheme := "http://"
		if a.secure {
			scheme = "https://"
		}
		if origin := c.GetHeader("Origin"); origin != "" && origin != a.origin && origin != scheme+c.Request.Host {
			fail(c, 403, "请求来源不受信任")
			return
		}
		if userID(c) != "" && subtle.ConstantTimeCompare([]byte(c.GetHeader("X-CSRF-Token")), []byte(c.GetString("csrf"))) != 1 {
			fail(c, 403, "会话校验失败，请刷新页面")
			return
		}
	}
	c.Next()
}
func requireUser(c *gin.Context) {
	if userID(c) == "" {
		fail(c, 401, "请先登录")
		return
	}
	c.Next()
}
func requireAdmin(c *gin.Context) {
	if userID(c) == "" {
		fail(c, 401, "请先登录")
		return
	}
	if c.GetString("role") != "admin" {
		fail(c, 403, "需要管理员权限")
		return
	}
	c.Next()
}
func (a *App) auth(c *gin.Context) {
	if !a.limiter.Allow("auth:"+c.ClientIP(), 15, 15*time.Minute) {
		fail(c, 429, "尝试过于频繁，请稍后再试")
		return
	}
	var in struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if c.ShouldBindJSON(&in) != nil {
		fail(c, 400, "请输入账号和密码")
		return
	}
	in.Username = strings.TrimSpace(in.Username)
	if len([]rune(in.Username)) < 3 || len(in.Username) > 32 || len(in.Password) < 10 || len(in.Password) > 128 {
		fail(c, 400, "账号需 3–32 字符，密码需 10–128 字符")
		return
	}
	var uid string
	if strings.HasSuffix(c.FullPath(), "register") {
		uid = id()
		tag, e := a.db.Exec(c, "INSERT INTO users(id,username,password_hash,profile) VALUES($1,$2,$3,$4) ON CONFLICT(username) DO NOTHING", uid, in.Username, hashPassword(in.Password), encode(M{"name": in.Username, "bio": "", "avatar": "", "cover": ""}))
		if !check(c, e) {
			return
		}
		if tag.RowsAffected() == 0 {
			fail(c, 409, "这个账号已被使用")
			return
		}
	} else {
		var h string
		var disabled bool
		e := a.db.QueryRow(c, "SELECT id,password_hash,disabled FROM users WHERE username=$1", in.Username).Scan(&uid, &h, &disabled)
		if e != nil {
			h = hashPassword("constant-time-dummy")
		}
		if !verifyPassword(in.Password, h) || disabled || e != nil {
			fail(c, 401, "账号或密码不正确")
			return
		}
	}
	token, csrf := id()+id(), id()
	_, e := a.db.Exec(c, "INSERT INTO sessions(token_hash,user_id,csrf,expires_at) VALUES($1,$2,$3,$4)", tokenHash(token), uid, csrf, time.Now().Add(30*24*time.Hour))
	if !check(c, e) {
		return
	}
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(a.cookieName(), token, 30*86400, "/", "", a.secure, true)
	c.JSON(200, M{"csrf": csrf})
}
func (a *App) me(c *gin.Context) {
	if userID(c) == "" {
		c.JSON(200, M{"user": nil, "csrf": "", "demo": a.demo})
		return
	}
	var raw []byte
	e := a.db.QueryRow(c, `SELECT jsonb_build_object('id',u.id,'username',u.username,'role',u.role,'profile',u.profile,'stats',jsonb_build_object('favorites',(SELECT count(*) FROM interactions WHERE user_id=u.id AND kind='favorite'),'downloads',(SELECT count(DISTINCT wallpaper_id) FROM downloads WHERE user_id=u.id),'subscriptions',(SELECT count(*) FROM subscriptions WHERE user_id=u.id),'history',(SELECT count(*) FROM history WHERE user_id=u.id))) FROM users u WHERE u.id=$1`, userID(c)).Scan(&raw)
	if !check(c, e) {
		return
	}
	c.Data(200, "application/json", []byte(`{"user":`+string(raw)+`,"csrf":"`+c.GetString("csrf")+`","demo":`+map[bool]string{true: "true", false: "false"}[a.demo]+`}`))
}
func (a *App) logout(c *gin.Context) {
	cookie, _ := c.Cookie(a.cookieName())
	_, e := a.db.Exec(c, "DELETE FROM sessions WHERE token_hash=$1", tokenHash(cookie))
	if !check(c, e) {
		return
	}
	c.SetCookie(a.cookieName(), "", -1, "/", "", a.secure, true)
	c.JSON(200, M{"ok": true})
}
func (a *App) profile(c *gin.Context) {
	var p struct {
		Name   string `json:"name"`
		Bio    string `json:"bio"`
		Avatar string `json:"avatar"`
		Cover  string `json:"cover"`
	}
	if c.ShouldBindJSON(&p) != nil || len([]rune(p.Name)) > 40 || len([]rune(p.Bio)) > 200 || p.Name == "" {
		fail(c, 400, "昵称不能为空，简介不能超过 200 字")
		return
	}
	for _, s := range []string{p.Avatar, p.Cover} {
		if s != "" && !strings.HasPrefix(s, "/media/") && !strings.HasPrefix(s, "/assets/") {
			fail(c, 400, "请选择站内图片")
			return
		}
	}
	_, e := a.db.Exec(c, "UPDATE users SET profile=$2 WHERE id=$1", userID(c), encode(M{"name": p.Name, "bio": p.Bio, "avatar": p.Avatar, "cover": p.Cover}))
	if check(c, e) {
		c.JSON(200, M{"ok": true})
	}
}
func (a *App) password(c *gin.Context) {
	var in struct {
		Current  string `json:"current"`
		Password string `json:"password"`
	}
	if c.ShouldBindJSON(&in) != nil || len(in.Password) < 10 || len(in.Password) > 128 {
		fail(c, 400, "新密码需 10–128 字符")
		return
	}
	var h string
	if !check(c, a.db.QueryRow(c, "SELECT password_hash FROM users WHERE id=$1", userID(c)).Scan(&h)) {
		return
	}
	if !verifyPassword(in.Current, h) {
		fail(c, 400, "当前密码不正确")
		return
	}
	tx, e := a.db.Begin(c)
	if !check(c, e) {
		return
	}
	defer tx.Rollback(c)
	_, e = tx.Exec(c, "UPDATE users SET password_hash=$2 WHERE id=$1", userID(c), hashPassword(in.Password))
	if !check(c, e) {
		return
	}
	_, e = tx.Exec(c, "DELETE FROM sessions WHERE user_id=$1", userID(c))
	if !check(c, e) {
		return
	}
	if check(c, tx.Commit(c)) {
		c.SetCookie(a.cookieName(), "", -1, "/", "", a.secure, true)
		c.JSON(200, M{"ok": true})
	}
}

func (a *App) cookieName() string {
	if a.demo {
		return "wallplanet_demo_session"
	}
	return "wallplanet_session"
}
