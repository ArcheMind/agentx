#!/bin/sh
set -eu

ax=${1:-./bin/ax}
root=$(CDPATH= cd -- "$(dirname "$0")/../.." && pwd)
recorder=$root/test/smoke/record-hook.sh
workspace=$(mktemp -d "${TMPDIR:-/tmp}/agentx-hooks-smoke.XXXXXX")
workspace=$(CDPATH= cd -- "$workspace" && pwd -P)
backup=$workspace/backup
project=$workspace/project
results=$workspace/results
mkdir -p "$backup" "$project/.agents" "$results"

owned_paths="$HOME/.agents/.agentx/hooks/claude
$HOME/.agents/.agentx/hooks/pi
$HOME/.gemini/extensions/agentx-hooks"

cleanup() {
	printf '%s\n' "$owned_paths" | while IFS= read -r path; do
		[ -n "$path" ] || continue
		key=$(printf '%s' "$path" | tr '/ ' '__')
		if [ -d "$backup/$key" ]; then
			rm -rf "$path"
			mkdir -p "$(dirname "$path")"
			cp -R "$backup/$key" "$path"
		elif [ -f "$path/.agentx-owned" ]; then
			rm -rf "$path"
		fi
	done
	printf 'Hook smoke evidence: %s\n' "$results"
}
trap cleanup EXIT HUP INT TERM

printf '%s\n' "$owned_paths" | while IFS= read -r path; do
	[ -n "$path" ] || continue
	if [ -e "$path" ] && [ ! -f "$path/.agentx-owned" ]; then
		printf 'Refusing to touch non-AgentX artifact: %s\n' "$path" >&2
		exit 1
	fi
	if [ -d "$path" ]; then
		key=$(printf '%s' "$path" | tr '/ ' '__')
		cp -R "$path" "$backup/$key"
	fi
done

for agent in claude codex gemini pi; do
	if ! command -v "$agent" >/dev/null 2>&1; then
		"$ax" agent install "$agent"
	fi
done

require_login() {
	agent=$1
	if ! "$ax" --json auth status "$agent" | node -e '
let data = "";
process.stdin.on("data", chunk => data += chunk);
process.stdin.on("end", () => process.exit(JSON.parse(data).providers.length > 0 ? 0 : 1));
'; then
		printf '%s is not logged in. Run: %s auth login %s\n' "$agent" "$ax" "$agent" >&2
		exit 1
	fi
}

require_login claude
require_login codex
require_login pi

pi_model=deepseek/deepseek-v4-flash

run_agent() {
	agent=$1
	model=$2
	shift 2
	agent_results=$results/$agent
	mkdir -p "$agent_results"
	node "$root/test/smoke/make-hooks.mjs" "$project/.agents/hooks.json" "$recorder" "$agent_results"
	"$ax" "$agent" --model "$model" --cwd "$project" --dry-run -- "$@" >"$agent_results/plan.json"
	if [ "$agent" = claude ]; then
		set -- --debug-file "$agent_results/provider-debug.log" "$@"
	fi
	if ! "$ax" "$agent" --model "$model" --cwd "$project" -- "$@" >"$agent_results/stdout.log" 2>"$agent_results/stderr.log"; then
		if [ "$agent" = gemini ]; then
			printf 'Gemini smoke failed. If authentication is missing, run: %s auth login gemini\n' "$ax" >&2
		fi
		return 1
	fi
	case "$agent" in
		claude) artifact=$HOME/.agents/.agentx/hooks/claude ;;
		gemini) artifact=$HOME/.gemini/extensions/agentx-hooks ;;
		pi) artifact=$HOME/.agents/.agentx/hooks/pi ;;
		*) artifact= ;;
	esac
	if [ -n "$artifact" ] && [ -d "$artifact" ]; then
		cp -R "$artifact" "$agent_results/projected-artifact"
	fi
	node "$root/test/smoke/assert-events.mjs" "$agent_results/events.jsonl" "$agent"
}

prompt='Reply with exactly OK.'
failures=

run_case() {
	agent=$1
	shift
	if run_agent "$agent" "$@"; then
		return
	fi
	printf '%s smoke failed; continuing with the remaining providers.\n' "$agent" >&2
	failures="${failures}${failures:+, }${agent}"
}

run_case claude haiku --print "$prompt"
run_case codex gpt-6-luna --dangerously-bypass-hook-trust exec --skip-git-repo-check "$prompt"
run_case gemini gemini-2.5-flash-lite --skip-trust --prompt "$prompt"
run_case pi "$pi_model" --print "$prompt"

if [ -n "$failures" ]; then
	printf 'Hook smoke failures: %s\n' "$failures" >&2
	exit 1
fi
