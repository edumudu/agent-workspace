# ADR 0043: tdd job checks that tests are committed first

Status: accepted, 2026-10-03.

## Decision

- `scripts/tdd-check` walks every non-merge commit in `base..head`. A commit fails the job if it adds or changes a test file and also changes production code.
- A test file is one the PR-wide check counts: a `*_test.go` file or a file under a `testdata/` dir, minus port fakes (ADR 0009) and minus whitespace-only or comment-only changes. Production code is any file outside `*_test.go`, `testdata/` and `test/`, except Markdown.
- A commit whose subject starts with `refactor` is exempt: renames and moves have to change both sides to keep compiling, and change no behavior.
- The commit check runs before the "tests fail on base" check, and both must pass.

## Why

- The root AGENTS.md says "Commit order proves it", but the job only checked that the PR's tests fail on base. A PR that put its tests and its code in one `feat:` commit passed, so test-first was not enforced (PR #156).
- Checking that each test commit fails on its parent would need a build per commit. Keeping tests and code in separate commits is cheap to check and is what the workflow already asks for.

## Rejected

- **Running each test commit against its parent:** a `go test` run per commit is slow on macOS runners, and the PR-wide run already proves the tests fail on base.
- **No refactor exemption:** a rename would then need a commit that does not compile.
- **Counting Markdown as production code:** docs edits carry no behavior to test.
