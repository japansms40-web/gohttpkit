package main

import (
	"bytes"
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// gitTimeout 单条 git 子命令的超时；git 卡在锁或凭据提示时到点即失败，不让钩子无限等待。
// 是 var 而非 const：测试调小以覆盖超时分支。
var gitTimeout = time.Minute

// gitOut 在 dir 下执行 git 并返回去掉首尾空白的 stdout。
// 输入 dir：工作目录，空串表示当前目录；args：git 子命令与参数。
// 返回：stdout 与执行错误（非零退出码也算错误，stderr 丢弃）。
func gitOut(dir string, args ...string) (string, error) {
	s, err := gitRaw(dir, args...)
	return strings.TrimSpace(s), err
}

// gitRaw 同 gitOut，但不裁剪输出（读文件内容 / diff 时保留原样）；超过 gitTimeout 视为失败。
func gitRaw(dir string, args ...string) (string, error) {
	// CLI 没有上游 ctx，这里是合法的根；与 runCmd 同款超时写法。
	ctx, cancel := context.WithTimeout(context.Background(), gitTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	return out.String(), err
}

// repoRoot 返回 dir 所在仓库的根目录；不在仓库内返回错误。
func repoRoot(dir string) (string, error) {
	return gitOut(dir, "rev-parse", "--show-toplevel")
}

// showAt 读取 rev 版本里的 path。
// 返回：内容与是否存在；rev 里没有该文件（新增文件）时 ok=false。
func showAt(root, rev, path string) (string, bool) {
	s, err := gitRaw(root, "show", rev+":"+path)
	if err != nil {
		return "", false
	}
	return s, true
}

// gitPath 返回 git 目录下 name 的绝对路径（兼容 worktree）。
func gitPath(root, name string) string {
	p, err := gitOut(root, "rev-parse", "--git-path", name)
	if err != nil {
		return ""
	}
	if !filepath.IsAbs(p) {
		p = filepath.Join(root, p)
	}
	return p
}
