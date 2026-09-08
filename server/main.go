package main

import (
	"context"
	"crypto/rand"
	_ "embed"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
)

//go:embed migrations/001_initial.sql
var migration string

//go:embed migrations/002_integrity.sql
var integrityMigration string

//go:embed migrations/003_item_retry.sql
var itemRetryMigration string

//go:embed migrations/004_job_snapshot.sql
var jobSnapshotMigration string

//go:embed migrations/005_batch_actor.sql
var batchActorMigration string

type M = map[string]any
type App struct {
	db            *pgxpool.Pool
	key           []byte
	origin        string
	secure        bool
	demo          bool
	storage       Storage
	limiter       *Limiter
	http          *http.Client
	fetchOverride func(context.Context, string) (StoredImage, error)
}

func env(k, d string) string {
	if s := os.Getenv(k); s != "" {
		return s
	}
	return d
}
func id() string {
	b := make([]byte, 16)
	if _, e := rand.Read(b); e != nil {
		panic(e)
	}
	return hex.EncodeToString(b)
}
func encode(v any) []byte                         { b, _ := json.Marshal(v); return b }
func fail(c *gin.Context, status int, msg string) { c.AbortWithStatusJSON(status, M{"error": msg}) }
func check(c *gin.Context, e error) bool {
	if e != nil {
		if errors.Is(e, pgx.ErrNoRows) {
			fail(c, 404, "内容不存在或已下架")
		} else {
			log.Printf("database operation failed: %T", e)
			fail(c, 500, "服务暂时不可用，请稍后重试")
		}
		return false
	}
	return true
}
func userID(c *gin.Context) string { return c.GetString("user_id") }
func rowsJSON(ctx context.Context, db *pgxpool.Pool, q string, args ...any) ([]M, error) {
	rows, e := db.Query(ctx, q, args...)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []M{}
	for rows.Next() {
		var raw []byte
		if e = rows.Scan(&raw); e != nil {
			return nil, e
		}
		var m M
		if e = json.Unmarshal(raw, &m); e != nil {
			return nil, e
		}
		out = append(out, m)
	}
	return out, rows.Err()
}
func migrate(ctx context.Context, db *pgxpool.Pool) error {
	tx, e := db.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	if _, e = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(723018)"); e != nil {
		return e
	}
	if _, e = tx.Exec(ctx, "CREATE TABLE IF NOT EXISTS schema_migrations(version integer PRIMARY KEY,applied_at timestamptz NOT NULL DEFAULT now())"); e != nil {
		return e
	}
	for i, sql := range []string{migration, integrityMigration, itemRetryMigration, jobSnapshotMigration, batchActorMigration} {
		var applied bool
		if e = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version=$1)", i+1).Scan(&applied); e != nil {
			return e
		}
		if applied {
			continue
		}
		if _, e = tx.Exec(ctx, sql); e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, "INSERT INTO schema_migrations(version) VALUES($1)", i+1); e != nil {
			return e
		}
	}
	return tx.Commit(ctx)
}
func initDB(ctx context.Context, dsn string) error {
	u, e := url.Parse(dsn)
	if e != nil {
		return e
	}
	name := strings.TrimPrefix(u.Path, "/")
	if name != "wallplanet" && name != "wallplanet_demo" {
		return fmt.Errorf("init-db only accepts wallplanet or wallplanet_demo")
	}
	u.Path = "/postgres"
	db, e := pgx.Connect(ctx, u.String())
	if e != nil {
		return errors.New("cannot connect to PostgreSQL administrative database")
	}
	defer db.Close(ctx)
	var exists bool
	if e = db.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname=$1)", name).Scan(&exists); e != nil {
		return e
	}
	if !exists {
		_, e = db.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize())
	}
	return e
}
func main() {
	_ = godotenv.Load("../.env", ".env")
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	demo := env("DEMO_MODE", "false") == "true"
	dsn := os.Getenv("DATABASE_URL")
	if demo {
		dsn = os.Getenv("DEMO_DATABASE_URL")
		u, _ := url.Parse(dsn)
		if u == nil || u.Path != "/wallplanet_demo" {
			log.Fatal("DEMO_MODE requires a separate wallplanet_demo database")
		}
	}
	if dsn == "" {
		log.Fatal("DATABASE_URL is required")
	}
	command := "serve"
	if len(os.Args) > 1 {
		command = os.Args[1]
	}
	if command == "init-db" {
		if e := initDB(ctx, dsn); e != nil {
			log.Fatal(e)
		}
		log.Print("database ready")
		return
	}
	db, e := pgxpool.New(ctx, dsn)
	if e != nil {
		log.Fatal("invalid database configuration")
	}
	defer db.Close()
	if e = db.Ping(ctx); e != nil {
		log.Fatal("cannot connect to database")
	}
	if e = migrate(ctx, db); e != nil {
		log.Fatal(e)
	}
	if command == "migrate" {
		log.Print("migrations applied")
		return
	}
	if command == "create-admin" {
		if e = createAdmin(ctx, db); e != nil {
			log.Fatal(e)
		}
		log.Print("administrator created")
		return
	}
	if command == "seed-demo" {
		if !demo {
			log.Fatal("seed-demo requires DEMO_MODE=true")
		}
		if e = seedDemo(ctx, db); e != nil {
			log.Fatal(e)
		}
		log.Print("demo data ready")
		return
	}
	key, e := base64.StdEncoding.DecodeString(os.Getenv("MASTER_KEY"))
	if e != nil || len(key) != 32 {
		log.Fatal("MASTER_KEY must be 32 random bytes encoded as base64")
	}
	dataDir := env("DATA_DIR", "../data")
	if demo {
		dataDir = filepath.Join(dataDir, "demo")
	}
	if e = os.MkdirAll(dataDir, 0700); e != nil {
		log.Fatal(e)
	}
	a := &App{db: db, key: key, origin: env("APP_ORIGIN", "http://localhost:5173"), secure: env("COOKIE_SECURE", "false") == "true", demo: demo, storage: &LocalStorage{Root: dataDir}, limiter: NewLimiter(), http: &http.Client{Timeout: 45 * time.Second}}
	gin.SetMode(gin.ReleaseMode)
	r := a.routes()
	srv := &http.Server{Addr: env("APP_ADDR", "127.0.0.1:8080"), Handler: r, ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 60 * time.Second}
	go a.worker(ctx)
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdown)
	}()
	log.Printf("WallPlanet listening on %s (demo=%t)", srv.Addr, demo)
	if e = srv.ListenAndServe(); e != nil && e != http.ErrServerClosed {
		log.Fatal(e)
	}
}
