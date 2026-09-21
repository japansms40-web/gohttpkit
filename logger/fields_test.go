package logger_test

import (
	"testing"

	"github.com/japansms40-web/gohttpkit/logger"
)

func TestFieldKeys_契约值(t *testing.T) {
	cases := []struct {
		name string
		got  string
		want string
	}{
		{"event", logger.FieldEvent, "event"},
		{"trace_id", logger.FieldTraceID, "trace_id"},
		{"span_id", logger.FieldSpanID, "span_id"},
		{"span_name", logger.FieldSpanName, "span_name"},
		{"error", logger.FieldError, "error"},
		{"parent_span_id", logger.FieldParentSpanID, "parent_span_id"},
		{"duration_ms", logger.FieldDurationMS, "duration_ms"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Logf("%s = %q", tc.name, tc.got)
			if tc.got != tc.want {
				t.Fatalf("got %q, want %q", tc.got, tc.want)
			}
		})
	}
}
