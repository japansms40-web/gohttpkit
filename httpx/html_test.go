package httpx_test

// html_test.go —— ExtractHTMLText 的单元契约。

import (
	"testing"

	"github.com/japansms40-web/gohttpkit/httpx"
)

// ─────────────────────────────── html.go ───────────────────────────────

func TestExtractHTMLText_忽略脚本样式并压缩空白(t *testing.T) {
	in := `<html><head><style>p{color:red}</style><script>var a=1</script></head>
	       <body><p>  Hello  </p><noscript>NO</noscript><iframe>IF</iframe><p>World</p></body></html>`
	got := httpx.ExtractHTMLText(in)
	t.Logf("in → %q", got)
	if got != "Hello World" {
		t.Fatalf("got %q", got)
	}
}

func TestExtractHTMLText_空输入(t *testing.T) {
	got := httpx.ExtractHTMLText("")
	t.Logf("empty → %q", got)
	if got != "" {
		t.Fatalf("got %q", got)
	}
}

func TestExtractHTMLText_畸形不panic(t *testing.T) {
	in := "<<<>}{{{not-html"
	got := httpx.ExtractHTMLText(in)
	t.Logf("畸形 → %q", got)
	// x/net/html 解析器极宽，几乎不返回 error；此用例锁「不 panic、有确定输出」。
	_ = got
}
