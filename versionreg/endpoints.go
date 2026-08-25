package versionreg

import "sort"

// Endpoint 端点标识（如 "user.profile" / "thread.send"）。用常量集中定义，
// 避免字符串字面量散落各处 —— 拼错一个字母只会得到「白名单为空」这种沉默失败。
type Endpoint string

// String 实现 fmt.Stringer。
func (e Endpoint) String() string { return string(e) }

// HeaderWhitelists 按 endpoint 组织的请求头白名单集合。
//
// 值直接喂给 httpx 的 RequestSpec.HeaderWhitelist：key 是要发的头名，
// value 为空串表示取构头值、非空表示用这个固定值。
//
// 内部 map 不导出，一律经访问器读取：白名单是「哪些头会真的发出去」的单一真相源，
// 允许外部直接拿到底层 map 就等于允许它被就地改写，那会变成幽灵 bug 的温床。
type HeaderWhitelists struct {
	m map[Endpoint]map[string]string
}

// NewHeaderWhitelists 从字面量构造（会做一次深拷贝，之后与入参无关）。
func NewHeaderWhitelists(src map[Endpoint]map[string]string) *HeaderWhitelists {
	w := &HeaderWhitelists{m: make(map[Endpoint]map[string]string, len(src))}
	for ep, headers := range src {
		cp := make(map[string]string, len(headers))
		for k, v := range headers {
			cp[k] = v
		}
		w.m[ep] = cp
	}
	return w
}

// For 返回某 endpoint 的白名单副本；未配置时返回 nil。
//
// 返回副本而不是内部 map：调用方经常会在发请求前临时往白名单里塞一两个头
// （某个接口这次要多带一个字段），拿到的若是内部 map，这个「临时」就会永久留下。
func (w *HeaderWhitelists) For(ep Endpoint) map[string]string {
	if w == nil {
		return nil
	}
	src, ok := w.m[ep]
	if !ok {
		return nil
	}
	out := make(map[string]string, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}

// Has 判断某 endpoint 是否配了白名单。
func (w *HeaderWhitelists) Has(ep Endpoint) bool {
	if w == nil {
		return false
	}
	_, ok := w.m[ep]
	return ok
}

// Endpoints 列出所有已配置白名单的 endpoint（字典序）。
func (w *HeaderWhitelists) Endpoints() []Endpoint {
	if w == nil {
		return nil
	}
	out := make([]Endpoint, 0, len(w.m))
	for ep := range w.m {
		out = append(out, ep)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
