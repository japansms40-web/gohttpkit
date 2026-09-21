package httpx_test

import (
	"testing"

	"github.com/japansms40-web/gohttpkit/httpx"
)

func TestHeaderNames_全小写契约(t *testing.T) {
	cases := []struct {
		name string
		got  string
		want string
	}{
		{"ContentType", httpx.HeaderContentType, "content-type"},
		{"ContentEncoding", httpx.HeaderContentEncoding, "content-encoding"},
		{"ContentLength", httpx.HeaderContentLength, "content-length"},
		{"UserAgent", httpx.HeaderUserAgent, "user-agent"},
		{"Host", httpx.HeaderHost, "host"},
		{"Origin", httpx.HeaderOrigin, "origin"},
		{"Referer", httpx.HeaderReferer, "referer"},
		{"AcceptLanguage", httpx.HeaderAcceptLanguage, "accept-language"},
		{"Location", httpx.HeaderLocation, "location"},
		{"SetCookie", httpx.HeaderSetCookie, "set-cookie"},
		{"Cookie", httpx.HeaderCookie, "cookie"},
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

func TestHeaderUserAgentCanonical_标准库形态(t *testing.T) {
	t.Logf("canonical=%q lowercase=%q", httpx.HeaderUserAgentCanonical, httpx.HeaderUserAgent)
	if httpx.HeaderUserAgentCanonical != "User-Agent" {
		t.Fatalf("HeaderUserAgentCanonical = %q, want User-Agent", httpx.HeaderUserAgentCanonical)
	}
	if httpx.HeaderUserAgentCanonical == httpx.HeaderUserAgent {
		t.Fatal("规范化写法与全小写发头约定不能是同一个常量")
	}
}
