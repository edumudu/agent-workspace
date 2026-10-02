#!/usr/bin/env bash
# why: usage: [BUDGET_SCALE=2] scripts/bench-hook.sh [path/to/agentws]
# Times 200 runs of `agentws hook` with the daemon up and down and prints
# p50/p95. Fails if p95 is over budget: 20 ms up, 60 ms down.
# BUDGET_SCALE multiplies both budgets, for slow shared CI runners.
# Each side gets 20 discarded warmup runs, then three rounds of 200; the
# lowest round p95 is judged, since a noisy neighbour only ever adds time
# and one bad burst should not fail a build whose hook is still fast.
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
import os
bin, label = sys.argv[1], sys.argv[2]
budget = float(sys.argv[3]) * float(os.environ.get("BUDGET_SCALE", "1"))
payload = b'{"session_id":"bench","tool_name":"Bash","tool_input":{"command":"ls"}}'
cmd = [bin, "hook", "--harness", "claude", "--event", "PreToolUse"]
def once():
    t = time.perf_counter()
    subprocess.run(cmd, input=payload, check=True)
    return (time.perf_counter() - t) * 1000
for _ in range(20):
    once()
rounds = []
for _ in range(3):
    runs = sorted(once() for _ in range(200))
    rounds.append((runs[189], runs[99]))
p95, p50 = min(rounds)
ok = p95 < budget
spread = ", ".join(f"{r[0]:.1f}" for r in rounds)
print(f"daemon {label}: p50 {p50:.1f} ms, p95 {p95:.1f} ms (round p95s {spread}; budget p95 < {budget:.0f} ms) {'ok' if ok else 'OVER BUDGET'}")
sys.exit(0 if ok else 1)
PY
}

"$bin" daemon start >/dev/null
status=0
measure up 20 || status=1
"$bin" daemon stop >/dev/null
measure down 60 || status=1
exit $status
