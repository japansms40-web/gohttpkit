<!-- 标题遵循 <type>(<scope>): 中文摘要；type 仅限 feat fix refactor test docs perf build ci chore。 -->

## 摘要

<!-- 写清为什么改，而不是列文件名。 -->

## 清单

- [ ] 新增导出符号：doc comment 写清「给谁用、什么场景」（规范第 2 / 7 章）
- [ ] 对外行为有对应 characterization 测试，或确认本次是等价重构且 `make char` 全绿（规范第 8 章）
- [ ] 新增/改动的函数已按角度补测（边界 / 错误路径 / nil / 并发…），错误分支断言「是哪个错」（`docs/TESTING.md`）；覆盖率过 `make cover` 门禁
- [ ] 没有在生产代码里引入裸魔法值（header 名 / 日志字段 key / encoding / 状态码区间）；该用枚举或具名 const（规范第 9 章）
- [ ] 破坏性变更已升版本（0.x 升 minor），并在 `docs/VERSIONING.md` 记录
- [ ] 未提交任何密钥 / 凭据 / 真实账号资料（`pre-commit` 与 CI gitleaks 会拦）
- [ ] 用户可见变化已记入 `CHANGELOG.md` 的 `[Unreleased]`（纯内部重构 / 测试可勾「不适用」）
- [ ] 未下调 `MIN_COVERAGE`、未新增 lint 豁免；新增 `//nolint` 均指定 linter 并写理由（规范第 14 章）
- [ ] 新增依赖已说明理由与许可证（规范第 12 章）；无新增则勾此项
- [ ] 删除或修改 characterization 断言的，已在摘要标注「行为变更」（`docs/TESTING.md` §11）

## AI 参与声明

<!-- 未使用 AI 写「无」。 -->

- 使用的代理 / 模型：<!-- 如 Claude Code、Cursor + GPT、Codex CLI -->
- AI 产出范围：<!-- 哪些文件 / 哪些部分由 AI 生成 -->
- 人工复核：<!-- 逐行读过哪些部分；哪些只看了测试结果 -->

## 回滚方案

<!-- 一般为「revert 本 PR」；涉及发布 / 数据格式 / 导出 API 时写清下游影响与 retract 计划（docs/RELEASE.md §6）。 -->

## 验证

- [ ] `make hooks`（首次贡献者：安装本地 git 门禁）
- [ ] `make check`
- [ ] `make lint-new`
- [ ] `make race`
- [ ] `make char`
- [ ] `make governance`

```
测试：<粘贴实际命令与结果；未运行的写「未运行（原因）」>
```
