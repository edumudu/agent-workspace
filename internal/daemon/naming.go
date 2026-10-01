package daemon

import (
	"context"
	"encoding/json"
	"log"
	"strings"
	"time"

	"github.com/giovaniif/agent-workspace/internal/app"
	"github.com/giovaniif/agent-workspace/internal/domain"
	"github.com/giovaniif/agent-workspace/internal/rpc"
)

const titleTimeout = 10 * time.Second

func WithTitles(r app.TitleResolver) Option {
	return func(d *Daemon) { d.titles = r }
}

// why: the title is merged into the task as it is when the answer lands, so
// a pin made meanwhile stays.
func (d *Daemon) resolveTitleAsync(task domain.Task) {
	if d.titles == nil || task.Source == domain.TaskText {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(d.ws.ctx, titleTimeout)
		defer cancel()
		title, err := d.titles.Title(ctx, task)
		if err != nil {
			log.Printf("title for %s: %v", task.Ref, err)
			return
		}
		d.query(func(s *state) {
			cur, ok := s.tasks[task.ID]
			if !ok {
				return
			}
			if next, changed := domain.WithTitle(cur, title); changed {
				s.emit(TaskChanged{Task: next})
			}
		})
	}()
}

func (d *Daemon) pinName(req rpc.Request) (*rpc.Response, bool) {
	var id, name string
	if req.Method == rpc.MethodSessionRename {
		var p rpc.SessionRenameParams
		if err := json.Unmarshal(req.Params, &p); err != nil {
			return errorResponse(req.ID, rpc.CodeBadRequest, "session.rename params: "+err.Error()), true
		}
		if strings.TrimSpace(p.Name) == "" {
			return errorResponse(req.ID, rpc.CodeBadRequest, "name is empty"), true
		}
		id, name = p.ID, p.Name
	} else {
		var ref rpc.SessionRef
		if err := json.Unmarshal(req.Params, &ref); err != nil {
			return errorResponse(req.ID, rpc.CodeBadRequest, "session.unpin params: "+err.Error()), true
		}
		id = ref.ID
	}
	found := false
	ok := d.query(func(s *state) {
		session, exists := s.sessions[id]
		if !exists {
			return
		}
		task, exists := s.tasks[session.TaskID]
		if !exists {
			return
		}
		found = true
		s.emit(TaskChanged{Task: domain.PinName(task, name)})
	})
	if ok && !found {
		return errorResponse(req.ID, rpc.CodeNotFound, "no session "+id), true
	}
	return result(req.ID, struct{}{}), ok
}
