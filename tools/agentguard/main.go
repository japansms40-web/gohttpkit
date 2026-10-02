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
	case "check-errors":
		return runCheck("check-errors", scanErrorPolicy, "生产代码错误构造符合规范")
	case "check-test-layout":
		return runCheck("check-test-layout", scanTestLayout, "测试文件与源文件一一对应")
	case "check-pkg-doc":
		return runCheck("check-pkg-doc", scanPkgDoc, "每个 Go 包有 doc.go 且文件结构树与实际文件一致")
	case "check-style":
		return runCheck("check-style", scanStyle, "代码规则（锁 / 事件声明 / panic 位置 / 根 ctx / 辅助函数归位）符合规范")
	case "check-commit-msg":
		return runCheckCommitMsg(args[1:])
	case "hook":
		return runHook(args[1:])
	default:
		usage()
		return 1
	}
}

// runCheck 在当前仓库根上跑一个静态检查子命令 name。
// 输入 scan：返回违规行的扫描函数；okMsg：无违规时打到 stdout 的结论。
// 返回：0 无违规；1 不在仓库内、扫描出错或有违规（违规逐行打到 stderr）。
func runCheck(name string, scan func(root string) ([]string, error), okMsg string) int {
	root, err := repoRoot("")
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, name+"：不在 Git 仓库内")
		return 1
	}
	violations, err := scan(root)
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, name+"：检查失败：", err)
		return 1
	}
	for _, v := range violations {
		_, _ = fmt.Fprintln(os.Stderr, v)
	}
	if len(violations) > 0 {
		return 1
	}
	fmt.Println(name + "：" + okMsg)
	return 0
}

func usage() {
	_, _ = fmt.Fprintln(os.Stderr, "用法：agentguard governance [--base REV] [--worktree] | agentguard check-errors | agentguard check-test-layout | agentguard check-pkg-doc | agentguard check-style | agentguard check-agents-docs | agentguard check-commit-msg <文件>|--range A..B | agentguard hook --agent claude|cursor|codex --event EVENT")
}
