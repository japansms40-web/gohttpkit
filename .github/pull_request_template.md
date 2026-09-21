## 摘要

<!-- 写清为什么改，而不是列文件名。 -->

## 清单

- [ ] 新增导出符号：doc comment 写清「给谁用、什么场景」（规范第 2 / 7 章）
- [ ] 对外行为有对应 characterization 测试，或确认本次是等价重构且 `make char` 全绿（规范第 8 章）
- [ ] 没有在生产代码里引入裸魔法值（header 名 / 日志字段 key / encoding / 状态码区间）；该用枚举或具名 const（规范第 9 章）
- [ ] 破坏性变更已升版本（0.x 升 minor），并在 `docs/VERSIONING.md` 记录

## 验证

- [ ] `make check`
- [ ] `make lint-new`
- [ ] `make race`
- [ ] `make char`
