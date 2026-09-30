package app

import (
	"context"
	"path/filepath"
	"sync"
	"time"

	"github.com/giovaniif/agent-workspace/internal/domain"
)

// WorktreeGitFacts is what git knows about one worktree. Fingerprint changes
// whenever HEAD or the status of any path does.
type WorktreeGitFacts struct {
	InDefault   bool
	OnDefault   bool
	Uncommitted int
	ModifiedAt  time.Time
	Fingerprint string
}

type CleanupGit interface {
	CleanupFacts(ctx context.Context, w domain.Worktree) (WorktreeGitFacts, error)
	// Backup writes the worktree's uncommitted changes, untracked files and
	// status under dir, which it creates.
	Backup(ctx context.Context, w domain.Worktree, dir string) error
	// CreateBranch points a new branch at HEAD without ever moving an
	// existing one, and returns the name it used.
	CreateBranch(ctx context.Context, w domain.Worktree, name string) (string, error)
	Prune(ctx context.Context, repo string) error
}

// ProcessTable maps each of paths to the processes whose cwd is inside it.
type ProcessTable interface {
	Holders(ctx context.Context, paths []string) (map[string][]string, error)
}

// Trash takes a directory out of place at once; deleting it is the trash's
// business, in the background.
type Trash interface {
	Move(path string) error
}

type CleanupAudit interface {
	Record(CleanupRecord)
}

type CleanupRecord struct {
	At      time.Time            `json:"at"`
	Path    string               `json:"path"`
	Branch  string               `json:"branch,omitempty"`
	Action  domain.CleanupAction `json:"action"`
	Reason  string               `json:"reason"`
	Outcome string               `json:"outcome"`
}

// SessionActivity is what the daemon knows about the worktree's session.
type SessionActivity struct {
	Live         bool
	LastActivity time.Time
}

type CleanupResult struct {
	Decision domain.CleanupDecision
	Outcome  string
}

// Cleanup plans and executes worktree cleanup. It remembers which states it
// already backed up, so a dirty worktree is not backed up on every run.
type Cleanup struct {
	git        CleanupGit
	procs      ProcessTable
	trash      Trash
	audit      CleanupAudit
	backupRoot string
	now        func() time.Time

	mu       sync.Mutex
	backedUp map[string]string
}

func NewCleanup(git CleanupGit, procs ProcessTable, trash Trash, audit CleanupAudit, backupRoot string, now func() time.Time) *Cleanup {
	return &Cleanup{git: git, procs: procs, trash: trash, audit: audit, backupRoot: backupRoot, now: now, backedUp: map[string]string{}}
}

type plannedWorktree struct {
	decision    domain.CleanupDecision
	fingerprint string
}

// Plan decides every worktree without changing anything.
func (c *Cleanup) Plan(ctx context.Context, wts []domain.Worktree, activity func(domain.Worktree) SessionActivity) []domain.CleanupDecision {
	planned := c.plan(ctx, wts, activity)
	out := make([]domain.CleanupDecision, len(planned))
	for i, p := range planned {
		out[i] = p.decision
	}
	return out
}

func (c *Cleanup) plan(ctx context.Context, wts []domain.Worktree, activity func(domain.Worktree) SessionActivity) []plannedWorktree {
	paths := make([]string, len(wts))
	for i, w := range wts {
		paths[i] = w.Path
	}
	holders, procErr := c.procs.Holders(ctx, paths)
	gitFacts := make([]WorktreeGitFacts, len(wts))
	gitErrs := make([]error, len(wts))
	var wg sync.WaitGroup
	sem := make(chan struct{}, refreshParallelism)
	for i := range wts {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			gitFacts[i], gitErrs[i] = c.git.CleanupFacts(ctx, wts[i])
		}()
	}
	wg.Wait()
	now := c.now()
	out := make([]plannedWorktree, len(wts))
	for i, w := range wts {
		g, act := gitFacts[i], activity(w)
		f := domain.CleanupFacts{
			InDefault:    g.InDefault,
			OnDefault:    g.OnDefault,
			Uncommitted:  g.Uncommitted,
			Holders:      holders[w.Path],
			SessionLive:  act.Live,
			LastActivity: latest(g.ModifiedAt, act.LastActivity),
		}
		switch {
		case procErr != nil:
			f.Unknown = "process check failed: " + procErr.Error()
		case gitErrs[i] != nil:
			f.Unknown = gitErrs[i].Error()
		}
		out[i] = plannedWorktree{decision: domain.PlanCleanup(w, f, now), fingerprint: g.Fingerprint}
	}
	return out
}

func latest(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

// Execute plans afresh and acts on it: dirty or detached worktrees are
// backed up and kept for the user, merged clean ones go to the trash once a
// last process check still finds nobody inside, and their repos are pruned.
func (c *Cleanup) Execute(ctx context.Context, wts []domain.Worktree, activity func(domain.Worktree) SessionActivity) []CleanupResult {
	planned := c.plan(ctx, wts, activity)
	results := make([]CleanupResult, len(planned))
	var removals []string
	for i, p := range planned {
		results[i] = CleanupResult{Decision: p.decision, Outcome: "kept"}
		switch p.decision.Action {
		case domain.CleanupBackupThenAsk:
			results[i].Outcome = c.backup(ctx, p)
		case domain.CleanupRemove:
			removals = append(removals, p.decision.Worktree.Path)
		}
	}
	// why: a shell or editor may have entered a worktree while facts were gathered.
	var holders map[string][]string
	var procErr error
	if len(removals) > 0 {
		holders, procErr = c.procs.Holders(ctx, removals)
	}
	pruned := map[string]bool{}
	var repos []string
	for i, r := range results {
		d := r.Decision
		if d.Action != domain.CleanupRemove {
			continue
		}
		switch h := holders[d.Worktree.Path]; {
		case procErr != nil:
			results[i].Outcome = "kept: process check failed: " + procErr.Error()
		case len(h) > 0:
			results[i].Outcome = "kept: in use by " + h[0]
		default:
			if err := c.trash.Move(d.Worktree.Path); err != nil {
				results[i].Outcome = "failed: " + err.Error()
				break
			}
			results[i].Outcome = "removed"
			if !pruned[d.Worktree.Repo] {
				pruned[d.Worktree.Repo] = true
				repos = append(repos, d.Worktree.Repo)
			}
		}
	}
	for _, repo := range repos {
		_ = c.git.Prune(ctx, repo)
	}
	for _, r := range results {
		if r.Outcome != "kept" && r.Outcome != "already backed up" {
			c.record(r)
		}
	}
	return results
}

func (c *Cleanup) backup(ctx context.Context, p plannedWorktree) string {
	w := p.decision.Worktree
	c.mu.Lock()
	done := p.fingerprint != "" && c.backedUp[w.ID] == p.fingerprint
	c.mu.Unlock()
	if done {
		return "already backed up"
	}
	dir := filepath.Join(c.backupRoot, c.now().Format("20060102-150405"), filepath.Base(w.Path))
	if err := c.git.Backup(ctx, w, dir); err != nil {
		return "failed: " + err.Error()
	}
	outcome := "backed up to " + dir
	if p.decision.BackupBranch != "" {
		name, err := c.git.CreateBranch(ctx, w, p.decision.BackupBranch)
		if err != nil {
			return outcome + ", branch failed: " + err.Error()
		}
		outcome += ", branch " + name
	}
	c.mu.Lock()
	c.backedUp[w.ID] = p.fingerprint
	c.mu.Unlock()
	return outcome
}

func (c *Cleanup) record(r CleanupResult) {
	d := r.Decision
	c.audit.Record(CleanupRecord{
		At:      c.now(),
		Path:    d.Worktree.Path,
		Branch:  d.Worktree.Branch,
		Action:  d.Action,
		Reason:  d.Reason,
		Outcome: r.Outcome,
	})
}
