package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestGitRaw_超时即失败(t *testing.T) {
	dir := t.TempDir()
	initRepo(t, dir, map[string]string{"a.txt": "a\n"})
	if _, err := gitRaw(dir, "rev-parse", "HEAD"); err != nil {
		t.Fatalf("正常超时下应成功: %v", err)
	}

	old := gitTimeout
	gitTimeout = time.Nanosecond
	t.Cleanup(func() { gitTimeout = old })

	out, err := gitRaw(dir, "rev-parse", "HEAD")
	t.Logf("超时 out=%q err=%v", out, err)
	if err == nil {
		t.Fatal("超过 gitTimeout 应返回错误，git 卡住时不能无限等待")
	}
}

// realPath 解开符号链接（macOS 的临时目录 /var 是 /private/var 的链接），便于比较路径。
func realPath(t *testing.T, p string) string {
	t.Helper()
	r, err := filepath.EvalSymlinks(p)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestRepoRoot_子目录返回仓库根(t *testing.T) {
	dir := t.TempDir()
	initRepo(t, dir, map[string]string{"a.txt": "a\n"})
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0o750); err != nil {
		t.Fatal(err)
	}
	got, err := repoRoot(sub)
	if err != nil {
		t.Fatal(err)
	}
	if realPath(t, got) != realPath(t, dir) {
		t.Fatalf("repoRoot(sub)=%q，应为仓库根 %q", got, dir)
	}
}

func TestRepoRoot_钩子导出GIT_DIR时仍返回仓库根(t *testing.T) {
	dir := t.TempDir()
	initRepo(t, dir, map[string]string{"a.txt": "a\n"})
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0o750); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_DIR", filepath.Join(realPath(t, dir), ".git"))
	got, err := repoRoot(sub)
	if err != nil {
		t.Fatal(err)
	}
	if realPath(t, got) != realPath(t, dir) {
		t.Fatalf("GIT_DIR 环境下 repoRoot(sub)=%q，应为仓库根 %q（不能返回子目录）", got, dir)
	}
}
