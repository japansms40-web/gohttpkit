package main

import (
	"path/filepath"
	"strings"
)

// evalEdit 裁决一次写文件。
// 输入 path：目标路径（绝对或相对 cwd）；cwd：相对路径的基准；root：仓库根。
// 返回：凭据文件与 .git 内部 → deny；其余 → allow。
// 门禁基础设施（钩子 / CI / lint / 守卫）不在这里拦：真正的放宽由治理守卫在改后、收尾、pre-push 与 CI 识别。
func evalEdit(path, cwd, root string) verdict {
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
