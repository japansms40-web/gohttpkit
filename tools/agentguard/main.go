// agentguard 是 gohttpkit 的治理守卫与 AI agent 钩子入口，独立子模块，不进核心库覆盖率。
//
// 子命令：
//
//	agentguard governance [--base REV] [--worktree]   CI / pre-push / agent 收尾共用的治理检查
//	agentguard hook --agent claude|cursor|codex --event <事件>   三家 agent 钩子的薄适配
//
// 规范出处：docs/ENGINEERING_GOVERNANCE.md §3（agent 钩子语义）、§4（规则 → 强制手段）。
package main

import (
	"fmt"
	"os"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

// run 分发子命令。
// 输入 args：去掉程序名的命令行参数。
// 返回：进程退出码。0 放行；1 治理检查不通过或用法错误；2 钩子拦截（三家 agent 都把 2 视为阻断）。
func run(args []string) int {
	if len(args) == 0 {
		usage()
		return 1
	}
	switch args[0] {
	case "governance":
		return runGovernance(args[1:])
	default:
		usage()
		return 1
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "用法：agentguard governance [--base REV] [--worktree] | agentguard hook --agent claude|cursor|codex --event EVENT")
}
