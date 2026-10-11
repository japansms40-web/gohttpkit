// Package loggertest 给测试观察日志用：Capture 把进程日志接进内存，并解析成结构化记录。
//
// 典型用法：
//
//	rec := loggertest.Capture(t, logger.LevelDebug)
//	doSomething(ctx)
//	if !slices.Contains(rec.Events(), "account.sync.started") { ... }
//
// Capture 只走公开的 logger.Init / logger.Close：用例期间进程日志（含标准库 log 与 slog 默认 logger）
// 都进这个 Recorder，用例结束时还原到 Capture 之前的状态。它改的是进程级状态，用到它的用例不要并行。
// 下游仓库的测试共用这一个实现，不必各自手写 slog.Handler 来截日志。
//
// 文件结构：
//
//	loggertest/
//	├── capture.go  Capture、Recorder 与 Record
//	└── doc.go      包文档（本文件）
package loggertest
