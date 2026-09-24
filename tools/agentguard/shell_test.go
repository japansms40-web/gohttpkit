package main

import (
	"path/filepath"
	"testing"
)

func TestEvalShell_拦截与放行(t *testing.T) {
	root := newRepo(t, "feat/x")
	cases := []struct {
		name, cmd string
		want      decision
	}{
		{"普通测试命令", "go test -count=1 ./httpx/ -run TestRetry", allow},
		{"普通提交", `git commit -m "feat(httpx): x"`, allow},
		{"推分支", "git push -u origin feat/x", allow},
		{"列出 tag", "git tag -l 'v*'", allow},
		{"只看 tag", "git tag", allow},
		{"打附注 tag", "git tag -a v0.4.0 -m v0.4.0", allow},
		{"打轻量 tag", "git tag v0.4.0", allow},
		{"删临时目录", "rm -rf /tmp/agentguard-xyz", allow},
		{"删仓库内子目录", "rm -rf out/", allow},
		{"非递归删文件", "rm coverage.out", allow},
		{"读示例 env", "cat .env.example", allow},
		{"脚本正文提到 git commit 与治理豁免不算提交", "python3 - <<'EOF'\ns = '| 在 main 上 git commit；提交说明写「治理豁免: x」'\nEOF", allow},

		{"no-verify 提交", `git commit --no-verify -m "x"`, deny},
		{"no-verify 推送", "git push --no-verify", deny},
		{"commit -n", `git commit -nm "x"`, deny},
		{"链式里的 no-verify", `make check && git commit -am x --no-verify`, deny},
		{"bash -c 嵌套", `bash -c "git push --force origin feat/x"`, deny},
		{"env 前缀", "GIT_TRACE=1 git push -f", deny},
		{"强推 lease", "git push --force-with-lease origin feat/x", deny},
		{"+refspec 强推", "git push origin +feat/x", deny},
		{"删远端分支", "git push origin :feat/x", deny},
		{"推 tags", "git push --tags", deny},
		{"推单个 tag", "git push origin v0.4.0", deny},
		{"推 refs/tags", "git push origin HEAD:refs/tags/v1", deny},
		{"删 tag", "git tag -d v0.3.1", deny},
		{"删 tag 长选项", "git tag --delete v0.3.1", deny},
		{"移动 tag", "git tag -f v0.3.1", deny},
		{"移动 tag 组合短选项", "git tag -fa v0.3.1 -m x", deny},
		{"移动 tag 长选项", "git tag --force v0.3.1", deny},
		{"reset hard", "git reset --hard HEAD~1", deny},
		{"clean -fd", "git clean -fd", deny},
		{"卸钩子", "git config --unset core.hooksPath", deny},
		{"读钩子目录 --get", "git config --get core.hooksPath", allow},
		{"读钩子目录 单键无值", "git config core.hooksPath", allow},
		{"读钩子目录 get 子命令", "git config get core.hooksPath", allow},
		{"读钩子目录 带作用域与文件", "git config --local --file .git/config --get core.hooksPath", allow},
		{"读钩子目录 --get 带值正则", "git config --get core.hooksPath githooks", allow},
		{"读钩子目录 大小写", "git config --get CORE.HOOKSPATH", allow},
		{"写钩子目录 两个位置参数", "git config core.hooksPath /dev/null", deny},
		{"写钩子目录 带作用域", "git config --local core.hooksPath /dev/null", deny},
		{"写钩子目录 -f 指定文件", "git config -f .git/config core.hooksPath /dev/null", deny},
		{"写钩子目录 set 子命令", "git config set core.hooksPath /dev/null", deny},
		{"删钩子目录 unset 子命令", "git config unset core.hooksPath", deny},
		{"删钩子目录 --unset-all", "git config --unset-all core.hooksPath", deny},
		{"改钩子目录 --replace-all", "git config --replace-all core.hooksPath /dev/null", deny},
		{"改钩子目录 --add", "git config --add core.hooksPath /dev/null", deny},
		{"--get 与 --unset 混用按写处理", "git config --get --unset core.hooksPath", deny},
		{"删 core 整节", "git config --remove-section core", deny},
		{"删 core 整节 子命令", "git config remove-section core", deny},
		{"读其它键不受影响", "git config --get user.name", allow},
		{"临时换钩子目录", "git -c core.hooksPath=/dev/null commit -m x", deny},
		{"-C 指向其它仓库照样拦", "git -C ../other reset --hard", deny},
		{"-C 的取值不当子命令", "git -C push status", allow},
		{"agent 自写治理豁免", "git commit -F - <<'EOF'\nfix: x\n\n治理豁免: 我觉得可以\nEOF", deny},
		{"读 env", "cat .env", deny},
		{"写 env.local", "echo A=1 >> .env.local", deny},
		{"碰放行标记", `touch "$(git rev-parse --git-path agent-guard-allow)"`, deny},
		{"删放行标记", "rm -f .git/agent-guard-allow", deny},
		{"提交说明正文提到 .env 与 rm -rf 不算执行", "git commit -q -F - <<'EOF'\nfix: x\n\n- 读写 .env 与私钥\nrm -rf /\nEOF\ngit log -1", allow},
		{"heredoc 之后的命令照样检查", "cat <<EOF\nhello\nEOF\ngit push --force", deny},
		{"<<- 缩进定界符", "cat <<-EOF\n\tgit push -f\n\tEOF\nls", allow},
		{"脚本正文提到放行标记不算触碰", "python3 - <<'EOF'\ns = '由人执行 `touch agent-guard-allow`'\nEOF", allow},
		{"删 HOME", "rm -rf ~/", deny},
		{"删仓库外", "rm -rf /etc/foo", deny},
		{"删仓库根", "rm -rf .", deny},
		{"仓库根通配", "rm -rf ./*", deny},
		{"删 .git", "rm -rf .git/hooks", deny},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			v := evalShell(c.cmd, root, root)
			t.Logf("%q → %d %s", c.cmd, v.Decision, v.Reason)
			if v.Decision != c.want {
				t.Fatalf("期望 %d，得到 %d（%s）", c.want, v.Decision, v.Reason)
			}
			if v.Decision != allow && v.Reason == "" {
				t.Fatal("拦截必须给出理由")
			}
		})
	}
}

func TestEvalShell_main分支允许提交与打tag(t *testing.T) {
	root := newRepo(t, "main")
	for _, cmd := range []string{`git commit -m "feat: x"`, "git tag -a v0.4.0 -m v0.4.0"} {
		if v := evalShell(cmd, root, root); v.Decision != allow {
			t.Fatalf("main 上 %q 应放行，得到 %+v", cmd, v)
		}
	}
	// main 上仍不得绕过钩子
	if v := evalShell(`git commit -n -m x`, root, root); v.Decision != deny {
		t.Fatalf("main 上 commit -n 仍应拒绝，得到 %+v", v)
	}
}

func TestSplitSegments(t *testing.T) {
	got := splitSegments(`a "b c" 'd;e' f\ g; h && i | j` + "\nk")
	want := [][]string{{"a", "b c", "d;e", "f g"}, {"h"}, {"i"}, {"j"}, {"k"}}
	if len(got) != len(want) {
		t.Fatalf("段数 %d != %d：%q", len(got), len(want), got)
	}
	for i := range want {
		if len(got[i]) != len(want[i]) {
			t.Fatalf("第 %d 段 %q != %q", i, got[i], want[i])
		}
		for j := range want[i] {
			if got[i][j] != want[i][j] {
				t.Fatalf("第 %d 段 %q != %q", i, got[i], want[i])
			}
		}
	}
	if got := splitSegments(`echo "a \"q\" b"`); len(got) != 1 || got[0][1] != `a "q" b` {
		t.Fatalf("双引号内转义处理错误：%q", got)
	}
	if got := splitSegments(""); len(got) != 0 {
		t.Fatalf("空命令应无段，得到 %q", got)
	}
}

func TestIsSecretPath(t *testing.T) {
	for p, want := range map[string]bool{
		".env": true, "a/.env.local": true, ".envrc": true, "server.pem": true, "id_ed25519": true,
		".env.example": false, "env.go": false, "docs/.environment.md": false, "key.go": false,
	} {
		if got := isSecretPath(p); got != want {
			t.Fatalf("isSecretPath(%q)=%v，期望 %v", p, got, want)
		}
	}
}

// newRepo 建一个在 branch 分支上、带一次提交的临时仓库，返回其真实路径。
func newRepo(t *testing.T, branch string) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) {
		t.Helper()
		if out, err := runCmd(dir, "git", args...); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q", "-b", branch)
	run("-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "--allow-empty", "-m", "init")
	return dir
}
