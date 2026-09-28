package main

import (
	"path"
	"regexp"
	"sort"
	"strings"
)

// pkgDocFile 是每个 Go 包唯一承载包注释的文件（CODE_STANDARDS §13）。
const pkgDocFile = "doc.go"

// pkgDocTreeHeading 是包注释里文件结构树的标题行；树条目只认它之后的行。
const pkgDocTreeHeading = "文件结构："

// pkgDocTreeEntry 匹配一层树条目：「├── 名称  用途」或「└── 名称  用途」。
// 以 │ 缩进的下层行不匹配——下层由子包自己的 doc.go 登记。
var pkgDocTreeEntry = regexp.MustCompile(`^[├└]── (\S+)[ \t]*(.*)$`)

// scanPkgDoc 检查每个 Go 包都有 doc.go 与一层文件结构树（CODE_STANDARDS §13）。
// 输入 root 是 Git 仓库根目录；扫描已跟踪与未跟踪（不含忽略）的 Go 文件，含独立子模块，跳过 testdata/。
// Go 包目录指直接含非测试 .go 文件的目录。规则：
//   - 必须有 doc.go，且带包注释；包注释只能写在 doc.go；
//   - 包注释里「文件结构：」之后的一层树条目，必须与本目录全部非测试 .go 文件 ∪ 子树含 Go 包的直接子目录（带 /）一一对应，
//     不得重复，每条带用途说明。
//
// 返回排序后的全部违规；列文件、读文件或解析失败时返回错误，不能静默放行。
func scanPkgDoc(root string) ([]string, error) {
	present, err := presentGoFiles(root)
	if err != nil {
		return nil, err
	}
	files := make(map[string][]string) // 包目录 → 非测试 .go 文件名
	for rel := range present {
		if strings.HasSuffix(rel, "_test.go") {
			continue
		}
		dir, base := path.Split(rel)
		dir = path.Clean(dir)
		files[dir] = append(files[dir], base)
	}
	children := make(map[string]map[string]bool) // 目录 → 子树含 Go 包的直接子目录名（带 /）
	for dir := range files {
		for child := dir; child != "."; child = path.Dir(child) {
			parent := path.Dir(child)
			if children[parent] == nil {
				children[parent] = make(map[string]bool)
			}
			children[parent][path.Base(child)+"/"] = true
		}
	}
	var violations []string
	for dir, names := range files {
		found, checkErr := checkPkgDoc(root, dir, names, children[dir])
		if checkErr != nil {
			return nil, checkErr
		}
		violations = append(violations, found...)
	}
	sort.Strings(violations)
	return violations, nil
}

// checkPkgDoc 检查一个包目录：包注释位置、doc.go 是否存在、文件结构树是否与 names ∪ subdirs 一致。
func checkPkgDoc(root, dir string, names []string, subdirs map[string]bool) ([]string, error) {
	docRel := path.Join(dir, pkgDocFile)
	want := make(map[string]bool, len(names)+len(subdirs))
	for sub := range subdirs {
		want[sub] = true
	}
	var out []string
	hasDoc := false
	for _, name := range names {
		want[name] = true
		if name == pkgDocFile {
			hasDoc = true
			continue
		}
		rel := path.Join(dir, name)
		f, err := parseGoFile(root, rel)
		if err != nil {
			return nil, err
		}
		// 只含指令（如文件级 //nolint:xxx，必须紧贴 package 才对整个文件生效）的注释组 Text() 为空，
		// go doc 也不当包文档，不算违规。
		if f.Doc != nil && strings.TrimSpace(f.Doc.Text()) != "" {
			out = append(out, rel+": 包注释只能写在 doc.go，请移入 "+docRel)
		}
	}
	if !hasDoc {
		return append(out, docRel+": 缺少 doc.go（包注释 + 文件结构树），见 CODE_STANDARDS §13"), nil
	}
	f, err := parseGoFile(root, docRel)
	if err != nil {
		return nil, err
	}
	if f.Doc == nil || strings.TrimSpace(f.Doc.Text()) == "" {
		return append(out, docRel+": 缺少包注释（须紧贴 package 子句）"), nil
	}
	entries, hasTree := parsePkgDocTree(f.Doc.Text())
	if !hasTree {
		return append(out, docRel+": 包注释缺少「"+pkgDocTreeHeading+"」文件树，逐项列出本目录非测试 .go 文件与子目录"), nil
	}
	return append(out, diffPkgDocTree(docRel, entries, want)...), nil
}

// pkgDocTreeItem 是文件结构树的一行：条目名与用途说明。
type pkgDocTreeItem struct {
	name string
	desc string
}

// parsePkgDocTree 取包注释中「文件结构：」之后的一层树条目；没有标题或标题后没有条目时 ok 为 false。
func parsePkgDocTree(doc string) (items []pkgDocTreeItem, ok bool) {
	inTree := false
	for line := range strings.SplitSeq(doc, "\n") {
		line = strings.TrimSpace(line)
		if line == pkgDocTreeHeading {
			inTree = true
			continue
		}
		if !inTree {
			continue
		}
		if m := pkgDocTreeEntry.FindStringSubmatch(line); m != nil {
			items = append(items, pkgDocTreeItem{name: m[1], desc: strings.TrimSpace(m[2])})
		}
	}
	return items, len(items) > 0
}

// diffPkgDocTree 对比树条目与实际应登记的集合，报重复、缺说明、多登与漏登。
func diffPkgDocTree(docRel string, items []pkgDocTreeItem, want map[string]bool) []string {
	var out []string
	listed := make(map[string]bool, len(items))
	for _, it := range items {
		if listed[it.name] {
			out = append(out, docRel+": 文件结构树重复登记 "+it.name)
			continue
		}
		listed[it.name] = true
		if it.desc == "" {
			out = append(out, docRel+": 文件结构树条目 "+it.name+" 缺少用途说明")
		}
		if !want[it.name] {
			out = append(out, docRel+": 文件结构树登记了不存在的 "+it.name+"（只列本目录非测试 .go 文件与含 Go 包的子目录）")
		}
	}
	for name := range want {
		if !listed[name] {
			out = append(out, docRel+": 文件结构树未登记 "+name)
		}
	}
	return out
}
