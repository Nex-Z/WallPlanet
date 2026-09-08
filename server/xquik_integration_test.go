package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestIntegrationXquikPartialReportKeepsWatermark(t *testing.T) {
	a := integrationApp(t)
	ctx := context.Background()
	_, e := a.db.Exec(ctx, "INSERT INTO sources(id,name,kind,query) VALUES('source','batch','accounts','NASA NASAWebb'); INSERT INTO jobs(id,source_id,status) VALUES('job','source','running')")
	if e != nil {
		t.Fatal(e)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Fatal("must not start a paid run")
		}
		if strings.HasSuffix(r.URL.Path, "/run-report") {
			w.Write([]byte(`{"results":{"completionReason":"partial_failure","failedSubtargets":1}}`))
			return
		}
		if r.URL.Path == "/datasets/dataset/items" {
			w.Write([]byte(`[]`))
			return
		}
		w.WriteHeader(404)
	}))
	defer srv.Close()
	old := apifyBase
	apifyBase = srv.URL
	defer func() { apifyBase = old }()
	a.http = srv.Client()
	warning, e := a.extractionReport(ctx, "mock-token", "job", ActorRun{StoreID: "store"})
	if e != nil || warning == "" {
		t.Fatal(warning, e)
	}
	if _, e = a.db.Exec(ctx, "UPDATE jobs SET upstream_error=$1,status='importing',dataset_id='dataset' WHERE id='job'", warning); e != nil {
		t.Fatal(e)
	}
	started := time.Now()
	if e = a.importDatasetPage(ctx, "mock-token", "job", "dataset", Source{ID: "source"}, 0, &started); e != nil {
		t.Fatal(e)
	}
	var status string
	var watermark *time.Time
	if e = a.db.QueryRow(ctx, "SELECT j.status,s.watermark FROM jobs j JOIN sources s ON s.id=j.source_id WHERE j.id='job'").Scan(&status, &watermark); e != nil {
		t.Fatal(e)
	}
	if status != "partial" || watermark != nil {
		t.Fatal(status, watermark)
	}
	// Correcting an earlier report parse must resume the same dataset, without a new run.
	warning = reportWarning(M{"outcome": "partial", "results": M{"completionReason": "source_exhausted", "failedSubtargets": float64(0)}})
	if warning != "" {
		t.Fatal(warning)
	}
	if _, e = a.db.Exec(ctx, "UPDATE jobs SET upstream_error=$1,status='importing' WHERE id='job'", warning); e != nil {
		t.Fatal(e)
	}
	if e = a.importDatasetPage(ctx, "mock-token", "job", "dataset", Source{ID: "source"}, 0, &started); e != nil {
		t.Fatal(e)
	}
	if e = a.db.QueryRow(ctx, "SELECT j.status,s.watermark FROM jobs j JOIN sources s ON s.id=j.source_id WHERE j.id='job'").Scan(&status, &watermark); e != nil {
		t.Fatal(e)
	}
	if status != "succeeded" || watermark == nil {
		t.Fatal(status, watermark)
	}

}
