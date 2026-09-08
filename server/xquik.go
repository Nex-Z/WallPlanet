package main

import (
	"context"
	"fmt"
	"net/url"
)

func reportWarning(report M) string {
	reason := str(report, "completionReason")
	failed, _ := report["failedSubtargets"].(float64)
	if reason == "" || reason == "partial_failure" || reason == "deadline_reached" || reason == "pagination_safety_limit" || reason == "invalid_input" || reason == "no_input" || failed > 0 {
		return fmt.Sprintf("Xquik 提取未完整结束（%s）；已保留可用结果，未推进水位。重试导入不会重新采集。", reason)
	}
	return ""
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
