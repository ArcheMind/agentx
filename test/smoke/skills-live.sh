#!/bin/sh
set -eu

ax=${1:-./bin/ax}
root=$(CDPATH= cd -- "$(dirname "$0")/../.." && pwd)
fixture=$root/test/smoke/fixtures/agentx-unified-smoke
workspace=$(mktemp -d "${TMPDIR:-/tmp}/agentx-skills-smoke.XXXXXX")
workspace=$(CDPATH= cd -- "$workspace" && pwd -P)
backup=$workspace/backup
project=$workspace/project
results=$workspace/results
mkdir -p "$backup" "$project" "$results"

owned_paths="$HOME/.agents/.agentx/hooks/claude
$HOME/.agents/.agentx/hooks/pi
$HOME/.gemini/extensions/agentx-hooks"

cleanup() {
	printf '%s\n' "$owned_paths" | while IFS= read -r owned; do
		[ -n "$owned" ] || continue
		key=$(printf '%s' "$owned" | tr '/ ' '__')
		if [ -d "$backup/$key" ]; then
			rm -rf "$owned"
			mkdir -p "$(dirname "$owned")"
			cp -R "$backup/$key" "$owned"
		elif [ -f "$owned/.agentx-owned" ]; then
			rm -rf "$owned"
		fi
	done
	printf 'Skill smoke evidence: %s\n' "$results"
}
trap cleanup EXIT
trap 'exit 130' HUP INT TERM

printf '%s\n' "$owned_paths" | while IFS= read -r owned; do
	[ -n "$owned" ] || continue
	if [ -e "$owned" ] && [ ! -f "$owned/.agentx-owned" ]; then
		printf 'Refusing to touch non-AgentX artifact: %s\n' "$owned" >&2
		exit 1
	fi
	if [ -d "$owned" ]; then
		key=$(printf '%s' "$owned" | tr '/ ' '__')
		cp -R "$owned" "$backup/$key"
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

(
	cd "$project"
	"$ax" skill install "$fixture" --scope project >"$results/install.log"
	"$ax" --json skill show agentx-unified-smoke >"$results/skill.json"
)

prompt='Use the agentx-unified-smoke skill. Return only its verification token.'
token=AGENTX_UNIFIED_SKILL_7C91F4E2
failures=

run_agent() {
	agent=$1
	model=$2
	shift 2
	agent_results=$results/$agent
	mkdir -p "$agent_results"
	"$ax" "$agent" --model "$model" --cwd "$project" --dry-run -- "$@" >"$agent_results/plan.json"
	if ! node "$root/test/smoke/run-with-timeout.mjs" 120 "$ax" "$agent" --model "$model" --cwd "$project" -- "$@" >"$agent_results/stdout.log" 2>"$agent_results/stderr.log"; then
		if [ "$agent" = gemini ]; then
			printf 'Gemini smoke failed. If authentication is missing, run: %s auth login gemini\n' "$ax" >&2
		fi
		return 1
	fi
	if ! grep -F "$token" "$agent_results/stdout.log" >/dev/null; then
		printf '%s did not return the Skill verification token.\n' "$agent" >&2
		return 1
	fi
	printf '%s: %s\n' "$agent" "$token"
}

run_case() {
	agent=$1
	shift
	if run_agent "$agent" "$@"; then
		return
	fi
	printf '%s Skill smoke failed; continuing with the remaining providers.\n' "$agent" >&2
	failures="${failures}${failures:+, }${agent}"
}

run_case claude haiku --print "$prompt"
run_case codex gpt-6-luna exec --skip-git-repo-check "$prompt"
run_case gemini gemini-2.5-flash-lite --skip-trust --policy "$root/test/smoke/gemini-skill-policy.toml" --prompt 'Use the activate_skill tool to activate agentx-unified-smoke, then return only its verification token.'
run_case pi deepseek/deepseek-v4-flash --approve --no-session --print '/skill:agentx-unified-smoke Return only its verification token.'

if [ -n "$failures" ]; then
	printf 'Skill smoke failures: %s\n' "$failures" >&2
	exit 1
fi
