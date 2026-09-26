package main

import (
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
