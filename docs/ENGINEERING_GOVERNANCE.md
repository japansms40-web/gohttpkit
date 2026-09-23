# 工程门禁与治理

本文件说明 gohttpkit 的规范**如何被强制执行**，以及如何让 Codex（GPT）/ Cursor（任意模型）/ Claude / Gemini 与人类读到同一套规则。
规范内容本身见 [`CODE_STANDARDS.md`](CODE_STANDARDS.md)、[`TESTING.md`](TESTING.md)；发布见 [`RELEASE.md`](RELEASE.md)；贡献流程见 [`../CONTRIBUTING.md`](../CONTRIBUTING.md)。

## 1. 核心原则：软约束 vs 硬门禁

- **软约束**：`AGENTS.md` / `CLAUDE.md` / `GEMINI.md` / `.cursor/rules/*.mdc` 只是**告诉模型该怎么写**，模型可能不遵守，人也可能忽略。它们提升一致性，但**拦不住**不合规代码。
- **硬门禁**：由工具执行、不依赖模型自觉。按离「写代码」的距离分四层，越靠前反馈越快，越靠后越难绕过：
  0. **agent 钩子**——AI 写文件 / 执行命令的当下拦截（Claude / Cursor / Codex 钩子，见 §3）。只管 AI，不管人。
  1. **本地 git 钩子**——拦 `git commit` / `git push`，秒级反馈。可被 `--no-verify` 绕过 ⇒ 定位「防手滑」。
  2. **CI**——服务端，`--no-verify` 绕不过。
  3. **分支保护**——**不绿 merge 不了**，「防故意」的最终防线（独立开发可选，见 §8）。

> 原则：**每条 MUST 都应能在 §4 对照表里找到至少一个机器强制手段**；找不到的就是「只靠自觉」，要么补工具，要么在评审清单里显式列出。

## 2. 跨 agent 单一事实源

| 文件 | 谁读 | 说明 |
|---|---|---|
| `AGENTS.md` | Codex CLI（GPT）、Cursor、GitHub Copilot、人 | **唯一正本**，改这里即改全部 |
| `CLAUDE.md` | Claude Code | 符号链接 → `AGENTS.md` |
| `GEMINI.md` | Gemini CLI | 符号链接 → `AGENTS.md` |
| `.cursor/rules/00-core.mdc` | Cursor（`alwaysApply`） | 每次会话必带：指向 AGENTS.md + AI 禁止事项摘要 |
| `.cursor/rules/<pkg>.mdc`、`testing.mdc` | Cursor（glob 触发） | 目录 / 文件类型级细化，只写本目录特有约定，不复制正本 |

- 一处修改、处处生效；规则正文只落在 `AGENTS.md` 与 `docs/`，`.mdc` 只放摘要 + 链接。
- `make agents-sync-check`（CI test job 跑）校验软链没被误改成实体文件或指错目标。
- **MUST** 在 Cursor 里切换到 GPT / Claude / 其它模型时规则不变——规则挂在仓库文件上，不挂在模型或个人设置上。
  个人 User Rules 不得与仓库规则冲突；冲突时以仓库为准。

### 各工具的读取路径与强制手段

| 工具 | 读哪些规则 | 第 0 层（agent 钩子）载体 | 状态 |
|---|---|---|---|
| Claude Code | `CLAUDE.md` → `AGENTS.md` | `.claude/settings.json`：`PreToolUse`（Bash / Edit / Write / Read）、`PostToolUse`、`Stop` | ✅ |
| Cursor（含 GPT / Claude 模型） | `AGENTS.md` + `.cursor/rules/*.mdc` | `.cursor/hooks.json`：`beforeShellExecution` / `beforeReadFile` / `preToolUse(Write)` / `afterFileEdit` / `stop` | ✅ |
| Codex CLI（GPT） | `AGENTS.md`（支持子目录嵌套 `AGENTS.md`） | `.codex/hooks.json`：`PreToolUse`（Bash / apply_patch）、`PostToolUse`、`Stop`；`.codex/config.toml`：沙箱与审批 | ✅（需信任项目并在 `/hooks` 审核一次） |
| Gemini CLI | `GEMINI.md` → `AGENTS.md` | 依赖第 1、2 层 | — |

## 3. 第 0 层：agent 钩子

三家 agent 钩子**语义一致**：裁决逻辑全在 `tools/agentguard`（独立 Go 子模块，带测试），入口是 `scripts/agent-guard.sh`
（首次或源码变更后自动编译到 `.git/agent-guard/`，之后单次约 40ms）；`.claude/settings.json`、`.cursor/hooks.json`、`.codex/hooks.json`
只是把各家原生事件映射到 `agentguard hook --agent <名> --event <事件>` 的薄适配。

| 时机 | 动作 | 拦截 / 执行 |
|---|---|---|
| 执行 shell 前 | 拒绝 | `--no-verify` / `git commit -n`；强推（`-f`、`--force-with-lease`、`+refspec`）；删远端引用；推 / 建 / 删 tag；`git reset --hard`；`git clean -f`；改 `core.hooksPath`；`rm -r` 仓库外（系统临时目录除外）/ 仓库根 / `.git` / 根目录通配 |
| 执行 shell 前 | 拒绝 | 在 `main` / `master` 上提交；agent 在提交说明里自写豁免标记 |
| 读写文件前 | 拒绝 | `.env*`（`.example` / `.sample` / `.template` 除外）、私钥类文件；`.git/` 内部；用 touch / rm / mv 等命令触碰放行标记 |
| 改文件前 | 需人确认 | 门禁基础设施：`.githooks/`、`.github/workflows/`、`.golangci.yml`、`tools/agentguard/`、`scripts/agent-guard.sh`、三家钩子配置。Claude 弹权限框；Cursor / Codex 无 ask，直接拒绝 |
| 改文件后 | 执行 | `.go` 文件 `gofmt -w` + `go vet` 所在包；改到 Makefile / `.golangci.yml` / 测试文件时跑治理守卫。问题回灌给 agent |
| 回合结束前 | 执行 | 治理守卫（`--worktree`，对比 merge-base）；有未提交 Go 改动时 `make check`；改到 `tools/agentguard` 时 `make tools-check`。失败要求 agent 继续修，**连续 3 次**仍失败则放行并提示人（防死循环） |

**人工放行**：确需让 agent 改门禁基础设施或下调门槛时，由人执行
`touch "$(git rev-parse --git-path agent-guard-allow)"`，完成后删除该文件。标记存在期间：受保护文件可改、收尾的治理违规不再拦截；
pre-push 与 CI 的治理守卫仍然生效，提交说明里仍需人写豁免标记与理由。

**首次启用**：Claude Code 打开项目即生效；Cursor 读取 `.cursor/hooks.json` 即生效；Codex 需先信任本项目，再在 `/hooks` 里审核一次。
三家都需要本机有 `go`（启动器会自动编译守卫）。

> 定位：第 0 层防手滑，不防对抗——shell 解析只处理引号、转义、`&& || ; |` 串联和 `bash -c` 嵌套，变量展开等绕得过去；
> 真正不可绕过的是第 2、3 层。钩子出错（如编译失败）时放行，不会把 agent 卡死。

## 4. 规则 → 强制手段对照表

状态：✅ 已强制 ｜ 🟡 部分 / 间接 ｜ ⬜ 待落地（见 §7）｜ 👁 仅评审

| 规则（出处） | 强制手段 | 状态 |
|---|---|---|
| gofmt / go vet | pre-commit、CI test | ✅ |
| lint 全量规则（errorlint、bodyclose、noctx、gosec、gocyclo、goconst、revive…） | pre-commit 增量、CI lint 全量 | ✅ |
| 禁止裸 `fmt.Print` / `log.*` / `slog.*`（CS §6） | `forbidigo` | ✅ |
| 覆盖率 ≥ `MIN_COVERAGE`=98（TESTING §1） | pre-push、CI `make cover` | ✅ |
| `MIN_COVERAGE` 只许上调 | 治理守卫：agent 收尾、pre-push、CI `governance` | ✅ |
| 并发无竞态（CS §4） | pre-push、CI `go test -race` | ✅ |
| characterization 行为锁定（CS §8） | pre-push、CI `make char` | ✅ |
| characterization 用例不得删 / 改断言迁就实现（TESTING §11） | 治理守卫：删改 `httpx/characterization_test.go` 既有行须标注「行为变更」 | ✅ |
| go.mod 整洁、无漏洞依赖（CS §12） | pre-commit、CI `tidy -diff`、`govulncheck` | ✅ |
| 依赖许可证白名单（CS §12） | CI license 检查（`go-licenses check`） | ⬜ |
| 密钥不入库（AGENTS 安全边界） | pre-commit、CI gitleaks | ✅ |
| 提交标题规范 | commit-msg、CI commits | ✅ |
| 提交正文 `测试：` 行 | commit-msg 对 `feat`/`fix`/`refactor`/`perf` 校验 | ⬜ |
| `//nolint` 必须指定 linter + 理由（CS §14） | `nolintlint`（pre-commit 增量、CI 全量） | ✅ |
| 不放宽 `.golangci.yml`（CS §14） | agent 钩子（改前需人确认）+ 治理守卫（关 linter / 加豁免 / 调松阈值） | ✅ |
| goroutine 不泄漏（CS §11、TESTING §8） | `goleak` in `TestMain` | ⬜ |
| 解压有大小上限（CS §10） | 代码实现 + characterization | ⬜ |
| 破坏性变更升版本（VERSIONING） | CI `apidiff` 对比上一个 tag | ⬜ |
| 类型错误非哨兵（CS §5） | `errorlint` 部分；自定义检查 `var Err… = errors.New` | 🟡 |
| panic 仅限 `Must*` / 初始化（CS §11） | 👁 评审；可选 `forbidigo` 规则按路径放行 | 👁 |
| 注释「输入：/返回：」、导出符号「给谁用」（CS §7） | `revive exported` 管存在性；格式靠评审 | 🟡 |
| IO 函数首参 `ctx`（CS §3） | `noctx` 管 HTTP；其余评审 | 🟡 |
| 状态码 / header / 日志 key 无裸字面量（CS §9） | `goconst`(min=2) | 🟡 |
| 不新增 `t.Skip` 屏蔽用例（TESTING §10） | 治理守卫 | ✅ |
| AI 不绕过门禁、不自行打 tag、不在 main 提交、不碰凭据（AGENTS） | agent 钩子（Claude / Cursor / Codex） | ✅ |
| examples 可运行 | CI test | ✅ |
| 跨 agent 规则同源 | `make agents-sync-check`（CI） | ✅ |

## 5. 本地钩子（零第三方依赖）

安装（clone 后跑一次）：

```bash
make hooks     # = git config core.hooksPath .githooks + chmod +x
```

| 钩子 | 跑什么 |
|---|---|
| `pre-commit` | gofmt / go vet / 增量 lint / go mod tidy / 密钥扫描 |
| `commit-msg` | `<type>(<scope>): 摘要` 规范 |
| `pre-push` | `make governance race char cover`（覆盖率门禁 = `MIN_COVERAGE`，当前 98%） |

- 走 `.githooks/` + `core.hooksPath`，**不需要 lefthook / husky / pre-commit 框架**，只依赖 `go`、`gofmt`、（可选）`golangci-lint`、`gitleaks`。
- 密钥扫描：装了 `gitleaks` 就用它（更准）；没装则退回内建 grep 规则。行内 `// notsecret` 或 `gitleaks:allow` 可豁免误报。
- 关闭：`git config --unset core.hooksPath`。
- 应急绕过：`git commit --no-verify` / `git push --no-verify`——**只在紧急情况**，且 CI 仍会拦。**AI 代理禁止使用。**

## 6. CI（`.github/workflows/ci.yml`）

| job | 内容 |
|---|---|
| `test` | agents-sync + build + vet + tidy + cover（≥ `MIN_COVERAGE`，当前 98%）+ race + char + examples |
| `governance` | `make tools-check` + 治理守卫（PR 对比 base，push 对比 before；PR 标题 / 正文里的标记同样生效） |
| `lint` | golangci-lint 全量 |
| `vuln` | `govulncheck ./...` 依赖漏洞 |
| `secrets` | gitleaks 密钥扫描 |
| `commits` | PR 内每条提交标题符合 `<type>(<scope>): 摘要` |

**CI 供应链规范**：

- **MUST** 第三方 action 钉到完整 commit SHA（注释写版本号），不用浮动 `@v4`；由 Dependabot 升级。（待落地）
- **MUST** 工具版本钉死（`golangci-lint`、`govulncheck`、`gitleaks`），不用 `latest`——同一提交重跑结果必须一致。（待落地）
- **MUST** workflow 默认 `permissions: contents: read`，job 需要更多权限时单独声明。（✅ 已满足）
- **SHOULD** Go 版本矩阵覆盖 RELEASE §7 规定的支持版本；race 至少在 Linux 跑。

## 7. 待落地清单（规范已定义，工具未实现）

按优先级。每项落地后回到 §4 把状态改成 ✅。

1. ~~CI 显式 `make char`、pre-push 文案~~ ✅ 已落地。
2. ~~`nolintlint`~~ ✅ 已落地。
3. ~~治理守卫~~ ✅ 已落地：`make governance`，接入 pre-push、CI `governance` job、agent 收尾。
4. ~~agent 钩子~~ ✅ 已落地，见 §3。
5. **commit-msg** 校验 `feat`/`fix`/`refactor`/`perf` 正文含 `测试：` 行。
6. **goleak** 接入 `httpx`、`netproxy`、`traffic`、`logger` 的 `TestMain`（新增测试依赖，需确认）。
7. **apidiff** CI job（PR 对比最近 tag，破坏性变化且未升 minor 时失败）+ **release workflow**（tag 触发：全量门禁 → GitHub Release）。
8. **供应链**：actions 钉 SHA、工具钉版本、`.github/dependabot.yml`（gomod + github-actions，每周）、`go-licenses check`。
9. **CI Go 版本矩阵** + nightly 长时 fuzz。
10. **代码差距**：`bodyDecodeInterceptor` 解压大小上限（CS §10）；`versionreg.Registry.Register` 的裸字符串 panic 改为类型错误（CS §11）。
11. **治理守卫扩展**：检测新增 `var Err… = errors.New(` 生产代码（CS §5）。

## 8. 分支保护（独立开发：可选）

独立开发日常有两种够用的做法，按自己习惯选一种，**都不需要**「代码审核 / approvals / CODEOWNERS」——那是给多人协作的，独立开发开了只会把自己锁死（GitHub 不让你批准自己的 PR）。

**做法 A：直接在 main 上干活（最省事）**
- 本地钩子（pre-commit / pre-push）已经拦住绝大多数问题；push 后 CI 再兜一道。
- 不配任何分支保护。适合快速迭代。

**做法 B：也想要「CI 不绿不许合进 main」**
只在 GitHub 网页配一次（Settings → Branches → Add branch ruleset，Target 选 `main`）：
1. 勾 **Require status checks to pass before merging**，把 `test`、`governance`、`lint`、`vuln`、`secrets`、`commits` 设为必需（首次要先让 CI 跑过一次它们才出现在候选列表）。
2. **不要**勾 Require approvals（或把数量设为 **0**）、**不要**勾 Require review from Code Owners——否则你自己的 PR 永远合不了。
3. 可选勾 Require a pull request before merging（那就得走 PR 流程：开分支 → PR → CI 绿 → 自己合）。想直接 push main 就别勾这条。
4. 建议同时勾 **Block force pushes** 与 tag 规则 **Restrict deletions / updates on `v*` tags**（已推送的 tag 不可移动，见 RELEASE §4）。

> 结论：独立开发把**本地钩子 + CI（push 触发）**用好就是完整闭环。分支保护属锦上添花；**多人或多 agent 并行开发时建议启用做法 B**。

## 9. 可选的本地增强（都可跳过）

- **本地对齐 CI 工具**：`go install golang.org/x/vuln/cmd/govulncheck@latest`、装 `gitleaks`、装 `golangci-lint`（不装则本地增量 lint 跳过、密钥扫描退回内建规则，CI 仍会全量跑）。
- **gitleaks 授权**：只有「组织(org)名下的私有仓」用 gitleaks-action 才需在 Secrets 配 `GITLEAKS_LICENSE`；**个人账号仓库（公开或私有）免费，无需配置**。
- **凭据**：git remote 不要写成 `https://user:<token>@github.com/...`（明文落在 `.git/config`）；用 `gh auth login` 或系统凭据助手。

## 10. 常见问题

- **覆盖率门禁能不能降到 80%？** 不能随手降。`MIN_COVERAGE` 只许上调；确需下调在提交说明里单列理由——降的那一刻门禁就成摆设（独立开发尤其要自己盯住，没人替你把关）。
- **gitleaks 误报了我的测试假 token？** 在该行加 `// notsecret`，或把路径加进 `.gitleaks.toml` 的 `allowlist.paths`。生产代码不要这么放行。
- **pre-push 太慢？** 它跑 race+char+cover，本就重。日常小步提交用 commit（只过 pre-commit），push 前一次性过重门禁；应急可 `--no-verify`，但 CI 会补上。
- **AI 说「测试都过了」可信吗？** 只信它贴出的命令与输出；第 0 层 `Stop` 钩子会在有 Go 改动时于收尾前强制跑 `make check`。
- **agent 钩子误拦了正常操作？** 记下命令与裁决理由，在 `tools/agentguard/*_test.go` 补一条用例再修规则；临时需要时按 §3 用放行标记。
