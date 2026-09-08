package main

import (
	"context"
	"fmt"
	"net/url"
)

func reportWarning(report M) string {
	// Current run-report nests extraction counters under results. Older reports
	// used top-level counters. The summary outcome may be partial simply because
	// fewer than maxItems were returned; use the specific extraction reason.
	results := report
	if nested, ok := report["results"].(map[string]any); ok {
		results = nested
	}
	reason := str(results, "completionReason")
	failed, _ := results["failedSubtargets"].(float64)
	if failed == 0 && (reason == "source_exhausted" || reason == "completed") {
		return ""
	}
	if reason == "" {
		return "Xquik 运行报告缺少结束原因，暂时无法确认提取完整性；已保留可用结果，未推进水位。重试导入不会重新采集。"
	}
	return fmt.Sprintf("Xquik 提取未完整结束（%s，失败查询 %.0f 个）；已保留可用结果，未推进水位。重试导入不会重新采集。", reason, failed)
}

func (a *App) extractionReport(ctx context.Context, token, jid string, run ActorRun) (string, error) {
	var actor string
	if e := a.db.QueryRow(ctx, "SELECT actor FROM jobs WHERE id=$1", jid).Scan(&actor); e != nil {
		return "", e
	}
	if actor != actorName {
		return "", nil
	}
	var report M
	if e := a.apify(ctx, token, "GET", "/key-value-stores/"+url.PathEscape(run.StoreID)+"/records/run-report", nil, &report); e != nil {
		return "", e
	}
	return reportWarning(report), nil
}
