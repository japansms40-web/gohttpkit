package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// agentKind 是钩子的调用方。三家输入输出格式不同，裁决逻辑相同。
type agentKind string

const (
	agentClaude agentKind = "claude"
	agentCursor agentKind = "cursor"
	agentCodex  agentKind = "codex"
)

// hookEvent 是 agentguard 自己的事件名，各家配置把原生事件映射到这里。
type hookEvent string

const (
	eventPreTool  hookEvent = "pre-tool"  // Claude / Codex PreToolUse，Cursor preToolUse
	eventPreShell hookEvent = "pre-shell" // Cursor beforeShellExecution
	eventPreRead  hookEvent = "pre-read"  // Cursor beforeReadFile
	eventPostTool hookEvent = "post-tool" // Claude / Codex PostToolUse，Cursor afterFileEdit
	eventStop     hookEvent = "stop"      // 三家的 Stop / stop
)

const (
	// maxStopBlocks：收尾检查连续拦截的上限，超过即放行并提示，防止 agent 修不好时无限循环。
	maxStopBlocks  = 3
	stopCounterRel = "agent-guard-stop-blocks"
	checkTimeout   = 5 * time.Minute
	feedbackTail   = 40
	toolsModuleRel = "tools/agentguard"
)

// hookInput 是三家钩子 stdin JSON 的并集，缺的字段为零值。
type hookInput struct {
	ToolName  string         `json:"tool_name"`
	ToolInput map[string]any `json:"tool_input"`
	Command   string         `json:"command"`   // Cursor beforeShellExecution
	FilePath  string         `json:"file_path"` // Cursor afterFileEdit / beforeReadFile
	Cwd       string         `json:"cwd"`
}

// runHook 读 stdin 的钩子 JSON，裁决后按对应 agent 的协议输出。
// 输入 args：--agent claude|cursor|codex、--event pre-tool|pre-shell|pre-read|post-tool|stop。
// 返回：进程退出码。Codex 拦截用 2；其余情况 0（裁决写在 stdout JSON 里）；参数错误 1。
// 不在 git 仓库内时一律放行——本钩子只管本仓。
func runHook(args []string) int {
	fs := flag.NewFlagSet("hook", flag.ContinueOnError)
	agent := fs.String("agent", "", "claude|cursor|codex")
	event := fs.String("event", "", "pre-tool|pre-shell|pre-read|post-tool|stop")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	raw, _ := io.ReadAll(os.Stdin)
	var in hookInput
	_ = json.Unmarshal(raw, &in) // 解析失败按空输入处理：放行
	cwd := in.Cwd
	if cwd == "" {
		cwd, _ = os.Getwd()
	}
	root, err := repoRoot(cwd)
	if err != nil {
		return 0
	}
	h := hookCtx{agent: agentKind(*agent), root: root, cwd: cwd,
		allowOverride: fileExists(gitPath(root, allowMarkerName))}

	switch hookEvent(*event) {
	case eventPreTool:
		return h.emitPre(h.evalTool(in))
	case eventPreShell:
		return h.emitPre(evalShell(in.Command, cwd, root))
	case eventPreRead:
		return h.emitPre(evalRead(in.FilePath))
	case eventPostTool:
		return h.emitPost(h.postEdit(editedPaths(in)))
	case eventStop:
		return h.emitStop(h.stopCheck())
	default:
		fmt.Fprintln(os.Stderr, "agentguard hook: 未知事件 "+*event)
		return 1
	}
}

type hookCtx struct {
	agent         agentKind
	root, cwd     string
	allowOverride bool
}

// evalTool 按工具输入的形状裁决：有 command 走 shell（或 Codex 补丁），有文件路径走读 / 写。
func (h hookCtx) evalTool(in hookInput) verdict {
	worst := allowVerdict
	if cmd := commandOf(in.ToolInput); cmd != "" {
		if isPatch(cmd) {
			for _, p := range patchPaths(cmd) {
				worst = stricter(worst, evalEdit(p, h.cwd, h.root))
			}
			return worst
		}
		return evalShell(cmd, h.cwd, h.root)
	}
	if p := pathOf(in.ToolInput); p != "" {
		if isReadTool(in.ToolName) {
			return evalRead(p)
		}
		return evalEdit(p, h.cwd, h.root)
	}
	return worst
}

func stricter(a, b verdict) verdict {
	if b.Decision > a.Decision {
		return b
	}
	return a
}

func isReadTool(name string) bool {
	switch name {
	case "Read", "Grep", "Glob", "LS", "NotebookRead":
		return true
	}
	return false
}

// commandOf 取工具输入里的命令文本；Codex 可能给 ["bash","-lc","..."] 数组。
func commandOf(ti map[string]any) string {
	switch c := ti["command"].(type) {
	case string:
		return c
	case []any:
		parts := make([]string, 0, len(c))
		for _, x := range c {
			parts = append(parts, fmt.Sprint(x))
		}
		if len(parts) >= 3 && (parts[1] == "-c" || parts[1] == "-lc") {
			return parts[2]
		}
		return strings.Join(parts, " ")
	}
	return ""
}

// pathOf 取工具输入里的目标文件路径（兼容三家不同的字段名）。
func pathOf(ti map[string]any) string {
	for _, k := range []string{"file_path", "notebook_path", "path", "target_file", "filePath"} {
		if s, ok := ti[k].(string); ok && s != "" {
			return s
		}
	}
	return ""
}

// editedPaths 取本次编辑涉及的文件：Cursor afterFileEdit 在顶层，Claude / Codex 在 tool_input 或补丁里。
func editedPaths(in hookInput) []string {
	if in.FilePath != "" {
		return []string{in.FilePath}
	}
	if cmd := commandOf(in.ToolInput); isPatch(cmd) {
		return patchPaths(cmd)
	}
	if p := pathOf(in.ToolInput); p != "" {
		return []string{p}
	}
	return nil
}

// postEdit 对改过的 Go 文件 gofmt -w 并 go vet 所在包；改到治理相关文件时顺带跑治理检查。
// 返回：需要反馈给 agent 的问题文本；没有问题返回空串。
func (h hookCtx) postEdit(paths []string) string {
	var problems []string
	vetDirs := map[string]bool{}
	governanceTouched := false
	for _, p := range paths {
		abs := resolvePath(p, h.cwd)
		if !within(abs, h.root) || !fileExists(abs) {
			continue
		}
		rel, _ := filepath.Rel(h.root, abs)
		rel = filepath.ToSlash(rel)
		if strings.HasSuffix(rel, ".go") {
			if out, err := runCmd(h.root, "gofmt", "-w", abs); err != nil {
				problems = append(problems, "gofmt 失败："+out)
			}
			vetDirs[filepath.Dir(abs)] = true
		}
		if rel == makefilePath || rel == golangciPath || strings.HasSuffix(rel, "_test.go") {
			governanceTouched = true
		}
	}
	for dir := range vetDirs {
		modRoot := h.root
		if within(dir, filepath.Join(h.root, toolsModuleRel)) {
			modRoot = filepath.Join(h.root, toolsModuleRel)
		}
		rel, _ := filepath.Rel(modRoot, dir)
		if out, err := runCmd(modRoot, "go", "vet", "./"+filepath.ToSlash(rel)); err != nil {
			problems = append(problems, "go vet 未通过：\n"+tail(out, feedbackTail))
		}
	}
	if governanceTouched {
		for _, v := range collectViolations(h.root, resolveBase(h.root, ""), true) {
			problems = append(problems, "治理守卫："+v.String())
		}
	}
	return strings.Join(problems, "\n")
}

// stopCheck 是回合收尾检查：治理守卫 + 有 Go 改动时 make check。
// 返回：block 为需要 agent 继续修复的理由；notice 为放弃拦截时给人看的提示。两者至多一个非空。
func (h hookCtx) stopCheck() (block, notice string) {
	counter := gitPath(h.root, stopCounterRel)
	changed := changedPaths(h.root)
	if len(changed) == 0 {
		writeCount(counter, 0)
		return "", ""
	}
	var problems []string
	vs := collectViolations(h.root, resolveBase(h.root, ""), true)
	if len(vs) > 0 && !h.allowOverride {
		for _, v := range vs {
			problems = append(problems, "治理守卫："+v.String())
		}
	}
	if anyGoChange(changed) {
		if out, err := runCmd(h.root, "make", "check"); err != nil {
			problems = append(problems, "make check 未通过：\n"+tail(out, feedbackTail))
		}
	}
	if anyUnder(changed, toolsModuleRel+"/") {
		if out, err := runCmd(h.root, "make", "tools-check"); err != nil {
			problems = append(problems, "make tools-check 未通过：\n"+tail(out, feedbackTail))
		}
	}
	if len(problems) == 0 {
		writeCount(counter, 0)
		return "", ""
	}
	n := readCount(counter) + 1
	if n > maxStopBlocks {
		writeCount(counter, 0)
		return "", fmt.Sprintf("agentguard：收尾检查连续 %d 次未通过，已停止拦截。请人工核对：\n%s", maxStopBlocks, strings.Join(problems, "\n"))
	}
	writeCount(counter, n)
	return fmt.Sprintf("agentguard 收尾检查未通过（第 %d/%d 次）。请修复后再结束；确实无法修复时，停止尝试并如实告诉用户哪些检查失败、原因是什么。\n%s",
		n, maxStopBlocks, strings.Join(problems, "\n")), ""
}

// emitPre 按 agent 协议输出前置裁决。只有放行 / 拒绝两档，不弹人工确认框。
func (h hookCtx) emitPre(v verdict) int {
	switch h.agent {
	case agentClaude:
		if v.Decision == allow {
			return 0
		}
		printJSON(map[string]any{"hookSpecificOutput": map[string]any{
			"hookEventName": "PreToolUse", "permissionDecision": "deny", "permissionDecisionReason": "agentguard：" + v.Reason}})
		return 0
	case agentCursor:
		perm := "allow"
		if v.Decision != allow {
			perm = "deny"
		}
		out := map[string]any{"permission": perm}
		if v.Reason != "" {
			out["user_message"] = "agentguard：" + v.Reason
			out["agent_message"] = "agentguard：" + v.Reason
		}
		printJSON(out)
		return 0
	default: // codex 及未知 agent：退出码 2 + stderr 即拦截
		if v.Decision == allow {
			return 0
		}
		fmt.Fprintln(os.Stderr, "agentguard："+v.Reason)
		return 2
	}
}

// emitPost 把编辑后发现的问题反馈给 agent（不撤销编辑）。
func (h hookCtx) emitPost(feedback string) int {
	if feedback == "" {
		return 0
	}
	switch h.agent {
	case agentCursor:
		printJSON(map[string]any{"additional_context": "agentguard：" + feedback})
	default:
		printJSON(map[string]any{"decision": "block", "reason": "agentguard：" + feedback})
	}
	return 0
}

// emitStop 输出收尾裁决：block 非空则要求 agent 继续；notice 非空则提示人。
func (h hookCtx) emitStop(block, notice string) int {
	switch {
	case block != "" && h.agent == agentCursor:
		printJSON(map[string]any{"followup_message": block})
	case block != "":
		printJSON(map[string]any{"decision": "block", "reason": block})
	case notice != "" && h.agent == agentClaude:
		printJSON(map[string]any{"systemMessage": notice})
	case notice != "":
		fmt.Fprintln(os.Stderr, notice)
	}
	return 0
}

// changedPaths 列出相对 HEAD 的未提交改动（含未跟踪文件），仓库相对路径。
func changedPaths(root string) []string {
	out, err := gitRaw(root, "status", "--porcelain", "--untracked-files=all")
	if err != nil {
		return nil
	}
	var paths []string
	for _, l := range strings.Split(out, "\n") {
		if len(l) < 4 {
			continue
		}
		p := l[3:]
		if i := strings.LastIndex(p, " -> "); i >= 0 {
			p = p[i+4:]
		}
		paths = append(paths, strings.Trim(p, `"`))
	}
	return paths
}

func anyGoChange(paths []string) bool {
	for _, p := range paths {
		if strings.HasSuffix(p, ".go") || filepath.Base(p) == "go.mod" || filepath.Base(p) == "go.sum" {
			return true
		}
	}
	return false
}

func anyUnder(paths []string, prefix string) bool {
	for _, p := range paths {
		if strings.HasPrefix(p, prefix) {
			return true
		}
	}
	return false
}

// runCmd 在 dir 下执行命令，合并 stdout / stderr 返回；超过 checkTimeout 视为失败。
func runCmd(dir, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), checkTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	return out.String(), err
}

// tail 取 s 的最后 n 行。
func tail(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}

func fileExists(p string) bool {
	if p == "" {
		return false
	}
	_, err := os.Stat(p)
	return err == nil
}

func readCount(p string) int {
	b, err := os.ReadFile(p)
	if err != nil {
		return 0
	}
	n, _ := strconv.Atoi(strings.TrimSpace(string(b)))
	return n
}

func writeCount(p string, n int) {
	if p == "" {
		return
	}
	if n == 0 {
		_ = os.Remove(p)
		return
	}
	_ = os.WriteFile(p, []byte(strconv.Itoa(n)), 0o600)
}

func printJSON(v any) {
	b, _ := json.Marshal(v)
	fmt.Println(string(b))
}
