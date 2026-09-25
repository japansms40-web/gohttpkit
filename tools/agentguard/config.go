package main

import (
	"fmt"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// config.go —— 仓库级差异配置。agentguard 由多个仓库共用（gohttpkit 自身、insgo 等），
// 各仓库只有 characterization 用例的定位与主干分支名不同，放在仓库根的 .agentguard.yml 里声明，不再复制源码改常量。

// configPath 是仓库根下的 agentguard 配置文件。
const configPath = ".agentguard.yml"

// defaultMainBranch 是未配置 main_branch 时的主干分支名。
const defaultMainBranch = "main"

// repoConfig 对应 .agentguard.yml 的结构。
type repoConfig struct {
	// MainBranch 是主干分支名（如 insgo 的 insgo190），用于取治理基线 merge-base(HEAD, origin/<MainBranch>)；空表示 main。
	MainBranch       string `yaml:"main_branch"`
	Characterization struct {
		// File 是 characterization 用例所在的测试文件（相对仓库根）；空表示不做 char 检查。
		File string `yaml:"file"`
		// FuncPattern 是 char 用例函数名正则；空表示整个文件都算 char 用例。
		// 应与该仓库 Makefile char 目标的 -run 正则同源。
		FuncPattern string `yaml:"func_pattern"`
	} `yaml:"characterization"`
}

// charRule 是解析后的 characterization 规则。
type charRule struct {
	// path 为空表示不检查。
	path string
	// funcRe 为 nil 表示整文件都算 char 用例。
	funcRe *regexp.Regexp
}

// parseCharRule 解析 .agentguard.yml 内容。
// 输入 src：配置全文，可为空串（等价于未配置）。
// 返回：解析出的规则；YAML 或正则非法时返回 error，调用方应按违规处理（宁可多拦）。
// 例：`characterization: {file: a_test.go, func_pattern: "^TestA_"}` → {path:"a_test.go", funcRe:^TestA_}。
func parseCharRule(src string) (charRule, error) {
	var cfg repoConfig
	if err := yaml.Unmarshal([]byte(src), &cfg); err != nil {
		return charRule{}, fmt.Errorf("%s 无法解析：%w", configPath, err)
	}
	rule := charRule{path: cfg.Characterization.File}
	if p := cfg.Characterization.FuncPattern; p != "" {
		re, err := regexp.Compile(p)
		if err != nil {
			return charRule{}, fmt.Errorf("%s 的 characterization.func_pattern 非法：%w", configPath, err)
		}
		rule.funcRe = re
	}
	return rule, nil
}

// loadCharRule 从【基线】读取 char 规则。
// 输入 root / base：仓库根与对比基线。
// 返回：基线没有配置文件时为零值规则（不检查）；解析失败返回 error。
// 读基线而不是工作区：同一次改动里改配置（换文件、放宽正则）不能让自己绕过检查。
func loadCharRule(root, base string) (charRule, error) {
	src, ok := showAt(root, base, configPath)
	if !ok {
		return charRule{}, nil
	}
	return parseCharRule(src)
}

// parseMainBranch 从 .agentguard.yml 内容取主干分支名。
// 输入 src：配置全文，可为空串。
// 返回：main_branch 去空白后的值；未配置、为空或 YAML 非法时回落 "main"（非法配置由 parseCharRule 报违规，这里不重复）。
// 例：`main_branch: insgo190` → "insgo190"；"" → "main"。
func parseMainBranch(src string) string {
	var cfg repoConfig
	if err := yaml.Unmarshal([]byte(src), &cfg); err != nil {
		return defaultMainBranch
	}
	if b := strings.TrimSpace(cfg.MainBranch); b != "" {
		return b
	}
	return defaultMainBranch
}
