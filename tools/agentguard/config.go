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

// configParseError 保留配置字段与原始解析错误，供治理守卫向用户解释原因。
type configParseError struct {
	Field string
	Err   error
}

// Error 返回与旧配置检查一致的可读文案。
// 输入 e 的 Field 为空表示 YAML 解析失败，否则表示具体字段非法。
// 返回包含配置路径、字段和原始原因的字符串。
func (e *configParseError) Error() string {
	if e.Field == "" {
		return fmt.Sprintf("%s 无法解析：%v", configPath, e.Err)
	}
	return fmt.Sprintf("%s 的 %s 非法：%v", configPath, e.Field, e.Err)
}

// Unwrap 暴露原始 YAML 或正则解析错误。
// 输入 e 为当前配置错误。返回 Err。
func (e *configParseError) Unwrap() error { return e.Err }

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
	Commit struct {
		// Scopes 是仓库级允许的提交 scope；含 Go 文件的目录（相对路径与目录名）自动允许，不用列。空表示不校验 scope。
		Scopes []string `yaml:"scopes"`
	} `yaml:"commit"`
	Style struct {
		// HelperPlacement 为 true 时 check-style 检查「声明了方法的文件里不放包级纯辅助函数」（CODE_STANDARDS §2）。
		HelperPlacement bool `yaml:"helper_placement"`
		// HelperPlacementExempt 是 helper-placement 放行的文件（相对仓库根的完整路径），每项须在配置注释里写理由。
		HelperPlacementExempt []string `yaml:"helper_placement_exempt"`
		// RootCtxAllow 是 root-ctx 规则放行的路径前缀（相对仓库根，如 gohttpkit 的 "logger/" 门面自身）。
		RootCtxAllow []string `yaml:"root_ctx_allow"`
	} `yaml:"style"`
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
		return charRule{}, &configParseError{Err: err}
	}
	rule := charRule{path: cfg.Characterization.File}
	if p := cfg.Characterization.FuncPattern; p != "" {
		re, err := regexp.Compile(p)
		if err != nil {
			return charRule{}, &configParseError{Field: "characterization.func_pattern", Err: err}
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

// loadStaticConfig 读 HEAD 提交里的 .agentguard.yml，供 check-style / check-commit-msg 使用。
// 输入 root：仓库根。
// 返回：解析后的配置；HEAD 不存在或其中没有该文件时为零值；YAML 非法时返回 *configParseError。
// 读 HEAD 而不是工作区：同一次改动里关掉开关或删 scope 白名单，不能让本次检查放行。
// 例：HEAD 里是 `style: {helper_placement: true}` → cfg.Style.HelperPlacement == true。
func loadStaticConfig(root string) (repoConfig, error) {
	src, ok := showAt(root, "HEAD", configPath)
	if !ok {
		return repoConfig{}, nil
	}
	var cfg repoConfig
	if err := yaml.Unmarshal([]byte(src), &cfg); err != nil {
		return repoConfig{}, &configParseError{Err: err}
	}
	return cfg, nil
}
