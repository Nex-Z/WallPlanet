package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestXquikNestedRunReport(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		complete   bool
	}{
		{"source exhausted below cap", `{"schemaVersion":1,"outcome":"partial","config":{"maxItems":1000},"results":{"realRows":100,"completionReason":"source_exhausted","failedSubtargets":0}}`, true},
		{"nested failure", `{"results":{"completionReason":"partial_failure","failedSubtargets":1}}`, false},
		{"failed counter wins", `{"results":{"completionReason":"source_exhausted","failedSubtargets":1}}`, false},
		{"nested takes precedence", `{"completionReason":"completed","results":{"completionReason":"deadline_reached","failedSubtargets":0}}`, false},
		{"legacy flat", `{"completionReason":"source_exhausted","failedSubtargets":0}`, true},
		{"missing reason", `{"results":{"realRows":100}}`, false},
		{"unknown reason", `{"results":{"completionReason":"new_unknown_state"}}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var report M
			if e := json.Unmarshal([]byte(tc.body), &report); e != nil {
				t.Fatal(e)
			}
			warning := reportWarning(report)
			if (warning == "") != tc.complete || strings.Contains(warning, "（）") {
				t.Fatal(warning)
			}
		})
	}
}
