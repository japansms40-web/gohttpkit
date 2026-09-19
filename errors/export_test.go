package errors

// export_test.go —— 仅 go test ./errors 时编译，不进生产二进制，也不进 godoc。
// 给 package errors_test 一个看关键词表快照的入口，避免为此污染生产导出面。

// RetryableKeywords 给 errors_test 看表快照；生产接入方看不到。
func RetryableKeywords() []string {
	return retryableKeywordsSnapshot()
}
