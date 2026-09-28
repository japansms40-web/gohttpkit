package main

import (
	"errors"
	"strings"
	"testing"
)

func TestParseCharRule(t *testing.T) {
	cases := []struct {
		name, src   string
		wantPath    string
		wantFuncRe  string // 空表示 funcRe 为 nil
		wantErrText string // 非空表示期望报错且文案含此片段
	}{
		{name: "空配置不检查", src: ""},
		{name: "只配文件按整文件", src: "characterization:\n  file: a_test.go\n", wantPath: "a_test.go"},
		{name: "文件加函数正则", src: "characterization:\n  file: a_test.go\n  func_pattern: ^TestA_\n", wantPath: "a_test.go", wantFuncRe: "^TestA_"},
		{name: "YAML 非法", src: "characterization: [", wantErrText: "无法解析"},
		{name: "正则非法", src: "characterization:\n  file: a_test.go\n  func_pattern: \"(\"\n", wantErrText: "func_pattern 非法"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rule, err := parseCharRule(c.src)
			if c.wantErrText != "" {
				if err == nil || !strings.Contains(err.Error(), c.wantErrText) {
					t.Fatalf("期望含 %q 的错误，得到 %v", c.wantErrText, err)
				}
				var typed *configParseError
				if !errors.As(err, &typed) || typed.Unwrap() == nil {
					t.Fatalf("配置解析错误应保留类型和底层原因，得到 %T %v", err, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("不应报错：%v", err)
			}
			if rule.path != c.wantPath {
				t.Fatalf("path 期望 %q，得到 %q", c.wantPath, rule.path)
			}
			gotRe := ""
			if rule.funcRe != nil {
				gotRe = rule.funcRe.String()
			}
			if gotRe != c.wantFuncRe {
				t.Fatalf("funcRe 期望 %q，得到 %q", c.wantFuncRe, gotRe)
			}
		})
	}
}

func TestParseMainBranch(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"未配置默认 main", "", "main"},
		{"只配 char 默认 main", "characterization:\n  file: a_test.go\n", "main"},
		{"显式主干", "main_branch: insgo190\n", "insgo190"},
		{"首尾空白裁剪", "main_branch: \"  release \"\n", "release"},
		{"YAML 非法回落 main", "main_branch: [", "main"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := parseMainBranch(c.src); got != c.want {
				t.Fatalf("parseMainBranch(%q)=%q，期望 %q", c.src, got, c.want)
			}
		})
	}
}
