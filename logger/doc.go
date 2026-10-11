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
//	body, err := client.Get(ctx, "/v1/account", nil)
//
// ctx 值不可变：EnsureTraceID / WithAttrs / StartSpan / WithTraceID 都返回派生 ctx，
// 必须写成 ctx = ...；丢掉返回值等于没注入。只走 httpx.Do 时，库内会给该次
// HTTP 日志补 trace；要让 Do 前后的业务日志同链，必须在业务入口先保留派生 ctx。
// 进程入口调一次 Init 接管全部日志输出（级别、格式、去向、轮转、固定字段），不用每个请求重复设置；
// 不调 Init 时走缺省输出（控制台 JSON、info 级）。
//
// DebugEvent / InfoEvent / WarnEvent / ErrorEvent 的 msg 与 event 同值；业务代码只打事件，不写自由文案日志。
// 保留字段（trace_id / span_id / parent_span_id / span_name / event / duration_ms / error / module）
// 不得由业务 attrs 重用；slog 允许重名 key，不同 JSON 消费器的取值可能不一致。
//
// 拿不到 ctx 的地方（main、init、启动配置）用具名 Logger，不必硬造 context.Background()：
//
//	var log = logger.Named("mymod") // 包级声明一次，每条带 module=mymod
//	started := logger.NewEvent("app.started") // 业务代码只打事件，不写自由文案
//	log.InfoEvent(started, slog.String("addr", addr)) // msg 与 event 同值
//	log.With(slog.String("account_id", "u1")).WarnEvent(logger.NewEvent("app.rate_limited"))
//
// Logger 不带 trace_id / span_id；要与 HTTP 日志同链仍用 InfoEvent(ctx, ev, ...)。
//
// 设计要点：
//   - 门面在 logger.go：Debug/Info/Warn/Error(ctx, msg, attrs...)，ctx 必传首参。
//   - named.go 的 Logger 是全局 logger 之上的字段视图，每次调用才读当前 handler，
//     包级 var 先于 Init 创建也生效。
//   - config.go 的 Init 是唯一的初始化入口：按 Config 建输出（落盘走 lumberjack 轮转）、挂固定字段，
//     并把标准库 log 与 slog 默认 logger 并入同一条通道；Close 撤销最近一次 Init。不读环境变量。
//     没有 SetHandler / SetLogger 这类注入点：全进程只有一个初始化入口、一个打印出口。
//   - handler.go 包装任意 slog.Handler，注入 trace_id / span_id / WithAttrs。
//   - context.go / span.go 管 ctx 字段和轻量 span，不引 OpenTelemetry。
//   - 落盘失败是 *FileSetupError，由 Init 返回，不 panic，进程日志状态保持不变。对比用 errors.As。
//   - Init / Close 改的是进程级全局状态，成对使用，可嵌套，后进先出。
//   - 测试要观察日志时用 logger/loggertest 的 Capture，它只走 Init / Close。
//
// 本包除标准库外只依赖 lumberjack（日志文件轮转）。
//
// 文件结构：
//
//	logger/
//	├── config.go   Config（级别 / 格式 / 去向 / 轮转）与唯一初始化入口 Init、Close
//	├── context.go  ctx 里的 trace_id 与 WithAttrs 字段
//	├── doc.go      包文档（本文件）
//	├── errors.go   本包类型错误：FileSetupError
//	├── event.go    机器事件 Event 与 EventAttr
//	├── fields.go   结构化日志字段 key 常量
//	├── handler.go  包装任意 slog.Handler，注入 trace_id / span_id / WithAttrs
//	├── logger.go   门面 Debug / Info / Warn / Error 与 Enabled
//	├── loggertest/ 测试用的日志捕获：Capture 走 Init 把日志接进内存
//	├── named.go    具名 Logger：拿不到 ctx 时用
//	└── span.go     轻量 span：StartSpan、span_id 与 DurationMs
package logger
