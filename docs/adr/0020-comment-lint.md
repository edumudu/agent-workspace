# ADR 0020: Comments are denied unless marked with a reason

Status: superseded by [ADR 0043](0043-no-comments.md) on 2026-10-03. Was accepted 2026-09-30; revised 2026-10-01 (#124) to deny by default.

## Decision

- A comment exists only to state a reason the code cannot show: a hidden constraint, a specific bug or external quirk it works around, or behavior that would surprise. Exported identifiers are no exception.
- Every Go comment group opens with a marker: `// why: <text>` or `// bug: <text>` (`/* why: */` for block comments). `bug:` names a concrete bug or tool quirk being worked around; `why:` covers everything else. Only the first line of a multi-line comment carries it. Doc comments, package docs and trailing comments are covered too.
- Exempt: tool directives (`//go:`, `//line`, `//export`, `//nolint:` with a reason after `//`), `// TODO(#n)`, `// Code generated ... DO NOT EDIT.`, and `// Copyright` / `// SPDX-License-Identifier:` headers. A directive line inside a group resets it, so the comment after it needs its own marker.
- `scripts/lint-comments` enforces this and `make lint` runs it. Non-Go files (shell, YAML, Lua, txtar) follow the same rule with `# why:` / `-- why:`; review covers them, not the linter.
- CodeRabbit (`.coderabbit.yaml`) flags any added or kept comment without such a reason and asks for its removal, since a marker alone does not prove the reason is real.
- `scripts/tdd-check` ignores test changes that touch only comments, so a comment cleanup does not need a failing test.

## Why

- The first version of this ADR only rejected docs that literally restated the declaration's name, and banners. Synonym restatements, "what" comments and narrating `why:` lines slipped through, and comments piled up.
- A required marker makes the author name the reason, and makes a missing one a lint failure instead of a review debate. The restating-doc and banner heuristics are dropped: an unmarked comment already fails.

## Rejected

- **Allow plain doc comments on exported identifiers for go doc:** most restate the signature; nothing here is a published library.
- **One marker only:** `bug:` makes workarounds easy to find and remove once the bug is fixed.
