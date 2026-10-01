package tui

import (
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/giovaniif/agent-workspace/internal/domain"
	"github.com/giovaniif/agent-workspace/internal/rpc"
)

const maxQueueRows = 8

type launchInput struct {
	text string
	err  string
	busy bool
	seq  int
}

type launchSentMsg struct{ seq int }

type launchFailedMsg struct {
	seq int
	err error
}

type queuedCall struct {
	method string
	params any
}

func (m Model) openLauncher() Model {
	m.launches++
	m.launching = &launchInput{seq: m.launches}
	return m
}

func (m Model) launcherKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	in := *m.launching
	m.launching = &in
	switch msg.String() {
	case "esc", "ctrl+c":
		m.launching = nil
		return m, nil
	}
	if in.busy {
		return m, nil
	}
	switch msg.String() {
	case "enter":
		return m, m.sendLaunch()
	case "ctrl+j":
		in.text += "\n"
	case "ctrl+u":
		in.text = ""
	case "backspace":
		if r := []rune(in.text); len(r) > 0 {
			in.text = string(r[:len(r)-1])
		}
	default:
		in.text += strings.ReplaceAll(msg.Text, "\n", " ")
	}
	in.err = ""
	return m, nil
}

func (m Model) launcherPaste(s string) Model {
	in := *m.launching
	if !in.busy {
		in.text += strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\r", "\n")
		in.err = ""
	}
	m.launching = &in
	return m
}

func (m Model) sendLaunch() tea.Cmd {
	in := m.launching
	issues, _ := domain.ParseIssueURLs(in.text)
	if len(issues) == 0 {
		in.err = "no Linear issue URL in the input"
		return nil
	}
	c := m.opts.Calls
	if c == nil {
		in.err = "not connected to the daemon"
		return nil
	}
	in.busy = true
	seq := in.seq
	defaults := m.opts.Defaults[domain.HarnessClaude]
	p := rpc.LauncherEnqueueParams{Input: in.text, Harness: string(domain.HarnessClaude), Model: defaults.Model, Effort: defaults.Effort}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
		defer cancel()
		if err := c.Call(ctx, rpc.MethodLauncherEnqueue, p, nil); err != nil {
			return launchFailedMsg{seq, err}
		}
		return launchSentMsg{seq}
	}
}

func (m Model) launcherLines() []string {
	s := m.styles
	in := m.launching
	out := []string{"", m.line(false, []piece{{s.header, " LAUNCH LINEAR ISSUES"}}, nil), ""}
	lines := strings.Split(in.text, "\n")
	for i, l := range lines {
		cursor := ""
		if i == len(lines)-1 {
			cursor = "▏"
		}
		out = append(out, m.line(false, []piece{{s.text, " " + cleanText(l) + cursor}}, nil))
	}
	issues, rejected := domain.ParseIssueURLs(in.text)
	summary := []piece{{s.sub, " " + count(len(issues), "issue")}}
	if len(rejected) > 0 {
		summary = append(summary, piece{s.peach, fmt.Sprintf(" · %d not a Linear issue URL", len(rejected))})
	}
	out = append(out, "", m.line(false, summary, nil))
	switch {
	case in.busy:
		out = append(out, m.line(false, []piece{{s.sub, " queuing…"}}, nil))
	case in.err != "":
		out = append(out, m.line(false, []piece{{s.peach, " ✗ " + in.err}}, nil))
	}
	return append(out, m.line(false, []piece{{s.dim, " ⏎ queue · ^j new line · ^u clear · esc cancel"}}, nil))
}

func (m Model) waitingItems() []domain.LaunchItem {
	var out []domain.LaunchItem
	for _, i := range m.queue {
		if !i.Starting && i.Err == "" {
			out = append(out, i)
		}
	}
	return out
}

func (m Model) queueOffers() map[string]domain.FallbackOffer {
	waiting := m.waitingItems()
	requests := make([]domain.StartRequest, len(waiting))
	for i, w := range waiting {
		requests[i] = w.Request
	}
	out := map[string]domain.FallbackOffer{}
	for i, offer := range domain.OfferFallbacks(m.quotas(), m.opts.Fallback, requests) {
		out[waiting[i].ID] = offer
	}
	return out
}

func (m Model) queueLines() []string {
	if len(m.queue) == 0 {
		return nil
	}
	s := m.styles
	offers := m.queueOffers()
	out := []string{"", m.line(false, []piece{{s.header, " QUEUE"}}, []piece{{s.sub, count(len(m.queue), "issue") + " "}})}
	for n, item := range m.queue {
		if n == maxQueueRows {
			out = append(out, m.line(false, []piece{{s.dim, fmt.Sprintf("   +%d more", len(m.queue)-maxQueueRows)}}, nil))
			break
		}
		detail := strings.TrimSpace(fmt.Sprintf("%s %s %s", item.Request.Harness, item.Request.Model, item.Request.Effort))
		state := piece{s.dim, "queued"}
		switch {
		case item.Starting:
			state = piece{s.blue, "starting…"}
		case item.Err != "":
			state = piece{s.peach, "✗ " + cleanText(item.Err)}
		}
		out = append(out, m.line(false, []piece{{s.text, " ○ "}, {s.bold, item.Ref}, {s.text, "  "}, state}, []piece{{s.sub, detail + " "}}))
		if offer, ok := offers[item.ID]; ok {
			text := strings.TrimRight(fmt.Sprintf("   ↳ c → %s %d%% %s", offer.Request.Harness, offer.Advice.OtherShortest.LeftPercent, offer.Request.Model), " ")
			out = append(out, m.line(false, []piece{{s.peach, text}}, nil))
		}
	}
	return out
}

func (m Model) callEach(calls []queuedCall) tea.Cmd {
	c := m.opts.Calls
	if c == nil || len(calls) == 0 {
		return nil
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
		defer cancel()
		for _, call := range calls {
			if err := c.Call(ctx, call.method, call.params, nil); err != nil {
				return errMsg{err}
			}
		}
		return nil
	}
}

func (m Model) takeQueueOffers() tea.Cmd {
	offers := m.queueOffers()
	var calls []queuedCall
	for _, item := range m.waitingItems() {
		if offer, ok := offers[item.ID]; ok {
			r := offer.Request
			calls = append(calls, queuedCall{rpc.MethodLauncherRetarget, rpc.LauncherRetargetParams{ID: item.ID, Harness: string(r.Harness), Model: r.Model, Effort: r.Effort}})
		}
	}
	return m.callEach(calls)
}

func (m Model) clearQueue() tea.Cmd {
	var calls []queuedCall
	for _, item := range m.queue {
		if !item.Starting {
			calls = append(calls, queuedCall{rpc.MethodLauncherDrop, rpc.LauncherItemRef{ID: item.ID}})
		}
	}
	return m.callEach(calls)
}
