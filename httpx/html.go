package httpx

import (
	"strings"

	"golang.org/x/net/html"
)

// html.go —— HTML 纯文本提取。独立成文件：它依赖 golang.org/x/net/html，
// 和拦截器、编解码分开，避免「只想看提纯规则」时翻完整条链。

// htmlSkipTags 是 ExtractHTMLText 解析时需忽略内容的标签集（运行期只读）。
var htmlSkipTags = map[string]bool{
	"script":   true,
	"style":    true,
	"noscript": true,
	"iframe":   true,
}

// ExtractHTMLText 从 HTML 中提取纯文本。
// 给只关心可见文案的调用方（错误页识别、摘要）；需要抠 script 里 token 的场景不要用。
// 输入 htmlContent：原始 HTML；空串原样返回；解析失败也原样返回输入（不报错）。
// 返回：遍历节点树取文本节点，忽略 script/style/noscript/iframe，去空白后用单个空格连接。
// 例：ExtractHTMLText("<p>hi</p>") → "hi"；
// ExtractHTMLText("") → ""；畸形输入解析失败 → 原串。
// 使用警告：现代页面的关键数据（内联 JSON、token）恰恰在 script 里。
// 需要从页面里抠 token 的场景【不要】用它，也不要挂 NewHTMLTextInterceptor —— 用原始 body。
func ExtractHTMLText(htmlContent string) string {
	doc, err := html.Parse(strings.NewReader(htmlContent))
	if err != nil {
		return htmlContent
	}

	var textNodes []string
	var extractText func(*html.Node)
	extractText = func(n *html.Node) {
		if n.Type == html.ElementNode && htmlSkipTags[n.Data] {
			return
		}
		if n.Type == html.TextNode {
			if text := strings.TrimSpace(n.Data); text != "" {
				textNodes = append(textNodes, text)
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			extractText(c)
		}
	}
	extractText(doc)
	return strings.Join(textNodes, " ")
}
