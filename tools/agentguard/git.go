package main

import (
	"bytes"
	"context"
	"os"
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
// git 在 worktree 里跑钩子会导出 GIT_DIR，此时 show-toplevel 返回当前目录而非仓库根
// （go run -C tools/agentguard 下就是子模块目录，check-* 会静默只扫子目录），
// 所以只对这一条命令剥离 GIT_DIR / GIT_WORK_TREE；其余 git 调用保留钩子环境（GIT_INDEX_FILE 等暂存区判断必需）。
func repoRoot(dir string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), gitTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--show-toplevel")
	cmd.Dir = dir
	cmd.Env = envWithout(os.Environ(), "GIT_DIR", "GIT_WORK_TREE")
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	return strings.TrimSpace(out.String()), err
}

// envWithout 返回去掉指定变量的环境副本，不修改入参与进程级环境。
// 输入 env：KEY=VALUE 形式的环境；keys：要去掉的变量名。
// 返回：过滤后的新切片。
func envWithout(env []string, keys ...string) []string {
	out := make([]string, 0, len(env))
outer:
	for _, kv := range env {
		for _, k := range keys {
			if strings.HasPrefix(kv, k+"=") {
				continue outer
			}
		}
		out = append(out, kv)
	}
	return out
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
