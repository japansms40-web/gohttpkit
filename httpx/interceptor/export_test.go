package interceptor

// export_test.go —— 仅 go test ./httpx/interceptor 时编译，不进生产二进制，也不进 godoc。
// 给 package interceptor_test 暴露未导出实现，避免为测试污染生产导出面。

// ComputeBackoff 暴露 computeBackoff，供 retry_test.go 做退避边界与溢出测试。
var ComputeBackoff = computeBackoff
