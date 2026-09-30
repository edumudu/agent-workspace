package daemon

import (
	"reflect"
	"time"

	"github.com/giovaniif/agent-workspace/internal/adapters/codex"
	"github.com/giovaniif/agent-workspace/internal/domain"
)

const (
	// usageTailBytes bounds how much of a rollout one read touches.
	usageTailBytes = 512 << 10
	// usageThrottle spaces out reads that tool-use hooks trigger; a turn can
	// fire dozens of them a second.
	usageThrottle = 2 * time.Second
)

type usageJob struct {
	running   bool
	again     bool
	lastStart time.Time
}

// requestUsage starts a worker that reads the session's rollout, unless one
// is already running (then it reads once more when done) or the last start
// was too recent and the request is not forced. It runs on the loop and never
// blocks it.
func (d *Daemon) requestUsage(s *state, sessionID, path string, force bool) {
	job := s.usage[sessionID]
	if job == nil {
		job = &usageJob{}
		s.usage[sessionID] = job
	}
	if job.running {
		job.again = true
		return
	}
	if !force && time.Since(job.lastStart) < usageThrottle {
		return
	}
	job.running = true
	job.lastStart = time.Now()
	go d.readUsage(sessionID, path)
}

func (d *Daemon) readUsage(sessionID, path string) {
	for {
		snap, err := codex.ReadSnapshotFile(path, usageTailBytes)
		again := false
		ran := d.query(func(s *state) {
			job := s.usage[sessionID]
			if current, ok := s.sessions[sessionID]; ok && err == nil {
				if next := withSnapshot(current, snap); next.Model != current.Model ||
					next.Effort != current.Effort || next.Usage != current.Usage ||
					!next.LimitsAt.Equal(current.LimitsAt) || next.SwitchWarning != current.SwitchWarning ||
					!reflect.DeepEqual(next.Switches, current.Switches) {
					s.emit(SessionChanged{Session: next})
				}
			}
			again, job.again = job.again, false
			job.running = again
		})
		if !ran || !again {
			return
		}
	}
}

func withSnapshot(s domain.Session, snap codex.Snapshot) domain.Session {
	s = s.Report(domain.StatusReport{Model: snap.Model, Effort: snap.Effort, At: snap.TurnAt})
	usage := snap.Usage()
	if snap.ContextLeftPercent != codex.UnknownPercent {
		s.Usage.ContextLeftPercent = usage.ContextLeftPercent
	}
	if len(snap.Limits) > 0 {
		s.Usage.LimitUsedPercent = usage.LimitUsedPercent
		limits := snap.RateLimits()
		at := snap.LimitsAt
		if at.IsZero() {
			// why: a rollout without timestamps must not look freshly reported on every read.
			at = time.Now()
			if reflect.DeepEqual(limits, s.Limits) && !s.LimitsAt.IsZero() {
				at = s.LimitsAt
			}
		}
		s.Limits, s.LimitsAt = limits, at
	}
	return s
}
