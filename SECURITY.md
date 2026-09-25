# 安全策略

## 支持的版本

| 版本 | 是否接收安全修复 |
|---|---|
| 最新 minor（当前 v0.5.x） | ✅ |
| 更早版本 | ❌ 请升级 |

0.x 阶段只维护最新 minor；1.0 之后维护最近两个 minor。

## 报告漏洞

**不要**在公开 issue、PR 或讨论区披露漏洞细节。

请通过 GitHub 仓库的 **Security → Report a vulnerability**（私有安全通告）提交，内容包括：

- 受影响版本与包（如 `httpx`、`netproxy`）
- 复现步骤或最小复现代码（请勿附真实凭据、真实账号、抓包原文）
- 影响评估（信息泄露 / 拒绝服务 / 绕过 TLS 校验等）

## 响应时限

| 阶段 | 目标 |
|---|---|
| 确认收到 | 3 个工作日内 |
| 初步评估与定级 | 7 个工作日内 |
| 修复发布 | 高危 30 天内，其余随下一个版本 |

修复按 [`docs/RELEASE.md`](docs/RELEASE.md) §6 发布 patch，必要时对受影响版本 `retract`；修复发布后在 GitHub Security Advisory 与 `CHANGELOG.md`「安全」分组公开。

## 本仓库自身的安全基线

- 密钥扫描：本地 `pre-commit` + CI `gitleaks`，硬门禁。
- 依赖漏洞：CI `govulncheck ./...`，硬门禁。
- 编码要求见 [`docs/CODE_STANDARDS.md`](docs/CODE_STANDARDS.md) §10。
