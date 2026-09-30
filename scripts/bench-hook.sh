#!/usr/bin/env bash
# Times 200 runs of `agentws hook` with the daemon up and down and prints
# p50/p95. Fails if p95 is over budget: 20 ms up, 60 ms down.
# Usage: scripts/bench-hook.sh [path/to/agentws]
set -euo pipefail
bin="${1:-./bin/agentws}"
[ -x "$bin" ] || make build >/dev/null
bin="$(cd "$(dirname "$bin")" && pwd)/$(basename "$bin")"
# why: macOS caps Unix socket paths at 104 bytes, so the home must be short.
home="$(mktemp -d /tmp/aws-bench.XXXXXX)"
export AGENTWS_HOME="$home" TMUX_PANE="%0"
trap '"$bin" daemon stop >/dev/null 2>&1 || true; rm -rf "$home"' EXIT

measure() {
  python3 - "$bin" "$1" "$2" <<'PY'
import subprocess, sys, time
bin, label, budget = sys.argv[1], sys.argv[2], float(sys.argv[3])
payload = b'{"session_id":"bench","tool_name":"Bash","tool_input":{"command":"ls"}}'
runs = []
for _ in range(200):
    t = time.perf_counter()
    subprocess.run([bin, "hook", "--harness", "claude", "--event", "PreToolUse"], input=payload, check=True)
    runs.append((time.perf_counter() - t) * 1000)
runs.sort()
p50, p95 = runs[99], runs[189]
ok = p95 < budget
print(f"daemon {label}: p50 {p50:.1f} ms, p95 {p95:.1f} ms (budget p95 < {budget:.0f} ms) {'ok' if ok else 'OVER BUDGET'}")
sys.exit(0 if ok else 1)
PY
}

"$bin" daemon start >/dev/null
status=0
measure up 20 || status=1
"$bin" daemon stop >/dev/null
measure down 60 || status=1
exit $status
