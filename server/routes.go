package main

import (
	"github.com/gin-gonic/gin"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

func (a *App) routes() *gin.Engine {
	r := gin.New()
	r.Use(gin.Recovery())
	_ = r.SetTrustedProxies(nil)
	api := r.Group("/api/v1")
	api.Use(a.session)
	api.GET("/health", func(c *gin.Context) {
		if a.db.Ping(c) != nil {
			fail(c, 503, "数据库不可用")
			return
		}
		c.JSON(200, M{"status": "ok", "demo": a.demo})
	})
	api.GET("/auth/me", a.me)
	api.POST("/auth/register", a.auth)
	api.POST("/auth/login", a.auth)
	api.POST("/auth/logout", requireUser, a.logout)
	api.GET("/home", a.home)
	api.GET("/wallpapers", a.wallpapers)
	api.GET("/wallpapers/:id", a.wallpaper)
	api.PUT("/wallpapers/:id/interactions/:kind", requireUser, a.interaction)
	api.DELETE("/wallpapers/:id/interactions/:kind", requireUser, a.interaction)
	api.POST("/wallpapers/:id/history", requireUser, a.history)
	api.POST("/wallpapers/:id/download/:media", requireUser, a.download)
	api.GET("/entities", a.entities)
	api.GET("/entities/:id", a.entity)
	api.PUT("/entities/:id/subscription", requireUser, a.subscription)
	api.DELETE("/entities/:id/subscription", requireUser, a.subscription)
	api.GET("/ranks", a.ranks)
	api.PATCH("/me/profile", requireUser, a.profile)
	api.PUT("/me/password", requireUser, a.password)
	admin := api.Group("/admin", requireAdmin)
	admin.GET("/overview", a.adminOverview)
	admin.GET("/settings", a.adminSettings)
	admin.PUT("/settings", a.saveSettings)
	admin.GET("/sources", a.listSources)
	admin.POST("/sources", a.saveSource)
	admin.POST("/sources/preview", a.previewSource)
	admin.PUT("/sources/:id", a.saveSource)
	admin.POST("/sources/:id/run", a.enqueue)
	admin.GET("/jobs", a.listJobs)
	admin.GET("/jobs/:id/items", a.jobItems)
	admin.POST("/jobs/:id/items/:item/retry", a.retryJobItem)
	admin.POST("/jobs/:id/retry", a.retryJob)
	admin.POST("/jobs/:id/reconcile", a.reconcileJob)
	admin.GET("/wallpapers", a.adminWallpapers)
	admin.PATCH("/wallpapers/:id", a.editWallpaper)
	admin.POST("/entities", a.saveEntity)
	admin.PUT("/entities/:id", a.saveEntity)
	admin.PUT("/homepage", a.saveHomepage)
	admin.GET("/users", a.listUsers)
	admin.PATCH("/users/:id", a.editUser)
	api.GET("/openapi.json", func(c *gin.Context) { c.Data(200, "application/json", openAPI) })
	r.GET("/media/:key", a.media)
	webDir := env("WEB_DIR", "../web/dist")
	r.NoRoute(func(c *gin.Context) {
		p := c.Request.URL.Path
		if strings.HasPrefix(p, "/api/") || strings.HasPrefix(p, "/media/") {
			fail(c, 404, "接口不存在")
			return
		}
		if c.Request.Method != http.MethodGet {
			fail(c, 405, "不支持的请求")
			return
		}
		clean := filepath.Clean(strings.TrimPrefix(p, "/"))
		if clean == ".." || strings.HasPrefix(clean, ".."+string(os.PathSeparator)) {
			fail(c, 404, "文件不存在")
			return
		}
		file := filepath.Join(webDir, clean)
		if st, e := os.Stat(file); e == nil && !st.IsDir() {
			c.Status(200)
			c.File(file)
			return
		}
		if clean != "." && filepath.Ext(clean) != "" {
			fail(c, 404, "文件不存在")
			return
		}
		c.Status(200)
		c.File(filepath.Join(webDir, "index.html"))
	})
	return r
}
