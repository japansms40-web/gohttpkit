// Package logger 提供基于 log/slog 的统一日志门面。
//
// 典型用途：httpx 与接入方业务共用一条日志通道；trace_id / span_id / WithAttrs
// 在 handler 层自动附加，调用点只传 ctx 和业务字段。
//
// 三行接入（业务入口必须保留派生 ctx，才能和后续 HTTP 日志同链）：
//
//	ctx = logger.EnsureTraceID(ctx) // 业务入口保留派生 ctx
//	logger.Info(ctx, "业务开始", logger.Event("account.sync.started"))
//	body, err := client.Get(ctx, "/v1/account", nil)
//
// ctx 值不可变：EnsureTraceID / WithAttrs / StartSpan / WithTraceID 都返回派生 ctx，
// 必须写成 ctx = ...；丢掉返回值等于没注入。只走 httpx.Do 时，库内会给该次
// HTTP 日志补 trace；要让 Do 前后的业务日志同链，必须在业务入口先保留派生 ctx。
// SetHandler 是可选的进程级注入点，不用每个请求重复设置。
//
// event 是机器契约，msg 继续是旧查询和人类文案契约：加 Event() 不改 msg。
// 保留字段（trace_id / span_id / parent_span_id / span_name / event / duration_ms / error）
// 不得由业务 attrs 重用；slog 允许重名 key，不同 JSON 消费器的取值可能不一致。
//
// 设计要点：
//   - 门面在 logger.go：Debug/Info/Warn/Error(ctx, msg, attrs...)，ctx 必传首参。
//   - 默认输出构造在 config.go。未注入外部 logger 时按 Config 走（默认 console JSON / info）。
//     级别 / 格式 / 去向只用 SetConfig / SetHandler / SetLogger，不读环境变量。
//   - handler.go 包装任意 slog.Handler，注入 trace_id / span_id / WithAttrs。
//   - context.go / span.go 管 ctx 字段和轻量 span，不引 OpenTelemetry。
//   - 落盘失败是 *FileSetupError，由 mustFileWriter panic 抛出。对比用 errors.As。
//   - 注入点（SetLogger / SetHandler / SetConfig）是进程级全局状态。
//
// 本包只依赖标准库。
package logger
