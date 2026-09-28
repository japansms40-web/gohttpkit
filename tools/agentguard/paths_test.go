package main

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestEvalEdit_凭据与git内部拒绝其余放行(t *testing.T) {
	root := t.TempDir()
	cases := []struct {
		name, path, cwd string
		deny            bool
		reason          string
	}{
		{"仓库内普通文件", "httpx/client.go", root, false, ""},
		{"绝对路径普通文件", filepath.Join(root, "a.go"), "/", false, ""},
		{"凭据文件", ".env", root, true, "凭据"},
		{"仓库外凭据也拦", "/tmp/elsewhere/id_ed25519", root, true, "凭据"},
		{"git 目录本身", ".git", root, true, ".git"},
		{"git 内部文件", ".git/hooks/pre-commit", root, true, ".git"},
		{"相对 cwd 解析进 git 内部", "../.git/config", filepath.Join(root, "sub"), true, ".git"},
		{"仓库外普通文件放行", "/tmp/elsewhere/x.go", root, false, ""},
		{"名字像 git 的普通目录", ".github/workflows/ci.yml", root, false, ""},
	}
	for _, c := range cases {
		v := evalEdit(c.path, c.cwd, root)
		t.Logf("%s: path=%q cwd=%q → %v %q", c.name, c.path, c.cwd, v.Decision, v.Reason)
		if (v.Decision == deny) != c.deny {
			t.Errorf("%s: Decision=%v，期望 deny=%v", c.name, v.Decision, c.deny)
		}
		if c.deny && !strings.Contains(v.Reason, c.reason) {
			t.Errorf("%s: 拒绝理由 %q 应提到 %q", c.name, v.Reason, c.reason)
		}
		if !c.deny && v != allowVerdict {
			t.Errorf("%s: 放行应返回 allowVerdict，得到 %+v", c.name, v)
		}
	}
}

func TestEvalRead_只拦凭据文件(t *testing.T) {
	for path, wantDeny := range map[string]bool{
		".env": true, "certs/server.pem": true, ".env.example": false, "docs/README.md": false, ".git/config": false,
	} {
		v := evalRead(path)
		t.Logf("%q → %v %q", path, v.Decision, v.Reason)
		if (v.Decision == deny) != wantDeny {
			t.Errorf("evalRead(%q) Decision=%v，期望 deny=%v", path, v.Decision, wantDeny)
		}
	}
}

func TestPatchPaths_取出四种头的路径并忽略其余行(t *testing.T) {
	patch := strings.Join([]string{
		"*** Begin Patch",
		"*** Add File: a/new.go",
		"+package a",
		"*** Update File:  b/old.go  ",
		"*** Move to: b/renamed.go",
		"*** Delete File: c/gone.go",
		"*** Add File:x.go",
		"*** End Patch",
	}, "\n")
	got := patchPaths(patch)
	t.Logf("paths=%v", got)
	want := []string{"a/new.go", "b/old.go", "b/renamed.go", "c/gone.go"}
	if !slices.Equal(got, want) {
		t.Fatalf("patchPaths=%v，期望 %v（冒号后缺空格的不算头）", got, want)
	}
	if got := patchPaths(""); got != nil {
		t.Fatalf("空补丁应返回 nil，得到 %v", got)
	}
}

func TestIsPatch_按首个非空白内容判断(t *testing.T) {
	for cmd, want := range map[string]bool{
		"*** Begin Patch\n*** Add File: a": true,
		"  \n*** Begin Patch":              true,
		"echo '*** Begin Patch'":           false,
		"":                                 false,
	} {
		if got := isPatch(cmd); got != want {
			t.Errorf("isPatch(%q)=%v，期望 %v", cmd, got, want)
		}
	}
}
