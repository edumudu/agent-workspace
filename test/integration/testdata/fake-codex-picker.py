#!/usr/bin/env python3
# Plays Codex 0.159's `/model` flow in a real terminal: a model popup, then a
# reasoning level popup, each moved with the arrow keys. Each finished choice
# is appended to the file named by $FAKE_CODEX_OUT as "<model> <effort>".
import os
import select
import sys
import termios
import tty

MODELS = ["GPT-6.1-Sol", "GPT-6-Astra", "GPT-6-Luna"]
EFFORTS = ["Low", "Medium", "High", "Extra high"]

out_path = os.environ["FAKE_CODEX_OUT"]
fd = sys.stdin.fileno()
tty.setraw(fd)
sys.stdout.write("\x1b[?2004h")

state = {"buf": "", "popup": None, "rows": [], "hi": 0, "model": None}


def draw():
    sys.stdout.write("\x1b[2J\x1b[H")
    sys.stdout.write("• earlier output\r\n  1. a numbered list\r\n\r\n")
    if state["popup"] is None:
        sys.stdout.write("› " + state["buf"])
    else:
        sys.stdout.write("  " + state["popup"] + "\r\n\r\n")
        for i, row in enumerate(state["rows"]):
            mark = "›" if i == state["hi"] else " "
            sys.stdout.write("%s %d. %s    description\r\n" % (mark, i + 1, row))
        sys.stdout.write("\r\n  Press enter to confirm or esc to go back")
    sys.stdout.flush()


def read_key():
    ch = os.read(fd, 1)
    if ch != b"\x1b":
        return ch.decode(errors="replace")
    seq = b""
    while select.select([fd], [], [], 0.05)[0]:
        seq += os.read(fd, 1)
        if seq[-1:].isalpha() or seq.endswith(b"~"):
            break
    return "\x1b" + seq.decode(errors="replace")


draw()
while True:
    k = read_key()
    if k in ("\x1b[200~", "\x1b[201~"):
        continue
    if state["popup"] is None:
        if k == "\r":
            if state["buf"] == "/model":
                state.update(popup="Select Model and Effort", rows=MODELS, hi=0)
            state["buf"] = ""
        elif len(k) == 1 and k.isprintable():
            state["buf"] += k
    elif k == "\x1b[A":
        state["hi"] = max(0, state["hi"] - 1)
    elif k == "\x1b[B":
        state["hi"] = min(len(state["rows"]) - 1, state["hi"] + 1)
    elif k == "\x1b":
        state.update(popup=None, model=None)
    elif k == "\r" and state["model"] is None:
        state["model"] = state["rows"][state["hi"]]
        state.update(popup="Select Reasoning Level for " + state["model"], rows=EFFORTS, hi=1)
    elif k == "\r":
        with open(out_path, "a") as f:
            f.write("%s %s\n" % (state["model"], state["rows"][state["hi"]]))
        state.update(popup=None, model=None)
    draw()
