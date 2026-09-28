package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// initRepo 在 dir 建 git 仓库，写入 files 并提交为基线。
func initRepo(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for p, s := range files {
		full := filepath.Join(dir, p)
		if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(s), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, args := range [][]string{
		{"init", "-q", "-b", "main"},
		{"config", "commit.gpgsign", "false"},
		{"add", "."},
		{"-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "-m", "base"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
}

func mustGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	s, err := gitOut(dir, args...)
	if err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
	return s
}

func anyContains(vs []violation, sub string) bool {
	for _, v := range vs {
		if strings.Contains(v.Detail, sub) {
			return true
		}
	}
	return false
}

func anyRule(vs []violation, rule string) bool {
	for _, v := range vs {
		if v.Rule == rule {
			return true
		}
	}
	return false
}
