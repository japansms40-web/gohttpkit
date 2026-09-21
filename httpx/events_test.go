package httpx_test

import (
	"testing"

	"github.com/japansms40-web/gohttpkit/httpx"
	"github.com/japansms40-web/gohttpkit/logger"
)

func TestHTTPEvents_名称稳定(t *testing.T) {
	cases := []struct {
		event logger.Event
		want  string
	}{
		{httpx.EventHTTPTransaction, "http.transaction"},
		{httpx.EventHTTPRetry, "http.retry"},
		{httpx.EventHTTPRetrySucceeded, "http.retry.succeeded"},
	}
	for _, tc := range cases {
		got := tc.event.Name()
		t.Logf("Name()=%q want=%q", got, tc.want)
		if got != tc.want {
			t.Fatalf("Name()=%q, want %q", got, tc.want)
		}
	}
}
