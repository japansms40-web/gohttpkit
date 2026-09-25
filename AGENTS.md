# gohttpkit 协作指南（AGENTS）

> 本文件是所有 AI 编码代理（Codex、Cursor、Claude、Gemini 等）与人类贡献者在本仓库工作的**唯一正本**。
> `CLAUDE.md` / `GEMINI.md` 是指向本文件的符号链接，改这里即改全部；Cursor 另读 `.cursor/rules/*.mdc` 做目录级细化。
> 规范全文见 [`docs/CODE_STANDARDS.md`](docs/CODE_STANDARDS.md)，测试见 [`docs/TESTING.md`](docs/TESTING.md)，贡献流程见 [`CONTRIBUTING.md`](CONTRIBUTING.md)，
> 版本策略见 [`docs/VERSIONING.md`](docs/VERSIONING.md)，发布见 [`docs/RELEASE.md`](docs/RELEASE.md)，
> 门禁与治理（含「规则 → 强制手段」对照表）见 [`docs/ENGINEERING_GOVERNANCE.md`](docs/ENGINEERING_GOVERNANCE.md)。

<!-- shared-rules:start —— insgo 与 gohttpkit 两仓库本段逐字一致，改一处须同步另一处 -->
## 通用协作规范

- 永远使用中文与用户沟通；代码注释、设计说明和提交说明也使用中文，外部协议、既有英文接口或 godoc 惯例要求英文时除外。
- 开始工作前先阅读与任务直接相关的代码、文档和配置。文档与当前实现不一致时，以只读检查和实际行为为准，并明确说明差异。
- 修改前执行 `git status --short`。工作树可能包含用户的 WIP，只处理当前任务需要的文件和 hunk，不覆盖、清理或顺带格式化无关改动。
- 优先完成已授权且可逆的工作。遇到会扩大范围、改变外部状态或需要业务选择的事项，再向用户确认。

## 通用测试与交付

- 先运行覆盖改动的最小检查，再按风险扩大范围；不得为了“全绿”主动触发真实账号、生产网络或外部服务副作用。
- 完成时如实列出实际运行的命令、结果和未运行项目。没有运行测试时说明原因，不得声称未执行的测试通过。
- 文档、配置和符号链接变更至少执行目标可读性检查与 `git diff --check`；技能变更还要校验 `SKILL.md` 结构和脚本语法。

## Git 与 Pull Request 规范

- 未经用户明确要求，不创建提交、不推送远端、不改写历史。
- 非合并提交标题使用 `<type>(<scope>): <中文摘要>`；`type` 仅使用 `feat`、`fix`、`refactor`、`test`、`docs`、`perf`、`build`、`ci`、`chore`。只有仓库级杂项可以省略 `scope`。
- 摘要直接说明主要变化和可观察结果，避免“更新代码”“修复问题”等笼统表述。
- 简单提交可以省略正文。实质性提交在标题后空一行，用 2–6 条项目符号说明背景、行为变化、兼容边界或关键取舍；涉及接口时写明方法、路径、输入输出及错误语义。
- 正文最后一段统一写真实验证结果：`测试：<命令及结果>`；未运行时写 `测试：未运行（原因）`。
- 不复制或虚构作者、邮箱以及 `Co-Authored-By`、`Claude-Session`、`Codex-Session` 等会话尾注；只有真实参与者明确要求时才记录署名。
- 每个提交只包含一个完整、可独立回滚的逻辑变更。提交前用 `git diff --cached` 复核范围和敏感信息，提交后用 `git show -s --format=%B HEAD` 核对最终提交信息。
- Pull Request 应包含目的摘要、真实测试证据、关联问题、配置或迁移说明，并按 `.github/pull_request_template.md` 勾选清单；密钥和环境配置变更要单独标注，可见 UI 变更附截图。

## 通用安全边界

- 不把密码、Token、Cookie、Session、CSRF Token、代理凭据、真实账号或设备资料写入源码、测试夹具、日志、补丁和提交记录。密钥扫描（`pre-commit` 与 CI）是硬门禁。
- 环境差异通过环境变量、本机忽略文件或密钥管理提供。除非任务明确需要，不读取或输出 `.env`、本机代理配置和凭据文件内容。
<!-- shared-rules:end -->

## 分支与 worktree

- **改代码用 worktree，不在主工作区切分支**：动手修改前 `git worktree add .worktrees/<type>-<topic> -b <type>/<topic>`，在该目录内改、测、提交；
  完成后回主工作区 `git switch main && git merge --ff-only <type>/<topic>`（不能快进时先在 worktree 里 `git rebase main`），
  再 `git worktree remove .worktrees/<type>-<topic> && git branch -d <type>/<topic>`。用户明确要求直接在当前工作区改时照做。
- 纯 git 操作（在 `main` 上提交、合并、打 tag）直接在主工作区的 `main` 上做，不需要切分支。

## 项目结构

本库是「导出即契约」的通用 Go HTTP 基建，不绑定任何具体 API。核心包：

- `httpx/`：`Client` + OkHttp 风格拦截器链框架（白名单发头、请求编码、快照、Transport）。入口 `httpx.NewClient`。
- `httpx/interceptor/`：内建拦截器（追踪、日志、解压、缓存、重试、桥接、状态语义、归类、HTML）与预设链；
  默认链在本包 `init()` 里注册，程序里需 import 一次本包（用到任何拦截器即已满足），否则 `httpx.NewClient` 返回 `*httpx.NoDefaultChainError`。
- `geo/`：国家 → Web locale / 时区大数据映射表，一表一文件，keyset 由测试对齐；`geo/locale_mobile/`（包名 `localemobile`）：国家 → Android 5 个 locale 头。
- `logger/`：结构化日志门面，`SetHandler` 注入实现。
- `netproxy/`：代理接入；`traffic/`：TCP 层流量计数。
- `versionreg/`：版本 / 配置注册表。
- `errors/`：**带字段的类型错误**（非哨兵），供调用方 `errors.As` 解出字段。
- `examples/`：`quickstart`（打真实接口）、`customchain` / `fidelity`（自带假服务器，离线可跑，CI 会跑后两个）。
- `tools/agentguard/`：AI 代理钩子与治理守卫（独立子模块，按 `tools/agentguard/vX.Y.Z` 打 tag，insgo 等下游按版本安装）；`scripts/`、`.githooks/`：启动脚本与本地 git 门禁。
- `docs/`：架构、规范、测试、治理、版本、发布与迁移文档。

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
- 合入口径：本地 `make ci`（= `check` + `lint-new` + `race` + `char` + `agents-sync-check` + `governance` + `tools-check`）；轻量自检 `make check`。
  与 CI 的差异：CI 的 lint 是**全量**（本地 `lint-new` 只查增量），CI 另跑 examples、`govulncheck`、密钥扫描与提交规范。
- 如实报告实际跑了哪些命令与结果；没跑的说明原因，不得声称未执行的测试通过。

## 强制门禁（不是靠自觉）

模型指令是软约束，真正拦得住的是下面三层，后两层与用哪个模型无关：

0. **agent 钩子**（`.claude/settings.json`、`.cursor/hooks.json`、`.codex/hooks.json` → `scripts/agent-guard.sh` → `tools/agentguard`）：
   执行命令前拦 `--no-verify`、强推、删 / 移动 / 推送 tag、`reset --hard`、`clean -f`、读写凭据（只放行 / 拒绝，不弹确认框）；
   改文件后 gofmt + vet；回合结束前跑治理守卫，有未提交 Go 改动时跑 `make check`、改到 `tools/agentguard` 时跑 `make tools-check`，
   不过不许收尾（连续 3 次后放行并提示人）。仓库差异（char 用例、主干分支名）写在 `.agentguard.yml`。规则与人工放行方式见 `docs/ENGINEERING_GOVERNANCE.md` §3。
1. **本地 git 钩子**（`make hooks` 一键装，走 `.githooks/` + `core.hooksPath`，零第三方依赖）：
   - `pre-commit`：`gofmt`、`go vet`、`golangci-lint --new-from-rev`、`go mod tidy -diff`、密钥扫描。
   - `commit-msg`：校验 `<type>(<scope>): 摘要` 约定。
   - `pre-push`：`make governance race char cover`。
   - 应急可 `--no-verify` 绕过，但**禁止常规使用**；绕过后 CI 仍会拦。
2. **CI**（服务端，`--no-verify` 绕不过）：`.github/workflows/ci.yml` 跑全量门禁 + 治理守卫 + 密钥扫描 + 提交规范 + 漏洞扫描。独立开发靠「本地钩子 + CI」即完整闭环；想再要「不绿不许合 main」可选配分支保护（approvals=0、不要 code-owner 审核），步骤见 `docs/ENGINEERING_GOVERNANCE.md`。

治理守卫（`make governance`）拦四类改动：`MIN_COVERAGE` 下调或删除、`.golangci.yml` 放宽（关 linter / 加豁免 / 调松阈值 / 删 forbidigo 规则）、`httpx/characterization_test.go` 既有行的删改、新增 `t.Skip` / `b.Skip` / `f.Skip`。确属有意为之时，由**人**在提交说明或 PR 文本里写「治理豁免: <理由>」（放行覆盖率、lint 与 Skip）或「行为变更」（放行 char）；agent 不得自写。

## AI 代理硬性纪律

适用于 Claude Code、Codex / GPT、Cursor（任意模型）及其它代理。违反任一条，产出按不合格处理，不论门禁是否恰好放行。

**标准工作流**（不可跳步）：

1. 读：与任务直接相关的代码、测试、本文件与对应 `docs/`。
2. 计划：多文件或改对外行为时先列改动点与影响面（导出符号、链顺序、错误类型、版本号）。
3. 先测：新行为 / bug 修复先写一条**因正确原因失败**的测试（见 `docs/TESTING.md` §5）。
4. 最小实现：只改任务需要的文件与 hunk。
5. 验证：先跑最小测试（具体包 / `-run`），再按风险扩到 `make check` / `make ci`。
6. 报告：列出实际执行的命令与结果、未执行项及原因、遗留风险。

**禁止事项**（MUST NOT）：

- 为过门禁而下调 `MIN_COVERAGE`、放宽 `.golangci.yml`（新增 `exclusions` / 关 linter / 调高阈值）、新增无理由 `//nolint`。
- 用 `--no-verify`、`t.Skip`、构建标签、改 `-run` 正则等手段绕过门禁或屏蔽失败用例。
- 修改或删除 characterization / 既有断言去迁就新实现；行为确需改变时，先在计划与提交说明（或 PR）中写清，再改测试。
- 新增第三方依赖、改 `go.mod` 的 `go` 版本、改导出符号签名，而未在计划中说明并获用户确认。
- 声称未执行的命令已通过；把「编译通过」当成「测试通过」；以「可能 flaky」为由重跑掩盖失败。
- 将凭据、真实账号、抓包原文写入代码、测试、日志、提交或对话输出。
- 顺带格式化、重命名或「优化」任务范围外的代码。

## 发布

PR 除通用要求外，还要写明破坏性变更与升版本判断（导出即契约，见 `docs/VERSIONING.md`）。
打 tag、写 `CHANGELOG.md`、升版本判断、hotfix / `retract` 流程见 [`docs/RELEASE.md`](docs/RELEASE.md)。AI 代理只在用户明确要求时于 `main` 上直接打附注 tag（不切分支、不开 worktree）；**推送 tag、发 Release 由人执行**，删除 / 移动 tag 一律禁止。

## scope 建议

优先沿用包 / 模块名：`httpx`、`interceptor`、`geo`、`localemobile`、`logger`、`netproxy`、`traffic`、`versionreg`、`errors`、`examples`、`agentguard`；
仓库级用 `docs`、`ci`、`build`、`hooks`、`governance`、`release`、`deps`。
