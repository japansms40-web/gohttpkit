# gohttpkit 协作指南（AGENTS）

> 本文件是所有 AI 编码代理（Codex、Cursor、Claude、Gemini 等）与人类贡献者在本仓库工作的**唯一正本**。
> `CLAUDE.md` / `GEMINI.md` 是指向本文件的符号链接，改这里即改全部；Cursor 另读 `.cursor/rules/*.mdc` 做目录级细化。
> 规范全文见 [`docs/CODE_STANDARDS.md`](docs/CODE_STANDARDS.md)，贡献流程见 [`CONTRIBUTING.md`](CONTRIBUTING.md)，
> 版本策略见 [`docs/VERSIONING.md`](docs/VERSIONING.md)，门禁与治理见 [`docs/ENGINEERING_GOVERNANCE.md`](docs/ENGINEERING_GOVERNANCE.md)。

## 通用协作规范

- 用中文沟通与写注释、设计说明、提交说明；外部协议、既有英文接口或 godoc 惯例要求英文时除外。
- 开工前先读与任务直接相关的代码、文档、配置。文档与实现不一致时以只读检查和实际行为为准，并说明差异。
- 修改前执行 `git status --short`。工作树可能有 WIP，只处理当前任务需要的文件与 hunk，不顺带格式化或清理无关改动。
- 优先完成已授权且可逆的工作。会扩大范围、改变外部状态或需要业务取舍的事项，先与用户确认。

## 项目结构

本库是「导出即契约」的通用 Go HTTP 基建，不绑定任何具体 API。核心包：

- `httpx/`：`Client` + OkHttp 风格拦截器链（重试/退避、解压、状态语义、白名单发头、快照）。默认链由 `interceptor` 子包组装。
- `geo/`：国家→locale / 时区大数据映射表，一表一文件，三表 keyset 由测试对齐。
- `logger/`：结构化日志门面，`SetHandler` 注入实现。
- `netproxy/`：代理接入与 TCP 层流量计数（`traffic/`）。
- `versionreg/`：版本/配置注册表。
- `errors/`：**带字段的类型错误**（非哨兵），供调用方 `errors.As` 解出字段。
- `examples/`：`quickstart`（打真实接口）、`customchain` / `fidelity`（自带假服务器，离线可跑，CI 会跑后两个）。
- `docs/`：架构、规范、版本与治理文档。

## 实现约束（要点，全文见 CODE_STANDARDS）

- **导出即契约**：新符号默认小写；确需导出的，doc comment 写明「给谁用、什么场景」。改导出符号的类型/字段/行为要按 `docs/VERSIONING.md` 升版本。
- **错误是类型不是哨兵**：库错误定义为 `type XxxError struct` + `Error()`，`return &XxxError{...}`，对比用 `errors.As`。禁止用 `errors.New` / `var ErrXxx` 当身份、禁止扫 `Error()` 文案。网络发送失败必须包成 `*httpx.TransportError`，否则重试层看不见。
- **默认链不做业务判断**：任何「替调用方对响应下结论」的逻辑都是可选拦截器，不进 `DefaultChain`。链顺序即语义，改序或插层前先 `make char`。
- **IO 函数第一参是 `context.Context`** 并沿链透传；超时用 `context.WithTimeout(ctx, …)`，不新建根 ctx。
- **日志走 `logger` 门面**（`logger.Info(ctx, …)`，ctx 必传），禁止裸 `fmt.Print` / `log.*` / `slog.*`（`logger/`、`examples/` 除外）；测试观察走 `t.Log` / `t.Logf`，不在 `*_test.go` 打 `logger`。协议 body 经 `TruncateBodyForLog` 截断并带原始长度。
- **含锁结构体**头部写并发模型注释（谁共享、哪把锁保护哪些字段）；受锁字段跨包只经访问器读写；锁内不做分配/IO。
- **常量与魔法值**：闭合集合且进 `switch` 的用 `type X string` 枚举；header 名/日志 key 用具名 const（不套 type）；状态码用 `StatusClass` / stdlib 常量。`geo/` 表内字面量与 `examples/` 豁免。
- **大数据表**（geo）独立成文件、表头写数据源，禁止文件名以 `_<GOOS>.go` 结尾（会被构建约束丢弃）。

## 测试与质量门禁

- 先跑覆盖改动的最小测试（具体包 / 具体 `-run`），再按风险扩大范围；离线优先，需要网络的测试自带 `httptest`。
- 对外行为改动**必须有 characterization 覆盖**（`make char`），或确认等价重构且 char 全绿。
- 覆盖率按**角度**驱动、不是凑行 %：对每个函数走查 [`docs/TESTING.md`](docs/TESTING.md) §2 角度表
  （边界 / 错误路径 / nil-零值 / 并发 / 契约 / 副作用…），补「能真出错」的角度；边界必测、错误分支断言
  「是哪个错」（`errors.As`/`errors.Is`）；禁止「调一次不断言」凑覆盖。
- 核心库（除 `examples/`）行覆盖率硬门禁（`make cover`，`MIN_COVERAGE` 目标 ≥98%，只许上调）；
  逐包定位短板用 `make cover-pkg`。
- 并发回归跑 `make race`（需要 C 编译器）。
- 合入口径：本地 `make ci`（= `check` + `lint-new` + `race` + `char`）与 CI 对齐；轻量自检 `make check`。
- 如实报告实际跑了哪些命令与结果；没跑的说明原因，不得声称未执行的测试通过。

## 强制门禁（不是靠自觉）

模型指令是软约束，真正拦得住的是这两层，且与用哪个模型无关：

1. **本地 git 钩子**（`make hooks` 一键装，走 `.githooks/` + `core.hooksPath`，零第三方依赖）：
   - `pre-commit`：`gofmt`、`go vet`、`golangci-lint --new-from-rev`、`go mod tidy -diff`、密钥扫描。
   - `commit-msg`：校验 `<type>(<scope>): 摘要` 约定。
   - `pre-push`：`make race char cover`。
   - 应急可 `--no-verify` 绕过，但**禁止常规使用**；绕过后 CI 仍会拦。
2. **CI**（服务端，`--no-verify` 绕不过）：`.github/workflows/ci.yml` 跑全量门禁 + 密钥扫描 + 提交规范 + 漏洞扫描。独立开发靠「本地钩子 + CI」即完整闭环；想再要「不绿不许合 main」可选配分支保护（approvals=0、不要 code-owner 审核），步骤见 `docs/ENGINEERING_GOVERNANCE.md`。

## Git 与 Pull Request 规范

- 未经明确要求，不创建提交、不推送、不改写历史。当前在默认分支（`main`）时先开分支。
- 非合并提交标题 `<type>(<scope>): <中文摘要>`；`type` 仅用 `feat`、`fix`、`refactor`、`test`、`docs`、`perf`、`build`、`ci`、`chore`。仅仓库级杂项可省略 `scope`。
- 摘要说清主要变化与可观察结果，避免「更新代码」「修复问题」。实质性提交空一行后用 2–6 条项目符号说明背景、行为变化、兼容边界；涉及接口写清方法、路径、输入输出与错误语义。
- 正文末段统一写真实验证：`测试：<命令及结果>`；未运行写 `测试：未运行（原因）`。
- 不虚构作者、邮箱及 `Co-Authored-By` 等会话尾注；只有真实参与者明确要求才署名。
- 每个提交只含一个完整、可独立回滚的逻辑变更。提交前 `git diff --cached` 复核范围与敏感信息，提交后 `git show -s --format=%B HEAD` 核对。
- PR 含目的摘要、真实测试证据、关联问题、破坏性变更与升版本说明；按 `.github/pull_request_template.md` 勾选清单。

## 通用安全边界

- 不把密码、Token、Cookie、Session、CSRF、代理凭据、真实账号或设备资料写入源码、测试夹具、日志、补丁与提交记录。密钥扫描（`pre-commit` 与 CI）是硬门禁。
- 环境差异通过环境变量或本机忽略文件提供。除非任务明确需要，不读取或输出 `.env`、凭据文件内容。

## scope 建议

优先沿用包/模块名：`httpx`、`geo`、`logger`、`netproxy`、`traffic`、`versionreg`、`errors`、`examples`、`docs`、`ci`、`build`、`hooks`。
