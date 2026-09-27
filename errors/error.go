package errors

import (
	"errors"
	"log/slog"
	"reflect"
	"slices"
	"strings"
)

// error.go —— 全局统一的结构化错误类型。
// 上下文不再靠 fmt.Errorf 前缀堆文本：Op 记录「在哪一步」，Kind 记录「属于哪一类」，
// Attrs 记录结构化附加信息，Err 保留内层错误。构造直接写字面量 &errors.Error{...}。

// Error 结构化错误。字段均可选；判定用 KindOf / IsKind 或 errors.AsType[*errors.Error]。
type Error struct {
	// Op 发生在哪一步，如 "android.v407.get_followers"（建议与 logger.StartSpan 名一致）。
	Op string
	// Kind 分类；nil 表示本层不分类，KindOf 继续向内查找。
	Kind Kind
	// Attrs 结构化附加信息（字段名、状态码、响应体片段等），Error() 按 key=value 输出。
	Attrs []slog.Attr
	// Err 内层错误，经 Unwrap 暴露给 errors.Is / errors.As。
	Err error
}

// Error 实现 error。
// 输入：接收者可为 nil。
// 返回：nil → "<nil>"；否则按 "op: kind: k=v k=v: 内层" 拼接，空段（含空名 Kind）跳过。
// 例：&Error{Op: "svc.get", Kind: NewKind("demo.x"), Err: io.EOF} → "svc.get: demo.x: EOF"。
func (e *Error) Error() string {
	if e == nil {
		return "<nil>"
	}
	parts := make([]string, 0, 4)
	if e.Op != "" {
		parts = append(parts, e.Op)
	}
	if name := kindName(e.Kind); name != "" {
		parts = append(parts, name)
	}
	if len(e.Attrs) > 0 {
		kv := make([]string, len(e.Attrs))
		for i, a := range e.Attrs {
			kv[i] = a.String()
		}
		parts = append(parts, strings.Join(kv, " "))
	}
	if e.Err != nil {
		parts = append(parts, e.Err.Error())
	}
	return strings.Join(parts, ": ")
}

// Unwrap 暴露内层错误。
// 输入：接收者可为 nil。
// 返回：nil 接收者返回 nil，否则返回 Err。
func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// KindOf 返回 err 链上从外到内第一个非 nil 的 Kind。
// 输入：err 可为 nil，可被任意 %w / Join / 其它 Unwrap 层包装。
// 返回：找到则为该 Kind；没有 *Error 或都未分类则为 nil。外层分类优先于内层。
func KindOf(err error) Kind {
	for e := range chain(err) {
		if e.Kind != nil {
			return e.Kind
		}
	}
	return nil
}

// IsKind 判断 err 链上是否有任意一层 *Error 的 Kind 等于 k。
// 输入：err、k 可为 nil（均返回 false）。
// 返回：按值比较；k 或链上 Kind 的动态类型不可比较时视为不相等，不 panic。
func IsKind(err error, k Kind) bool {
	if k == nil || !reflect.TypeOf(k).Comparable() {
		return false
	}
	for e := range chain(err) {
		if e.Kind != nil && reflect.TypeOf(e.Kind).Comparable() && e.Kind == k {
			return true
		}
	}
	return false
}

// AttrsOf 合并 err 链上所有 *Error 的 Attrs，从外到内。
// 输入：err 可为 nil。
// 返回：新切片（不共享调用方底层数组）；链上没有 Attrs 时为 nil。给日志一次打全上下文用。
func AttrsOf(err error) []slog.Attr {
	var out []slog.Attr
	for e := range chain(err) {
		out = append(out, slices.Clone(e.Attrs)...)
	}
	return out
}

// chain 从外到内遍历 err 链上的每个非 nil *Error。
// 输入：err 可为 nil。
// 返回：迭代器；每步用 errors.AsType 找下一个 *Error（支持 Join 与任意 Unwrap 层），再从其 Err 继续。
func chain(err error) func(yield func(*Error) bool) {
	return func(yield func(*Error) bool) {
		for err != nil {
			e, ok := errors.AsType[*Error](err)
			if !ok || e == nil {
				return
			}
			if !yield(e) {
				return
			}
			err = e.Err
		}
	}
}

// kindName 安全取名。
// 输入：k 可为 nil 接口。
// 返回：nil 时为空串，否则为 k.Name()。
func kindName(k Kind) string {
	if k == nil {
		return ""
	}
	return k.Name()
}
