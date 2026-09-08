package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"log"
	"net/url"
	"strings"
	"time"
)

func (a *App) refreshRanks(ctx context.Context) error {
	tx, e := a.db.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	for _, period := range []string{"7d", "30d", "all"} {
		since := time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)
		if period == "7d" {
			since = time.Now().AddDate(0, 0, -7)
		}
		if period == "30d" {
			since = time.Now().AddDate(0, 0, -30)
		}
		_, e = tx.Exec(ctx, `INSERT INTO rank_snapshots(period,wallpaper_id,score,updated_at)
 SELECT $1,w.id,COALESCE(i.score,0)+COALESCE(d.score,0),now() FROM wallpapers w
 LEFT JOIN (SELECT wallpaper_id,sum(CASE WHEN kind='favorite' THEN 4 ELSE 1 END) AS score FROM interactions WHERE created_at>=$2 GROUP BY wallpaper_id) i ON i.wallpaper_id=w.id
 LEFT JOIN (SELECT wallpaper_id,count(*)*2 AS score FROM (SELECT DISTINCT user_id,wallpaper_id,(created_at AT TIME ZONE 'UTC')::date FROM downloads WHERE created_at>=$2) distinct_downloads GROUP BY wallpaper_id) d ON d.wallpaper_id=w.id
 ON CONFLICT(period,wallpaper_id) DO UPDATE SET score=excluded.score,updated_at=excluded.updated_at`, period, since)
		if e != nil {
			return e
		}
	}
	return tx.Commit(ctx)
}
func (a *App) worker(ctx context.Context) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	lastRank := time.Time{}
	for {
		if ctx.Err() != nil {
			return
		}
		if time.Since(lastRank) > time.Hour {
			if e := a.refreshRanks(ctx); e != nil {
				log.Print("rank refresh failed")
			} else {
				lastRank = time.Now()
			}
			_, _ = a.db.Exec(ctx, "DELETE FROM sessions WHERE expires_at<now()")
		}
		if !a.demo {
			if e := a.tick(ctx); e != nil && ctx.Err() == nil {
				log.Printf("collector: %s", e)
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
func (a *App) tick(ctx context.Context) error {
	token, e := a.apifyToken(ctx)
	if e != nil {
		return nil
	}
	conn, e := a.db.Acquire(ctx)
	if e != nil {
		return e
	}
	defer conn.Release()
	var locked bool
	if e = conn.QueryRow(ctx, "SELECT pg_try_advisory_lock(723019)").Scan(&locked); e != nil || !locked {
		return e
	}
	defer func() {
		release, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = conn.Exec(release, "SELECT pg_advisory_unlock(723019)")
	}()
	// A durable 'starting' record left by a previous process is never blindly submitted again.
	if _, e = a.db.Exec(ctx, "UPDATE jobs SET status='uncertain',error='启动结果不明，正在核对 Apify 运行记录' WHERE status='starting'"); e != nil {
		return e
	}
	var jid, status string
	var runID, dataset *string
	var sourceRaw, inputRaw []byte
	var offset int
	var started *time.Time
	e = a.db.QueryRow(ctx, "SELECT j.id,j.status,j.run_id,j.dataset_id,j.dataset_offset,COALESCE(j.source_snapshot,to_jsonb(s)),j.input,j.started_at FROM jobs j JOIN sources s ON s.id=j.source_id WHERE j.status IN ('uncertain','running','importing','retrying') ORDER BY j.created_at LIMIT 1").Scan(&jid, &status, &runID, &dataset, &offset, &sourceRaw, &inputRaw, &started)
	if e != nil && !errors.Is(e, pgx.ErrNoRows) {
		return e
	}
	if e == nil {
		s, e := sourceFromJSON(sourceRaw)
		if e != nil {
			return e
		}
		switch status {
		case "uncertain":
			return a.reconcileAutomatic(ctx, token, jid, inputRaw, started)
		case "running":
			if runID == nil {
				return errors.New("running job missing run ID")
			}
			run, e := a.getRun(ctx, token, *runID)
			if e != nil {
				return a.jobError(ctx, jid, e)
			}
			if run.Status == "SUCCEEDED" {
				if run.DatasetID == "" {
					return a.finishJob(ctx, jid, "failed", "Apify 未返回 Dataset")
				}
				upstream, reportErr := a.extractionReport(ctx, token, jid, run)
				if reportErr != nil {
					return a.jobError(ctx, jid, reportErr)
				}
				_, e = a.db.Exec(ctx, "UPDATE jobs SET status='importing',dataset_id=$2,error=$3,upstream_error=$3 WHERE id=$1", jid, run.DatasetID, upstream)
				return e
			}
			if run.Status == "FAILED" || run.Status == "TIMED-OUT" || run.Status == "ABORTED" {
				return a.finishJob(ctx, jid, "failed", "Apify 运行结束: "+run.Status)
			}
			return nil
		case "importing":
			if dataset == nil {
				return errors.New("import job missing dataset")
			}
			return a.importDatasetPage(ctx, token, jid, *dataset, s, offset, started)
		case "retrying":
			return a.retryItems(ctx, jid, s)
		}
	}
	// The scheduler queues each enabled source once; manual requests use the same queue.
	tx, e := a.db.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	rows, e := tx.Query(ctx, "SELECT id,interval_hours FROM sources WHERE enabled AND kind<>'account' AND next_run<=now() FOR UPDATE SKIP LOCKED")
	if e != nil {
		return e
	}
	type due struct {
		id    string
		hours int
	}
	sources := []due{}
	for rows.Next() {
		var s due
		if e = rows.Scan(&s.id, &s.hours); e != nil {
			rows.Close()
			return e
		}
		sources = append(sources, s)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	for _, s := range sources {
		if _, e = tx.Exec(ctx, "INSERT INTO jobs(id,source_id,source_snapshot) SELECT $1,id,to_jsonb(s) FROM sources s WHERE s.id=$2 ON CONFLICT DO NOTHING", id(), s.id); e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, "UPDATE sources SET next_run=now()+make_interval(hours=>$2) WHERE id=$1", s.id, s.hours); e != nil {
			return e
		}
	}
	if e = tx.Commit(ctx); e != nil {
		return e
	}
	var recent bool
	if e = a.db.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM jobs WHERE started_at>now()-interval '5 minutes')").Scan(&recent); e != nil || recent {
		return e
	}
	e = a.db.QueryRow(ctx, "SELECT j.id,COALESCE(j.source_snapshot,to_jsonb(s)) FROM jobs j JOIN sources s ON s.id=j.source_id WHERE j.status='queued' ORDER BY j.created_at LIMIT 1").Scan(&jid, &sourceRaw)
	if e != nil {
		return nil
	}
	s, e := sourceFromJSON(sourceRaw)
	if e != nil {
		return e
	}
	query, err := normalizeSourceQuery(s.Kind, s.Query)
	if err != nil {
		return a.finishJob(ctx, jid, "failed", err.Error())
	}
	s.Query = query
	input := buildInput(s, time.Now())
	if _, e = a.db.Exec(ctx, "UPDATE jobs SET status='starting',input=$2,started_at=now(),source_snapshot=$3 WHERE id=$1", jid, encode(input), sourceRaw); e != nil {
		return e
	}
	var result struct {
		Data ActorRun `json:"data"`
	}
	e = a.apify(ctx, token, "POST", "/acts/"+actorPath+"/runs?build=latest&timeout=300", input, &result)
	if e != nil || result.Data.ID == "" {
		_, dbErr := a.db.Exec(ctx, "UPDATE jobs SET status='uncertain',error='启动结果不明，将核对运行记录；请勿重复创建任务' WHERE id=$1", jid)
		return dbErr
	}
	_, e = a.db.Exec(ctx, "UPDATE jobs SET status='running',run_id=$2,error='' WHERE id=$1", jid, result.Data.ID)
	return e
}
func (a *App) jobError(ctx context.Context, jid string, cause error) error {
	_, e := a.db.Exec(ctx, "UPDATE jobs SET error=$2 WHERE id=$1", jid, cause.Error())
	return e
}
func (a *App) finishJob(ctx context.Context, jid, status, msg string) error {
	_, e := a.db.Exec(ctx, "UPDATE jobs SET status=$2,error=$3,finished_at=now() WHERE id=$1", jid, status, msg)
	return e
}
func (a *App) reconcileAutomatic(ctx context.Context, token, jid string, raw []byte, started *time.Time) error {
	var result struct {
		Data struct {
			Items []ActorRun `json:"items"`
		} `json:"data"`
	}
	var historicalActor string
	if e := a.db.QueryRow(ctx, "SELECT actor FROM jobs WHERE id=$1", jid).Scan(&historicalActor); e != nil {
		return e
	}
	if historicalActor != actorName && historicalActor != "xtdata/twitter-x-scraper" {
		return a.jobError(ctx, jid, errors.New("未知历史 Actor"))
	}
	if e := a.apify(ctx, token, "GET", "/acts/"+strings.ReplaceAll(historicalActor, "/", "~")+"/runs?desc=true&limit=20", nil, &result); e != nil {
		return a.jobError(ctx, jid, e)
	}
	matches := []ActorRun{}
	for _, r := range result.Data.Items {
		if a.verifyRunInput(ctx, token, r, raw, started) == nil {
			matches = append(matches, r)
		}
	}
	if len(matches) == 1 {
		_, e := a.db.Exec(ctx, "UPDATE jobs SET status='running',run_id=$2,error='' WHERE id=$1", jid, matches[0].ID)
		return e
	}
	return a.jobError(ctx, jid, errors.New("未找到唯一匹配运行；请在 Apify 控制台核对后关联 run ID，或确认不存在此次运行"))
}
func (a *App) importDatasetPage(ctx context.Context, token, jid, dataset string, s Source, offset int, started *time.Time) error {
	var items []M
	if e := a.apify(ctx, token, "GET", fmt.Sprintf("/datasets/%s/items?format=json&clean=true&offset=%d&limit=50", url.PathEscape(dataset), offset), nil, &items); e != nil {
		return a.jobError(ctx, jid, e)
	}
	for i, raw := range items {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		created, cause := a.importTweet(ctx, s, raw)
		if e := a.recordItem(ctx, jid, raw, created, cause); e != nil {
			return e
		}
		if _, e := a.db.Exec(ctx, "UPDATE jobs SET dataset_offset=$2 WHERE id=$1", jid, offset+i+1); e != nil {
			return e
		}
	}
	if len(items) == 50 {
		return nil
	}
	var failed int
	if e := a.db.QueryRow(ctx, "SELECT count(*) FROM job_items WHERE job_id=$1 AND NOT resolved", jid).Scan(&failed); e != nil {
		return e
	}
	var upstream string
	if e := a.db.QueryRow(ctx, "SELECT upstream_error FROM jobs WHERE id=$1", jid).Scan(&upstream); e != nil {
		return e
	}
	status := "succeeded"
	if failed > 0 || upstream != "" {
		status = "partial"
	}
	if started != nil && upstream == "" {
		if _, e := a.db.Exec(ctx, "UPDATE sources SET watermark=GREATEST(COALESCE(watermark,$2),$2) WHERE id=$1", s.ID, *started); e != nil {
			return e
		}
	}
	return a.finishJob(ctx, jid, status, upstream)
}
func (a *App) recordItem(ctx context.Context, jid string, raw M, created bool, cause error) error {
	key := str(raw, "id", "id_str")
	if key == "" {
		key = tokenHash(string(encode(raw)))
	}
	msg := ""
	if cause != nil {
		msg = cause.Error()
	}
	tx, e := a.db.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	_, e = tx.Exec(ctx, "INSERT INTO job_items(id,job_id,source_key,payload,error,resolved) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(job_id,source_key) DO UPDATE SET error=excluded.error,resolved=excluded.resolved,attempts=job_items.attempts+1", id(), jid, key, encode(raw), msg, cause == nil)
	if e != nil {
		return e
	}
	if created {
		_, e = tx.Exec(ctx, "UPDATE jobs SET imported=imported+1 WHERE id=$1", jid)
	}
	if e != nil {
		return e
	}
	return tx.Commit(ctx)
}
func (a *App) retryItems(ctx context.Context, jid string, s Source) error {
	rows, e := a.db.Query(ctx, "SELECT payload FROM job_items WHERE job_id=$1 AND NOT resolved AND ((SELECT retry_item_id FROM jobs WHERE id=$1)='' OR id=(SELECT retry_item_id FROM jobs WHERE id=$1)) ORDER BY id", jid)
	if e != nil {
		return e
	}
	items := []M{}
	for rows.Next() {
		var b []byte
		if e = rows.Scan(&b); e != nil {
			rows.Close()
			return e
		}
		var m M
		_ = json.Unmarshal(b, &m)
		items = append(items, m)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	for _, raw := range items {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		created, cause := a.importTweet(ctx, s, raw)
		if e = a.recordItem(ctx, jid, raw, created, cause); e != nil {
			return e
		}
	}
	_, e = a.db.Exec(ctx, "UPDATE jobs SET status='importing',error='',retry_item_id='' WHERE id=$1", jid)
	return e
}
