# ADR 0008: tdd-check exempts port fake files

Status: accepted, 2026-09-29.

## Decision

- `scripts/tdd-check` skips added or changed test files named `fakes_test.go` or `*_fake_test.go`.
- In-memory fakes of ports live only in those files, one per package.
- Behavior tests never go in them. Every other test file is still checked and must fail on base.

## Why

- When a port gains a method, every fake must gain it too. That edit is a test-file change with no behavior to fail on base, so the per-package check failed it (ADR 0002).
- Keeping fakes in their own files makes the exemption a filename rule, with no diff parsing.

## Rejected

- **Exempting all test-only changes:** would let behavior tests that already pass slip through.
