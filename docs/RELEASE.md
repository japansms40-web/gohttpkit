# 发布规范

> 本库是被 `go get` 引用的库，没有「部署」环节——**发布 = 推一个不可撤回的 tag**。
> Go module proxy 会永久缓存每个已推送的版本，推错只能 `retract`，不能删除或覆盖。所以发布前的检查比发布动作本身重要。
>
> 配套：升版本判断见 [`VERSIONING.md`](VERSIONING.md)；门禁见 [`ENGINEERING_GOVERNANCE.md`](ENGINEERING_GOVERNANCE.md)。

## 1. 角色

- **MUST** 打 tag、推 tag、发 GitHub Release 由人执行。AI 代理只负责准备：CHANGELOG 草稿、升版本建议、检查单结果。
- **MUST** 只从 `main` 的已通过 CI 的提交打 tag；不从个人分支、不从本地未推送提交打。

## 2. 版本号判断

按 [`VERSIONING.md`](VERSIONING.md)。速查：

| 变化 | 0.x 阶段 | ≥1.0 |
|---|---|---|
| 删除 / 改签名 / 改字段类型 / 改默认行为 / 往导出接口加方法 | minor | major |
| 新增导出符号、新增 struct 字段（零值保持旧行为）、新增子包 | minor | minor |
| 仅修 bug、文档、内部重构、性能（对外行为不变） | patch | patch |
| `go.mod` 的 `go` 指令上调 | minor | minor |

- **MUST** 有疑问时取更高一档。「新增导出符号只升 patch」不允许——下游 `go get -u=patch` 预期不会看到新 API。

## 3. 发布前检查单

在待发布提交上逐项完成，结果贴进 Release 说明或发布 PR：

- [ ] CI 全绿（`test` / `lint` / `vuln` / `secrets` / `commits`）
- [ ] 本地 `make ci` 全绿（含 `race`、`char`）
- [ ] `go run ./examples/customchain && go run ./examples/fidelity` 通过
- [ ] API 对比：`apidiff`（`golang.org/x/exp/cmd/apidiff`）与上一个 tag 对比，结果与第 2 节判断一致
      ```bash
      apidiff -m github.com/japansms40-web/gohttpkit@<上一个tag> github.com/japansms40-web/gohttpkit@<待发布提交>
      ```
- [ ] `govulncheck ./...` 无可达漏洞
- [ ] `CHANGELOG.md`：`[Unreleased]` 改成本版本号与日期，分「破坏 / 新增 / 修复 / 弃用 / 安全」
- [ ] `docs/VERSIONING.md` 对应版本段落：「未发布」字样去掉；破坏点写清下游如何迁移
- [ ] 弃用符号：到期该删的已删（见 `CODE_STANDARDS.md` §13），新弃用的有 `// Deprecated:` 与移除版本
- [ ] README 中的示例代码与当前 API 一致

## 4. 发布步骤

```bash
git switch main && git pull --ff-only
git tag -a vX.Y.Z -m "vX.Y.Z"          # 附注 tag，不用轻量 tag
git push origin vX.Y.Z
```

- **MUST** tag 格式 `vMAJOR.MINOR.PATCH`；预发布用 `vX.Y.Z-rc.N`。
- **MUST NOT** 移动、删除后重打已推送的 tag（proxy 已缓存，会造成校验和不一致）。
- **SHOULD** 在 GitHub 上建 Release，正文取 `CHANGELOG.md` 对应段落 + 第 3 节检查结果。

## 5. 发布后验证

在仓库外的空目录执行：

```bash
go mod init smoke && go get github.com/japansms40-web/gohttpkit@vX.Y.Z && go build ./...
```

- **MUST** 能拉取、能编译。失败立即按第 6 节处理。
- **SHOULD** 用 `pkg.go.dev` 确认新版本文档已渲染、`Deprecated` 标记生效。

## 6. 回滚、hotfix 与 retract

库版本无法「下线」，只能**往前发**：

- **有问题的版本**：在 `go.mod` 加 `retract vX.Y.Z // 原因`，随修复版本一起发布（retract 声明本身须在更高版本里）。
  下游 `go get -u` 会跳过被撤回的版本，`go list -m -retracted` 可见原因。
- **hotfix**：从出问题的 tag 开 `release/vX.Y` 分支 → 修复（先写复现测试）→ 发 `vX.Y.(Z+1)` → 修复 cherry-pick 回 `main`。
  0.x 阶段一般直接在 `main` 修并发 patch。
- **安全问题**：按 [`../SECURITY.md`](../SECURITY.md) 私下处理，修复版本发布后再公开细节。
- **MUST** 每次 retract / hotfix 在 `CHANGELOG.md` 记录原因与受影响版本范围。

## 7. Go 版本支持

- **MUST** 只使用当前最新稳定版 Go。`go.mod` 的 `go` 指令与 CI（`go-version: stable`）保持为这一版；官方放出新的稳定版就上调，不保留上一档 minor。
- **MUST** 上调 `go.mod` 的 `go` 指令按 minor 发布，并在 CHANGELOG 写明。直接依赖同样保持最新稳定版，用模块工具升级，不手写过期版本。
