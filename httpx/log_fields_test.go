package httpx_test

import (
	"testing"

	"github.com/japansms40-web/gohttpkit/httpx"
)

func TestLogFieldKeys_契约值(t *testing.T) {
	cases := []struct {
		name string
		got  string
		want string
	}{
		{"method", httpx.LogFieldMethod, "method"},
		{"url", httpx.LogFieldURL, "url"},
		{"proxy", httpx.LogFieldProxy, "proxy"},
		{"exit_ip", httpx.LogFieldExitIP, "exit_ip"},
		{"asn", httpx.LogFieldASN, "asn"},
		{"req_headers", httpx.LogFieldReqHeaders, "req_headers"},
		{"req_params", httpx.LogFieldReqParams, "req_params"},
		{"req_body", httpx.LogFieldReqBody, "req_body"},
		{"req_body_len", httpx.LogFieldReqBodyLen, "req_body_len"},
		{"status", httpx.LogFieldStatus, "status"},
		{"proto", httpx.LogFieldProto, "proto"},
		{"proto_major", httpx.LogFieldProtoMajor, "proto_major"},
		{"resp_headers", httpx.LogFieldRespHeaders, "resp_headers"},
		{"resp_body", httpx.LogFieldRespBody, "resp_body"},
		{"resp_body_len", httpx.LogFieldRespBodyLen, "resp_body_len"},
		{"duration_ms", httpx.LogFieldDurationMS, "duration_ms"},
		{"slow", httpx.LogFieldSlow, "slow"},
		{"attempt", httpx.LogFieldAttempt, "attempt"},
		{"max_retries", httpx.LogFieldMaxRetries, "max_retries"},
		{"backoff", httpx.LogFieldBackoff, "backoff"},
		{"err_type", httpx.LogFieldErrType, "err_type"},
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
