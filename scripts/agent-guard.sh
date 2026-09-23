#!/usr/bin/env bash
# agentguard 启动器：三家 agent 钩子（Claude / Cursor / Codex）的统一入口。
# 首次或 tools/agentguard 源码变更后自动编译到 git 目录（不入库），之后直接执行，单次 <50ms。
# 用法：scripts/agent-guard.sh hook --agent claude|cursor|codex --event <事件>  （stdin 为钩子 JSON）
# 编译失败时退出 1：三家都按「钩子出错、放行」处理，git 钩子与 CI 仍兜底，不会把 agent 卡死。
set -uo pipefail

root=$(git -C "${CLAUDE_PROJECT_DIR:-$PWD}" rev-parse --show-toplevel 2>/dev/null) || exit 0
src="$root/tools/agentguard"
bin="$(git -C "$root" rev-parse --absolute-git-dir)/agent-guard/agentguard"

if [ ! -x "$bin" ] || [ -n "$(find "$src" \( -name '*.go' -o -name go.mod -o -name go.sum \) -newer "$bin" 2>/dev/null | head -1)" ]; then
  mkdir -p "$(dirname "$bin")"
  if ! (cd "$src" && go build -o "$bin" . ) >&2; then
    echo "agentguard：编译失败，本次钩子放行" >&2
    exit 1
  fi
fi
exec "$bin" "$@"
