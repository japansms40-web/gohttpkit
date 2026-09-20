package versionreg

import "sort"

// endpoints.go —— 按 endpoint 组织的白名单 / 参数访问器。
// 独立成文件：和注册表骨架分开，复用注册表但不想用白名单的接入方不必读这份契约。

// Endpoint 端点标识（如 "user.profile" / "thread.send"）。
// 给各版本配置用常量集中定义，避免字符串字面量散落。
// 输入：构造时就是原串。
// 返回：String 原样返回底层 string。
type Endpoint string

// String 实现 fmt.Stringer。
// 输入：接收者是端点标识本身。
// 返回：底层字符串。
func (e Endpoint) String() string { return string(e) }

// HeaderWhitelists 按 endpoint 组织的请求头白名单集合。
// 给接入方喂给 httpx 的 RequestSpec.HeaderWhitelist：key 是要发的头名，
// value 为空串表示取构头值、非空表示用这个固定值。
//
// 内部 map 不导出，一律经访问器读取：白名单是「哪些头会真的发出去」的单一真相源，
// 允许外部直接拿到底层 map 就等于允许它被就地改写。
type HeaderWhitelists struct {
	m map[Endpoint]map[string]string
}

// NewHeaderWhitelists 从字面量构造（会做一次深拷贝，之后与入参无关）。
// 输入：src 为 nil 得到可用的空集合；空 map 同样是空集合。不改调用方 map。
// 返回：非 nil *HeaderWhitelists；For 未命中为 nil，Has 为 false，Endpoints 长度为 0。
// 例：NewHeaderWhitelists(nil).For("x") → nil（不是空 map）。
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
// 给发请求前临时改白名单的调用方：拿到的是副本，「这次多带一个头」不会留下。
// 输入：ep 端点标识；接收者可为 nil。
// 返回：未配置或接收者 nil → nil（发全量头）；已配置 → 新 map。不要把 nil 改成空 map。
// 例：已配置 "user.profile" → 副本；For("zzz") → nil。
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
// 输入：ep 端点标识；接收者可为 nil。
// 返回：nil 接收者或未配置 → false。
func (w *HeaderWhitelists) Has(ep Endpoint) bool {
	if w == nil {
		return false
	}
	_, ok := w.m[ep]
	return ok
}

// Endpoints 列出所有已配置白名单的 endpoint（字典序）。
// 输入：接收者可为 nil。
// 返回：nil 接收者 → nil；空集合 → 长度为 0 的切片；否则新切片。
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
