package interceptor

import "github.com/japansms40-web/gohttpkit/httpx"

// init 把 DefaultChain 注册为 httpx.NewClient 的默认链：import 本包后，httpx.NewClient 未传 Interceptors 时自动装上。
func init() {
	httpx.RegisterDefaultChain(DefaultChain)
}
