package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// boolTightness 描述一个开关：从 strict 值改成相反值即放宽。
type boolTightness struct {
	path   []string
	strict bool
}

// numericRules 是数值调高即放宽的阈值路径。
var numericRules = [][]string{
	{"linters", "settings", "gocyclo", "min-complexity"},
	{"linters", "settings", "goconst", "min-occurrences"},
	{"linters", "settings", "goconst", "min-len"},
}

var boolRules = []boolTightness{
	{[]string{"linters", "settings", "goconst", "ignore-calls"}, false},
	{[]string{"linters", "settings", "nolintlint", "require-specific"}, true},
	{[]string{"linters", "settings", "nolintlint", "require-explanation"}, true},
	{[]string{"linters", "settings", "nolintlint", "allow-unused"}, false},
	{[]string{"run", "tests"}, true},
	{[]string{"issues", "new"}, false},
}

// compareGolangci 找出 .golangci.yml 相对基线的放宽：关 linter、加豁免、调松阈值。
// 输入 old / cur：基线与当前的 YAML 全文。
// 返回：违规列表；基线无法解析或不是 map 时不检查（无从对比），当前无法解析报一条违规。
func compareGolangci(old, cur string) []violation {
	var o, c map[string]any
	if yaml.Unmarshal([]byte(old), &o) != nil || o == nil {
		return nil
	}
	if err := yaml.Unmarshal([]byte(cur), &c); err != nil {
		return []violation{{ruleGolangci, ".golangci.yml 无法解析：" + err.Error(), overrideGovernance}}
	}
	return append(compareLinterSets(o, c), compareThresholds(o, c)...)
}

// compareLinterSets 比较 linter 开关、豁免与禁用 / 排除列表：基线有而当前没有（或反之）即放宽。
func compareLinterSets(o, c map[string]any) []violation {
	var vs []violation
	add := func(format string, a ...any) {
		vs = append(vs, violation{ruleGolangci, fmt.Sprintf(format, a...), overrideGovernance})
	}
	if od, cd := scalarAt(o, "linters", "default"), scalarAt(c, "linters", "default"); od != cd {
		add("linters.default 从 %q 改为 %q", od, cd)
	}
	for _, s := range minus(listAt(o, "linters", "enable"), listAt(c, "linters", "enable")) {
		add("关闭了 linter：%s", s)
	}
	for _, s := range minus(listAt(c, "linters", "disable"), listAt(o, "linters", "disable")) {
		add("新增 disable：%s", s)
	}
	for _, s := range minus(flatten(at(c, "linters", "exclusions")), flatten(at(o, "linters", "exclusions"))) {
		add("新增豁免：linters.exclusions.%s", s)
	}
	for _, s := range minus(flatten(at(c, "issues")), flatten(at(o, "issues"))) {
		// issues 下的 new / new-from-rev / max-* 都能整片隐藏告警
		add("issues 配置新增 / 变更：issues.%s", s)
	}
	for _, s := range minus(listAt(o, "linters", "settings", "forbidigo", "forbid"), listAt(c, "linters", "settings", "forbidigo", "forbid")) {
		add("删除了 forbidigo 禁用规则：%s", s)
	}
	for _, s := range minus(listAt(c, "linters", "settings", "gosec", "excludes"), listAt(o, "linters", "settings", "gosec", "excludes")) {
		add("gosec 新增排除：%s", s)
	}
	return vs
}

// compareThresholds 比较数值阈值与严格开关（numericRules / boolRules）。
func compareThresholds(o, c map[string]any) []violation {
	var vs []violation
	add := func(format string, a ...any) {
		vs = append(vs, violation{ruleGolangci, fmt.Sprintf(format, a...), overrideGovernance})
	}
	for _, p := range numericRules {
		ov, ok1 := numberAt(o, p...)
		cv, ok2 := numberAt(c, p...)
		if ok1 && (!ok2 || cv > ov) {
			add("%s 从 %g 放宽为 %v", strings.Join(p, "."), ov, at(c, p...))
		}
	}
	for _, r := range boolRules {
		ov, ok1 := at(o, r.path...).(bool)
		cv, ok2 := at(c, r.path...).(bool)
		// 基线是严格值、当前不再是严格值（改反或删掉）即放宽
		if ok1 && ov == r.strict && (!ok2 || cv != r.strict) {
			add("%s 从 %v 放宽", strings.Join(r.path, "."), ov)
		}
		// 基线没写（取默认）而当前显式写成宽松值，同样算放宽
		if !ok1 && ok2 && cv != r.strict {
			add("%s 新增为宽松值 %v", strings.Join(r.path, "."), cv)
		}
	}
	return vs
}

func at(m any, path ...string) any {
	cur := m
	for _, p := range path {
		mm, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur = mm[p]
	}
	return cur
}

func scalarAt(m any, path ...string) string {
	v := at(m, path...)
	if v == nil {
		return ""
	}
	return fmt.Sprint(v)
}

func numberAt(m any, path ...string) (float64, bool) {
	switch v := at(m, path...).(type) {
	case int:
		return float64(v), true
	case float64:
		return v, true
	default:
		return 0, false
	}
}

// listAt 把 path 处的列表转成规范化字符串切片（元素为 map 时用 JSON 序列化，键已排序）。
func listAt(m any, path ...string) []string {
	l, ok := at(m, path...).([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(l))
	for _, e := range l {
		out = append(out, canon(e))
	}
	return out
}

// flatten 把一个配置段压平成「key=值」集合：列表逐元素展开，标量直接取值，嵌套 map 递归。
func flatten(v any) []string {
	var out []string
	var walk func(prefix string, v any)
	walk = func(prefix string, v any) {
		switch x := v.(type) {
		case map[string]any:
			keys := make([]string, 0, len(x))
			for k := range x {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				p := k
				if prefix != "" {
					p = prefix + "." + k
				}
				walk(p, x[k])
			}
		case []any:
			for _, e := range x {
				out = append(out, prefix+"="+canon(e))
			}
		case nil:
		default:
			out = append(out, prefix+"="+fmt.Sprint(x))
		}
	}
	walk("", v)
	return out
}

func canon(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprint(v)
	}
	return string(b)
}

// minus 返回在 a 中而不在 b 中的元素，保持 a 的顺序。
func minus(a, b []string) []string {
	set := make(map[string]bool, len(b))
	for _, s := range b {
		set[s] = true
	}
	var out []string
	for _, s := range a {
		if !set[s] {
			out = append(out, s)
		}
	}
	return out
}
