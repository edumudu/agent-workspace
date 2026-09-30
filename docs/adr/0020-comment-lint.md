# ADR 0020: Lint restating doc comments and banners

Status: accepted, 2026-09-30.

## Decision

- `scripts/lint-comments` checks every func, type, const and var doc comment. It fails when the first word is the declared name and every other word (numbers count as words) is either filler (a, the, is, returns, new, creates, gets, sets, of, for, given, which, that and a few more) or a word taken from the name, the receiver, the parameter names or the type names. Words are split on camel case and compared after stripping plural and verb endings, so `// Close closes the store.` on `(s *Store) Close()` fails.
- It fails on section banners: a top-level comment outside any declaration that is a run of `-`, `=`, `*`, `#`, `/`, `~` or `_` with at most four words, or a lone line of at most three words with no `:`, `,`, `;`, parentheses or sentence punctuation (`.`, `?`, `!`), so `// See README.` passes.
- A grouped `const (...)`, `var (...)` or `type (...)` doc is not checked, only each spec's own doc.
- `make lint` runs it, and CI runs `make lint`.

## Why

- A doc that repeats the signature costs a read and says nothing.- The check only fires when nothing is left, so it has no false positives on a doc that adds a single real fact. It misses restatements that use a synonym (`// Option configures a Daemon.`); those stay a review call.
