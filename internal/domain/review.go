package domain

import (
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"path/filepath"
	"strconv"
	"strings"
)

type ReviewScope string

const (
	ScopeLastTurn    ReviewScope = "last_turn"
	ScopeUncommitted ReviewScope = "uncommitted"
	ScopeBranch      ReviewScope = "branch"
)

// ReviewScopes is the order the scope toggle cycles through.
var ReviewScopes = []ReviewScope{ScopeLastTurn, ScopeUncommitted, ScopeBranch}

var (
	ErrNoTurn          = errors.New("no prompt since the review began")
	ErrNoDefaultBranch = errors.New("the repo has no origin default branch")
	ErrUnknownScope    = errors.New("unknown review scope")
)

// Shift moves delta steps through ReviewScopes, wrapping. An unknown scope
// counts as the first.
func (s ReviewScope) Shift(delta int) ReviewScope {
	i := 0
	for j, x := range ReviewScopes {
		if x == s {
			i = j
		}
	}
	n := len(ReviewScopes)
	return ReviewScopes[((i+delta)%n+n)%n]
}

// RangeFacts is what a worktree knows when a review opens: its newest turn
// snapshot ref (empty if none) and its repo's default branch.
type RangeFacts struct {
	LatestTurn    string
	DefaultBranch string
}

// DiffRange is where a scope's diff starts; it always ends at the working
// tree. With MergeBase, From is resolved to its merge base with HEAD.
type DiffRange struct {
	From      string
	MergeBase bool
}

func RangeFor(scope ReviewScope, f RangeFacts) (DiffRange, error) {
	switch scope {
	case ScopeLastTurn:
		if f.LatestTurn == "" {
			return DiffRange{}, ErrNoTurn
		}
		return DiffRange{From: f.LatestTurn}, nil
	case ScopeUncommitted:
		return DiffRange{From: "HEAD"}, nil
	case ScopeBranch:
		if f.DefaultBranch == "" {
			return DiffRange{}, ErrNoDefaultBranch
		}
		return DiffRange{From: "origin/" + f.DefaultBranch, MergeBase: true}, nil
	}
	return DiffRange{}, ErrUnknownScope
}

const turnPrefix = "refs/agentws/turns/"

// WorktreeKey names a worktree inside a ref. Worktrees of one repo share its
// refs, so turn refs carry the worktree they snapshot.
func WorktreeKey(path string) string {
	sum := sha1.Sum([]byte(filepath.Clean(path)))
	return hex.EncodeToString(sum[:6])
}

// TurnRef is refs/agentws/turns/<session>/<worktree key>/<n>.
func TurnRef(session, worktree string, n int) string {
	return turnPrefix + refSafe(session) + "/" + WorktreeKey(worktree) + "/" + strconv.Itoa(n)
}

func refSafe(s string) string {
	var b strings.Builder
	for _, r := range s {
		ok := r == '-' || r == '_' || r == '.' || (r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
		if !ok {
			r = '-'
		}
		b.WriteRune(r)
	}
	return strings.Trim(b.String(), ".")
}

type turn struct {
	ref     string
	session string
	key     string
	n       int
	numeric bool
}

func parseTurn(ref string) (turn, bool) {
	rest, ok := strings.CutPrefix(ref, turnPrefix)
	if !ok {
		return turn{}, false
	}
	parts := strings.Split(rest, "/")
	if len(parts) != 3 {
		return turn{}, false
	}
	n, err := strconv.Atoi(parts[2])
	return turn{ref: ref, session: parts[0], key: parts[1], n: n, numeric: err == nil}, true
}

// LatestTurn is the highest-numbered turn ref of session in worktree, or ""
// and 0 when it has none.
func LatestTurn(refs []string, session, worktree string) (string, int) {
	best, bestN := "", 0
	for _, t := range turnsOf(refs, session, worktree) {
		if t.numeric && t.n > bestN {
			best, bestN = t.ref, t.n
		}
	}
	return best, bestN
}

// OlderTurns are session's turn refs in worktree numbered below n: the ones
// a new snapshot n makes obsolete.
func OlderTurns(refs []string, session, worktree string, n int) []string {
	var out []string
	for _, t := range turnsOf(refs, session, worktree) {
		if t.numeric && t.n < n {
			out = append(out, t.ref)
		}
	}
	return out
}

// TurnsOfWorktree is every turn ref of any session in worktree, the ones to
// drop when it is removed.
func TurnsOfWorktree(refs []string, worktree string) []string {
	key := WorktreeKey(worktree)
	var out []string
	for _, r := range refs {
		if t, ok := parseTurn(r); ok && t.key == key {
			out = append(out, r)
		}
	}
	return out
}

func turnsOf(refs []string, session, worktree string) []turn {
	key, s := WorktreeKey(worktree), refSafe(session)
	var out []turn
	for _, r := range refs {
		if t, ok := parseTurn(r); ok && t.key == key && t.session == s {
			out = append(out, t)
		}
	}
	return out
}

type FileStatus string

const (
	FileAdded    FileStatus = "A"
	FileModified FileStatus = "M"
	FileDeleted  FileStatus = "D"
	FileRenamed  FileStatus = "R"
)

type LineKind byte

const (
	LineContext LineKind = ' '
	LineAdded   LineKind = '+'
	LineDeleted LineKind = '-'
)

// DiffLine numbers are 1-based; Old is 0 on an added line and New on a
// deleted one.
type DiffLine struct {
	Kind LineKind
	Old  int
	New  int
	Text string
}

type Hunk struct {
	Header string
	Lines  []DiffLine
}

// FileDiff.Blob is the new side's blob hash from the index line
// (zeros for a deletion), which identifies the content a viewed mark saw.
type FileDiff struct {
	Path    string
	OldPath string
	Status  FileStatus
	Added   int
	Deleted int
	Binary  bool
	Blob    string
	Hunks   []Hunk
}

// ParseDiff reads `git diff` output made without color or external diff.
// Paths are taken from the header lines, so quoted paths stay quoted.
func ParseDiff(out string) []FileDiff {
	var files []FileDiff
	var f *FileDiff
	var h *Hunk
	oldN, newN := 0, 0
	for line := range strings.Lines(out) {
		line = strings.TrimSuffix(line, "\n")
		if rest, ok := strings.CutPrefix(line, "diff --git "); ok {
			files = append(files, FileDiff{Status: FileModified, Path: headerPath(rest)})
			f, h = &files[len(files)-1], nil
			continue
		}
		if f == nil {
			continue
		}
		if h != nil {
			switch {
			case strings.HasPrefix(line, "+"):
				h.Lines = append(h.Lines, DiffLine{Kind: LineAdded, New: newN, Text: line[1:]})
				newN++
				f.Added++
				continue
			case strings.HasPrefix(line, "-"):
				h.Lines = append(h.Lines, DiffLine{Kind: LineDeleted, Old: oldN, Text: line[1:]})
				oldN++
				f.Deleted++
				continue
			case strings.HasPrefix(line, " "):
				h.Lines = append(h.Lines, DiffLine{Kind: LineContext, Old: oldN, New: newN, Text: line[1:]})
				oldN++
				newN++
				continue
			case strings.HasPrefix(line, `\`):
				continue
			}
		}
		switch {
		case strings.HasPrefix(line, "@@ "):
			f.Hunks = append(f.Hunks, Hunk{Header: line})
			h = &f.Hunks[len(f.Hunks)-1]
			oldN, newN = hunkStarts(line)
		case strings.HasPrefix(line, "new file mode"):
			f.Status = FileAdded
		case strings.HasPrefix(line, "deleted file mode"):
			f.Status = FileDeleted
		case strings.HasPrefix(line, "rename from "):
			f.Status = FileRenamed
			f.OldPath = strings.TrimPrefix(line, "rename from ")
		case strings.HasPrefix(line, "rename to "):
			f.Path = strings.TrimPrefix(line, "rename to ")
		case strings.HasPrefix(line, "index "):
			f.Blob = indexBlob(line)
		case strings.HasPrefix(line, "Binary files "):
			f.Binary = true
		case strings.HasPrefix(line, "+++ "):
			if p := strings.TrimPrefix(line, "+++ "); p != "/dev/null" {
				f.Path = strings.TrimPrefix(p, "b/")
			}
		case strings.HasPrefix(line, "--- "):
			if p := strings.TrimPrefix(line, "--- "); p != "/dev/null" && f.Status != FileRenamed {
				f.Path = strings.TrimPrefix(p, "a/")
			}
		}
	}
	return files
}

// headerPath takes b/<path> from "a/<path> b/<path>"; both halves are equal
// unless the file was renamed, and a rename names its paths again later.
func headerPath(rest string) string {
	if i := strings.Index(rest, " b/"); i >= 0 {
		return rest[i+3:]
	}
	return rest
}

func indexBlob(line string) string {
	fields := strings.Fields(strings.TrimPrefix(line, "index "))
	if len(fields) == 0 {
		return ""
	}
	_, blob, _ := strings.Cut(fields[0], "..")
	return blob
}

// hunkStarts reads the start lines of "@@ -a[,b] +c[,d] @@". A hunk with a
// zero count starts one before its first line, so the next line is start+1.
func hunkStarts(header string) (int, int) {
	fields := strings.Fields(header)
	if len(fields) < 3 {
		return 0, 0
	}
	start := func(s string) int {
		num, count, hasCount := strings.Cut(s[1:], ",")
		n, _ := strconv.Atoi(num)
		if hasCount && count == "0" {
			return n + 1
		}
		return n
	}
	return start(fields[1]), start(fields[2])
}

// ViewedMark says the user viewed Path in Worktree when its content was Blob.
type ViewedMark struct {
	Worktree string
	Path     string
	Blob     string
}

func (m ViewedMark) Key() string { return m.Worktree + "\x00" + m.Path }

// IsViewed holds only while the file's content is the one that was viewed,
// so a mark resets as soon as the file changes again.
func IsViewed(marks map[string]ViewedMark, worktree string, f FileDiff) bool {
	m, ok := marks[ViewedMark{Worktree: worktree, Path: f.Path}.Key()]
	return ok && m.Blob == f.Blob
}

// SplitRow is one row of the split view; a nil side is blank.
type SplitRow struct {
	Left  *DiffLine
	Right *DiffLine
}

// SplitRows pairs each run of deletions with the additions that follow it,
// line by line, and puts context on both sides.
func SplitRows(h Hunk) []SplitRow {
	var rows []SplitRow
	lines := h.Lines
	for i := 0; i < len(lines); {
		if lines[i].Kind == LineContext {
			l := lines[i]
			rows = append(rows, SplitRow{Left: &l, Right: &l})
			i++
			continue
		}
		var dels, adds []DiffLine
		for i < len(lines) && lines[i].Kind == LineDeleted {
			dels = append(dels, lines[i])
			i++
		}
		for i < len(lines) && lines[i].Kind == LineAdded {
			adds = append(adds, lines[i])
			i++
		}
		for j := range max(len(dels), len(adds)) {
			var r SplitRow
			if j < len(dels) {
				r.Left = &dels[j]
			}
			if j < len(adds) {
				r.Right = &adds[j]
			}
			rows = append(rows, r)
		}
	}
	return rows
}

// WorktreeReview is one worktree's files in a review. Err is set, and Files
// empty, when its diff could not be built.
type WorktreeReview struct {
	Worktree Worktree
	Files    []FileDiff
	Err      string
}
