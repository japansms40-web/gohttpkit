package httpx_test

import (
	"net/http"
	"testing"

	"github.com/japansms40-web/gohttpkit/httpx"
)

func TestClassifyStatus_按百位归类(t *testing.T) {
	cases := []struct {
		name string
		code int
		want httpx.StatusClass
	}{
		{"99 未知", 99, httpx.ClassUnknown},
		{"100 信息", http.StatusContinue, httpx.ClassInformational},
		{"199 信息上沿", 199, httpx.ClassInformational},
		{"200 成功", http.StatusOK, httpx.ClassSuccess},
		{"299 成功上沿", 299, httpx.ClassSuccess},
		{"300 重定向", http.StatusMultipleChoices, httpx.ClassRedirection},
		{"399 重定向上沿", 399, httpx.ClassRedirection},
		{"400 客户端错", http.StatusBadRequest, httpx.ClassClientError},
		{"499 客户端错上沿", 499, httpx.ClassClientError},
		{"500 服务端错", http.StatusInternalServerError, httpx.ClassServerError},
		{"599 服务端错上沿", 599, httpx.ClassServerError},
		{"600 未知", 600, httpx.ClassUnknown},
		{"0 未知", 0, httpx.ClassUnknown},
		{"负数未知", -1, httpx.ClassUnknown},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := httpx.ClassifyStatus(tc.code)
			t.Logf("code=%d → %v", tc.code, got)
			if got != tc.want {
				t.Fatalf("ClassifyStatus(%d) = %v, want %v", tc.code, got, tc.want)
			}
		})
	}
}

func TestIsSuccessStatus_仅2xx(t *testing.T) {
	cases := []struct {
		code int
		want bool
	}{
		{http.StatusOK, true},
		{299, true},
		{199, false},
		{http.StatusMultipleChoices, false},
		{http.StatusBadRequest, false},
	}
	for _, tc := range cases {
		got := httpx.IsSuccessStatus(tc.code)
		t.Logf("IsSuccessStatus(%d) = %v", tc.code, got)
		if got != tc.want {
			t.Fatalf("IsSuccessStatus(%d) = %v, want %v", tc.code, got, tc.want)
		}
	}
}

func TestIsErrorStatus_保留大于等于400(t *testing.T) {
	cases := []struct {
		code int
		want bool
	}{
		{http.StatusOK, false},
		{http.StatusMultipleChoices, false},
		{399, false},
		{http.StatusBadRequest, true},
		{http.StatusInternalServerError, true},
		{599, true},
		{600, true},
	}
	for _, tc := range cases {
		got := httpx.IsErrorStatus(tc.code)
		t.Logf("IsErrorStatus(%d) = %v", tc.code, got)
		if got != tc.want {
			t.Fatalf("IsErrorStatus(%d) = %v, want %v", tc.code, got, tc.want)
		}
	}
}
