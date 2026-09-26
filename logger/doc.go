// Package logger 提供基于 log/slog 的统一日志门面。
//
// 典型用途：httpx 与接入方业务共用一条日志通道；trace_id / span_id / WithAttrs
// 在 handler 层自动附加，调用点只传 ctx 和业务字段。
//
// 三行接入（业务入口必须保留派生 ctx，才能和后续 HTTP 日志同链）：
//
//	ctx = logger.EnsureTraceID(ctx) // 业务入口保留派生 ctx
//	accountSyncStarted := logger.NewEvent("account.sync.started")
//	logger.InfoEvent(ctx, accountSyncStarted, slog.String("account_id", "123"))
//	logger.Info(ctx, "业务开始", logger.EventAttr(accountSyncStarted.Name()))
//	body, err := client.Get(ctx, "/v1/account", nil)
//
// ctx 值不可变：EnsureTraceID / WithAttrs / StartSpan / WithTraceID 都返回派生 ctx，
// 必须写成 ctx = ...；丢掉返回值等于没注入。只走 httpx.Do 时，库内会给该次
// HTTP 日志补 trace；要让 Do 前后的业务日志同链，必须在业务入口先保留派生 ctx。
// SetHandler 是可选的进程级注入点，不用每个请求重复设置。
//
// InfoEvent / WarnEvent 的 msg 与 event 同值。需要人类文案与机器事件分离时，
// 才用 Info(ctx, msg, EventAttr(name))。
// 保留字段（trace_id / span_id / parent_span_id / span_name / event / duration_ms / error / module）
// 不得由业务 attrs 重用；slog 允许重名 key，不同 JSON 消费器的取值可能不一致。
//
// 拿不到 ctx 的地方（main、init、启动配置）用具名 Logger，不必硬造 context.Background()：
//
//	var log = logger.Named("mymod") // 包级声明一次，每条带 module=mymod
//	log.Info("启动完成", slog.String("addr", addr))
//	log.With(slog.String("account_id", "u1")).Warn("限流")
//
// Logger 不带 trace_id / span_id；要与 HTTP 日志同链仍用 Info(ctx, ...)。
//
// 设计要点：
//   - 门面在 logger.go：Debug/Info/Warn/Error(ctx, msg, attrs...)，ctx 必传首参。
//   - named.go 的 Logger 是全局 logger 之上的字段视图，每次调用才读当前 handler，
//     包级 var 先于 SetHandler / SetConfig 创建也生效。
//   - 默认输出构造在 config.go。未注入外部 logger 时按 Config 走（默认 console JSON / info）。
//     级别 / 格式 / 去向只用 SetConfig / SetHandler / SetLogger，不读环境变量。
//   - handler.go 包装任意 slog.Handler，注入 trace_id / span_id / WithAttrs。
//   - context.go / span.go 管 ctx 字段和轻量 span，不引 OpenTelemetry。
//   - 落盘失败是 *FileSetupError，由 mustFileWriter panic 抛出。对比用 errors.As。
//   - 注入点（SetLogger / SetHandler / SetConfig）是进程级全局状态。
//
// 本包只依赖标准库。
package logger
