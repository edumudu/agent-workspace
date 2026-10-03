# ADR 0044: tdd-check compares Go tests by token

Status: accepted, 2026-10-03.

## Decision

- A changed `*_test.go` file counts only if its Go tokens changed. `scripts/tdd-check` builds a small `go/scanner` program at run time and compares the old and new versions with it. Whitespace and `//` and `/* */` comments are ignored; string literals are compared exactly.
- Other files under `testdata/` use `git diff --ignore-space-change --ignore-blank-lines`, and also ignore `//` and `#` comment lines except in `*.golden` and `*.go.txt` fixtures.
- Both checks list renames (`-M`). A moved and edited test counts at its new path, and the base run removes the old path before running the package's tests, so the old and new copies don't collide. It also removes every test file the PR deleted, because git reports a heavily rewritten move as a delete and an add.
- `scripts/tddcheck` tests the script by building throwaway modules and running it the way CI does.

## Why

- `--ignore-all-space` treated `"a b"` → `"ab"` in a test's expected value as no change, so a behavior change in a test could skip both checks.
- `-I '^[[:space:]]*(//|#)'` matched only line comments, so an edit to a block comment counted as a test change.
- The base run skipped renamed tests. Adding renames without removing the old path left duplicate test symbols on base, and the compile error passed as "fails on base".

## Rejected

- **Ignoring whitespace amount with `--ignore-space-change` in Go tests:** it still treats `"a  b"` and `"a b"` as equal.
- **A separate helper under `scripts/`:** CI copies `tdd-check` out of the tree before checking out base, so the script has to carry the scanner itself.
