package interceptor

import "github.com/japansms40-web/gohttpkit/httpx"

// NewClient 创建带默认链的客户端。
// 给开箱即用的接入方：Interceptors 为 nil 时填 DefaultChain()，再交给 httpx.New。
// 输入 opts：规则同 httpx.New；空切片（非 nil）视为调用方显式自组链，不会回落默认链。
// 返回：同 httpx.New。例：NewClient(httpx.Options{Headers: h})。
func NewClient(opts httpx.Options) (*httpx.Client, error) {
	if opts.Interceptors == nil {
		opts.Interceptors = DefaultChain()
	}
	return httpx.New(opts)
}
