package main

import (
	"path/filepath"
	"strings"
)

// protectedPaths 是门禁基础设施：agent 改它们等于改裁判，必须由人确认。
// 以 / 结尾表示目录前缀，否则为精确的仓库相对路径。
var protectedPaths = []string{
	".githooks/",
	".github/workflows/",
	".golangci.yml",
	"tools/agentguard/",
	"scripts/agent-guard.sh",
	".claude/settings.json",
	".cursor/hooks.json",
	".codex/",
}

// evalEdit 裁决一次写文件。
// 输入 path：目标路径（绝对或相对 cwd）；allowInfra：人工放行标记是否存在。
// 返回：凭据文件与 .git 内部 → deny；门禁基础设施 → ask（有放行标记则 allow）；仓库外路径 → allow（不归本仓管）。
func evalEdit(path, cwd, root string, allowInfra bool) verdict {
	p := resolvePath(path, cwd)
	if isSecretPath(p) {
		return denyf("不得读写凭据文件（" + path + "），见 AGENTS.md「通用安全边界」")
	}
	if !within(p, root) {
		return allowVerdict
	}
	rel, _ := filepath.Rel(root, p)
	rel = filepath.ToSlash(rel)
	if rel == ".git" || strings.HasPrefix(rel, ".git/") {
		return denyf("不得直接改写 .git 内部文件（" + rel + "）")
	}
	if allowInfra {
		return allowVerdict
	}
	for _, pp := range protectedPaths {
		if rel == pp || (strings.HasSuffix(pp, "/") && strings.HasPrefix(rel, pp)) {
			return verdict{Decision: ask, Reason: rel + " 属于门禁基础设施（钩子 / CI / lint / 守卫），改动须用户确认。" +
				"非 Claude 的 agent 请让用户执行 touch \"$(git rev-parse --git-path " + allowMarkerName + ")\" 后重试，改完删除该文件"}
		}
	}
	return allowVerdict
}

// evalRead 裁决一次读文件：只拦凭据文件。
func evalRead(path string) verdict {
	if isSecretPath(path) {
		return denyf("不得读取凭据文件（" + path + "），见 AGENTS.md「通用安全边界」")
	}
	return allowVerdict
}

// patchPaths 从 Codex apply_patch 文本里取出涉及的文件路径（Add / Update / Delete / Move to）。
func patchPaths(patch string) []string {
	var out []string
	for _, l := range strings.Split(patch, "\n") {
		for _, pre := range []string{"*** Add File: ", "*** Update File: ", "*** Delete File: ", "*** Move to: "} {
			if strings.HasPrefix(l, pre) {
				out = append(out, strings.TrimSpace(strings.TrimPrefix(l, pre)))
			}
		}
	}
	return out
}

func isPatch(cmd string) bool {
	return strings.HasPrefix(strings.TrimSpace(cmd), "*** Begin Patch")
}
