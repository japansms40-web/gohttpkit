// agentguard 是治理守卫与 AI agent 钩子入口，独立子模块，不进核心库覆盖率。
// gohttpkit 自身与下游仓库（按版本 go install）共用；仓库差异写在各自根目录的 .agentguard.yml。
//
// 子命令：
//
//	agentguard governance [--base REV] [--worktree]   CI / pre-push / agent 收尾共用的治理检查
//	agentguard check-errors   检查生产 Go 源码的直接错误构造与 panic 语法
//	agentguard check-test-layout   检查测试文件与源文件一一对应（foo.go ↔ foo_test.go）
//	agentguard check-pkg-doc   检查每个 Go 包有 doc.go，且文件结构树与实际文件一致
//	agentguard check-style   代码规则：lock-defer / event-decl / panic-placement / root-ctx / helper-placement
//	agentguard check-commit-msg <文件>|--range A..B   提交说明：标题 / scope 白名单 / feat·fix·refactor·perf 的「测试：」行 / 会话尾注
//	agentguard hook --agent claude|cursor|codex --event <事件>   三家 agent 钩子的薄适配
//
// 规范出处：docs/ENGINEERING_GOVERNANCE.md §3（agent 钩子语义）、§4（规则 → 强制手段）。
//
// 文件结构：
//
//	agentguard/
//	├── commit_msg.go    check-commit-msg：提交说明规范（标题、scope、测试行、会话尾注）
//	├── config.go        读取仓库根 .agentguard.yml 的仓库级差异配置
//	├── doc.go           包文档（本文件）
//	├── error_policy.go  check-errors：禁止直接 errors.New / fmt.Errorf 与字符串 panic
//	├── git.go           git 命令封装：取输出、定位仓库根、读指定版本文件
//	├── golangci.go      对比 .golangci.yml 相对基线的放宽
//	├── governance.go    governance 子命令：覆盖率、lint、char、Skip 四类守卫
//	├── hook.go          三家 agent 钩子的薄适配与事件分发
//	├── main.go          子命令分发与 runCheck
//	├── paths.go         编辑 / 读取路径与补丁路径的放行判定
//	├── pkg_doc.go       check-pkg-doc：每个 Go 包有 doc.go 且文件结构树与实际一致
//	├── shell.go         shell 命令拦截规则（--no-verify、强推、tag、凭据等）
//	├── style_policy.go  check-style：锁、事件声明、panic 位置、根 ctx、辅助函数归位
//	└── test_layout.go   check-test-layout：foo.go ↔ foo_test.go 一一对应
package main
