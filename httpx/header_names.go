package httpx

// header_names.go —— 本库用到的 HTTP header 名。单一事实源，禁止在发头 / 读头处再写同名字面量。
// 值一律小写，遵守「全小写发头」约定。不作 type 枚举：它们只当 map key，套 type 只会引入转换噪音。

const (
	HeaderContentType     = "content-type"
	HeaderContentEncoding = "content-encoding"
	HeaderContentLength   = "content-length"
	HeaderUserAgent       = "user-agent"
	HeaderHost            = "host"
	HeaderOrigin          = "origin"
	HeaderReferer         = "referer"
	HeaderAcceptLanguage  = "accept-language"
	HeaderLocation        = "location"
	HeaderSetCookie       = "set-cookie"
	HeaderCookie          = "cookie"

	// HeaderUserAgentCanonical 是标准库 http.Request.Header 对 UA 的规范化 key。
	// 仅用于抑制 Go 默认 UA、或与 http.Header 的 MIME 规范化形态对齐；发头仍用 HeaderUserAgent。
	HeaderUserAgentCanonical = "User-Agent"
)
