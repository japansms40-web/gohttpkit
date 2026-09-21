# 工程门禁与治理

本文件说明 gohttpkit 的规范**如何被强制执行**，以及如何让 Codex / Cursor / Claude / Gemini 与人类读到同一套规则。
规范内容本身见 [`CODE_STANDARDS.md`](CODE_STANDARDS.md)；贡献流程见 [`../CONTRIBUTING.md`](../CONTRIBUTING.md)。

## 1. 核心原则：软约束 vs 硬门禁

- **软约束**：`AGENTS.md` / `CLAUDE.md` / `GEMINI.md` / `.cursor/rules/*.mdc` 只是**告诉模型该怎么写**，模型可能不遵守，人也可能忽略。它们提升一致性，但**拦不住**不合规代码。
- **硬门禁**：只有两层真正拦得住，且与用哪个模型无关（发生在 git / CI 层，不在模型层）：
  1. **本地 git 钩子**——拦 `git commit` / `git push`，秒级反馈。可被 `--no-verify` 绕过 ⇒ 定位「防手滑」。
  2. **CI + 分支保护**——服务端，`--no-verify` 绕不过，**不绿 merge 不了** ⇒ 「防故意」的最终防线。

> 独立开发：**本地钩子是主门禁**（防手滑、防泄密），CI 兜底跨平台与 Linux 上的 race；分支保护是可选加强（见 §6）。等有了协作者再上强制审核。

## 2. 跨 agent 单一事实源

| 文件 | 谁读 | 说明 |
|---|---|---|
| `AGENTS.md` | Codex、GitHub Copilot、新版 Cursor、人 | **唯一正本**，改这里即改全部 |
| `CLAUDE.md` | Claude Code | 符号链接 → `AGENTS.md` |
| `GEMINI.md` | Gemini CLI | 符号链接 → `AGENTS.md` |
| `.cursor/rules/*.mdc` | Cursor | 目录级细化（`geo.mdc`、`httpx.mdc`），glob 触发 |

- 一处修改、处处生效；diff 只落在 `AGENTS.md`。
- `make agents-sync-check`（也在 CI 的 test job 跑）校验软链没被误改成实体文件或指错目标。

## 3. 三层门禁总览

| 层 | 载体 | 跑什么 | 能否绕过 |
|---|---|---|---|
| 本地 pre-commit | `.githooks/pre-commit` | gofmt / go vet / 增量 lint / go mod tidy / 密钥扫描 | `--no-verify`（禁止常规用） |
| 本地 commit-msg | `.githooks/commit-msg` | `<type>(<scope>): 摘要` 规范 | `--no-verify` |
| 本地 pre-push | `.githooks/pre-push` | `make race char cover(≥90%)` | `--no-verify` |
| 服务端 CI | `.github/workflows/ci.yml` | 上述全部 + govulncheck + gitleaks + 提交规范 | **不可绕过（配分支保护后）** |

## 4. 本地钩子（零第三方依赖）

安装（clone 后跑一次）：

```bash
make hooks     # = git config core.hooksPath .githooks + chmod +x
```

- 走 `.githooks/` + `core.hooksPath`，**不需要 lefthook / husky / pre-commit 框架**，只依赖 `go`、`gofmt`、（可选）`golangci-lint`、`gitleaks`。
- 密钥扫描：装了 `gitleaks` 就用它（更准）；没装则退回内建 grep 规则。行内 `// notsecret` 或 `gitleaks:allow` 可豁免误报。
- 关闭：`git config --unset core.hooksPath`。
- 应急绕过：`git commit --no-verify` / `git push --no-verify`——**只在紧急情况**，且 CI 仍会拦。

> 想换 lefthook（并行执行、更快）也行，把 `.githooks` 的逻辑搬进 `lefthook.yml` 即可，门禁语义不变。

## 5. CI（`.github/workflows/ci.yml`）

| job | 内容 |
|---|---|
| `test` | agents-sync + build + vet + tidy + cover(≥90%) + race + examples |
| `lint` | golangci-lint 全量 |
| `vuln` | `govulncheck ./...` 依赖漏洞 |
| `secrets` | gitleaks 密钥扫描 |
| `commits` | PR 内每条提交标题符合 `<type>(<scope>): 摘要` |

## 6. 分支保护（独立开发：可选）

独立开发日常有两种够用的做法，按自己习惯选一种，**都不需要**「代码审核 / approvals / CODEOWNERS」——那是给多人协作的，独立开发开了只会把自己锁死（GitHub 不让你批准自己的 PR）。

**做法 A：直接在 main 上干活（最省事）**
- 本地钩子（pre-commit / pre-push）已经拦住绝大多数问题；push 后 CI 再兜一道。
- 不配任何分支保护。适合快速迭代。

**做法 B：也想要「CI 不绿不许合进 main」**
只在 GitHub 网页配一次（Settings → Branches → Add branch ruleset，Target 选 `main`）：
1. 勾 **Require status checks to pass before merging**，把 `test`、`lint`、`vuln`、`secrets`、`commits` 设为必需（首次要先让 CI 跑过一次它们才出现在候选列表）。
2. **不要**勾 Require approvals（或把数量设为 **0**）、**不要**勾 Require review from Code Owners——否则你自己的 PR 永远合不了。
3. 可选勾 Require a pull request before merging（那就得走 PR 流程：开分支 → PR → CI 绿 → 自己合）。想直接 push main 就别勾这条。

> 结论：独立开发把**本地钩子 + CI（push 触发）**用好就是完整闭环。分支保护属锦上添花，不是必需。

## 7. 可选的本地增强（都可跳过）

- **本地对齐 CI 工具**：`go install golang.org/x/vuln/cmd/govulncheck@latest`、装 `gitleaks`、装 `golangci-lint`（不装则本地增量 lint 跳过、密钥扫描退回内建规则，CI 仍会全量跑）。
- **gitleaks 授权**：只有「组织(org)名下的私有仓」用 gitleaks-action 才需在 Secrets 配 `GITLEAKS_LICENSE`；**个人账号仓库（公开或私有）免费，无需配置**。

## 8. 常见问题

- **覆盖率门禁能不能降到 80%？** 不能随手降。`MIN_COVERAGE` 只许上调；确需下调在提交说明里单列理由——降的那一刻门禁就成摆设（独立开发尤其要自己盯住，没人替你把关）。
- **gitleaks 误报了我的测试假 token？** 在该行加 `// notsecret`，或把路径加进 `.gitleaks.toml` 的 `allowlist.paths`。生产代码不要这么放行。
- **pre-push 太慢？** 它跑 race+char+cover，本就重。日常小步提交用 commit（只过 pre-commit），push 前一次性过重门禁；应急可 `--no-verify`，但 CI 会补上。
