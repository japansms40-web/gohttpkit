package httpx

import (
	"strings"

	"golang.org/x/net/html"
)

// htmlSkipTags 是 ExtractHTMLText 解析时需忽略内容的标签集（运行期只读）。
var htmlSkipTags = map[string]bool{
	"script":   true,
	"style":    true,
	"noscript": true,
	"iframe":   true,
}

// ExtractHTMLText 从 HTML 中提取纯文本：遍历节点树取文本节点，忽略 script/style/noscript/iframe，
// 去掉多余空白后用单个空格连接。解析失败时原样返回输入。
//
// 使用警告：它会剥掉 <script>，而现代页面的关键数据（各种内联 JSON、token）恰恰在 script 里。
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
