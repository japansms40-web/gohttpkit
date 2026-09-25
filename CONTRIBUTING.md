# 贡献指南

本库是「导出即契约」的 Go HTTP 基建。新符号默认小写；确需导出的，doc comment 必须写明给谁用、什么场景。规范全文见 [`docs/CODE_STANDARDS.md`](docs/CODE_STANDARDS.md)，测试规范见 [`docs/TESTING.md`](docs/TESTING.md)，版本策略见 [`docs/VERSIONING.md`](docs/VERSIONING.md)，发布流程见 [`docs/RELEASE.md`](docs/RELEASE.md)，安全问题请按 [`SECURITY.md`](SECURITY.md) 私下报告。

## 首次上手

clone 后先跑一次 `make hooks`，安装本地 git 门禁（pre-commit / commit-msg / pre-push）。

## 提交前清单

本地请按顺序跑（或一条 `make ci` 对齐 CI）：

- [ ] `make check` —— build + vet + cover（核心库 ≥ `MIN_COVERAGE`，当前 98%）+ tidy
- [ ] `make lint-new` —— 只对增量严格（`golangci-lint --new-from-rev`）
- [ ] `make race` —— 并发回归（需要 C 编译器）
- [ ] `make char` —— 对外行为锁定；改拦截器链 / 发头 / 解压 / 状态语义之前必跑
- [ ] `make governance` —— 治理守卫：覆盖率棘轮、lint 放宽、char 删改、新增 `t.Skip`（pre-push 与 CI 也会跑）

## 评审要点（按规范章节）

对照 [`docs/CODE_STANDARDS.md`](docs/CODE_STANDARDS.md)：

1. **包设计**：导出即契约；默认链顺序即语义；业务判断不进默认链。
2. **API 设计**：IO 函数第一参是 `context.Context`；超过 2 个业务字段用结构体。
3. **并发模型**：含锁类型写清谁共享、哪把锁保护哪些字段；`make race` 过是合入门禁。
4. **错误处理**：本库错误必须是带字段的类型，对比用 `errors.As`，禁止哨兵冒充身份。
5. **日志**：生产代码走 `logger` 门面；协议 body 经 `TruncateBodyForLog`。
6. **注释**：输入 / 返回写清；导出符号写给谁用。
7. **测试**：对外行为改动必须有 characterization；按角度补测（边界 / 错误路径 / nil / 并发…），
   错误分支断言「是哪个错」，覆盖率过门禁。完整规范见 [`docs/TESTING.md`](docs/TESTING.md)；测试日志用 `t.Log`。
8. **常量、枚举与魔法值**：闭合集合用 `type` 枚举；header 名 / 日志 key 用具名 const；状态码用 `StatusClass` 或 stdlib 常量。禁止在分支、拼装、打点处再写同名字面量。

## 破坏性变更

导出符号的类型、字段或行为变了，必须升版本（0.x 阶段升 minor）。见 [`docs/VERSIONING.md`](docs/VERSIONING.md)。

## 评审补充（第 10–16 章）

9. **安全**：不默认关 TLS 校验；外部输入有上限；代理凭据不进日志。
10. **资源**：Body 所有权明确；goroutine 有退出路径；请求路径不 panic。
11. **依赖**：新增须论证 + 许可证白名单；升级单独提交。
12. **API 演进**：删改导出符号先 `Deprecated` 一个 minor；每包有包级文档。
13. **lint 豁免**：`//nolint:<linter> // 理由`，不为过门禁放宽配置。

## AI 辅助贡献

允许使用 Claude / Cursor / Codex 等代理，但产出按人写的标准评审。代理须遵守 [`AGENTS.md`](AGENTS.md)「AI 代理硬性纪律」；PR 模板的「AI 参与声明」必填。
