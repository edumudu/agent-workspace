# ADR 0043: No comments

Status: accepted, 2026-10-03. Supersedes [ADR 0020](0020-comment-lint.md).

## Decision

- The repo has no comments: not in Go (tests included), shell, Lua, YAML, SQL, Makefiles or e2e `.txtar` scripts.
- Only tool directives remain, because tools read them: `//go:` lines, `//line`, `//export`, a bare `//nolint:<linters>`, `// Code generated ... DO NOT EDIT.`, a shebang, `# shellcheck` and `# yaml-language-server:` lines.
- `scripts/lint-comments` enforces this in `make lint`. It checks Go with `go/ast` (skipping Go files under `testdata/`) and the other languages line by line, outside quotes. It walks `.github/` too, and finds extensionless shell scripts by their shebang.
- What a comment used to carry goes elsewhere:
  - A hidden constraint or tool quirk goes into a test named after it, which fails if the workaround is removed, and into the commit message that adds the workaround.
  - Usage and design notes go in the scoped `AGENTS.md`, an ADR or the README.
  - Open work goes in a GitHub issue.

## Why

- ADR 0020 required a `why:` or `bug:` marker, but a linter cannot judge whether the reason is real. Agents added the marker to ordinary doc comments, so on 2026-10-03 about half of the 455 marked comments restated the code. `bug:` was also misused for gaps in our own code.
- A total ban is simple to state, simple to lint and leaves nothing to argue about in review.

## Consequences

- Tool quirks that comments used to explain are now visible only through tests and `git log -S`. Those included git's racy-index check, `lsof` and `du` exiting 1 with good output, and tmux falling back to another pane. A workaround without a test that pins it is at risk of being "simplified" away.
- The removal ran on 2026-10-03 in the commit "refactor: delete every comment"; `git show` on it recovers any deleted note.
