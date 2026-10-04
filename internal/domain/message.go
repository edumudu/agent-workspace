package domain

import (
	"bytes"
	"encoding/json"
	"strings"
	"time"
	"unicode/utf8"
)

type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
	RoleSystem    Role = "system"
)

type ToolStatus string

const (
	ToolRunning ToolStatus = "running"
	ToolDone    ToolStatus = "done"
	ToolFailed  ToolStatus = "failed"
)

const (
	ToolSummaryRunes = 120
	ToolTextRunes    = 2000
)

type MessageTool struct {
	Name    string
	Summary string
	Status  ToolStatus
}

type Message struct {
	ID     string
	Cursor int64
	Turn   string
	Role   Role
	Text   string
	Tool   *MessageTool
	At     time.Time
}

type TranscriptLine struct {
	Start int64
	End   int64
	Data  []byte
}

func TranscriptFromHook(payload []byte) string {
	var p struct {
		TranscriptPath string `json:"transcript_path"`
	}
	if json.Unmarshal(payload, &p) != nil {
		return ""
	}
	return p.TranscriptPath
}

func TranscriptLines(data []byte, base int64) ([]TranscriptLine, int64) {
	var lines []TranscriptLine
	pos := 0
	for {
		i := bytes.IndexByte(data[pos:], '\n')
		if i < 0 {
			break
		}
		line := bytes.TrimSpace(data[pos : pos+i])
		end := pos + i + 1
		if len(line) > 0 {
			lines = append(lines, TranscriptLine{Start: base + int64(pos), End: base + int64(end), Data: line})
		}
		pos = end
	}
	return lines, base + int64(pos)
}

type MessageLog struct {
	out   []Message
	index map[string]int
	open  map[string]Message
}

func (l *MessageLog) Add(m Message) {
	if m.Tool != nil && m.Tool.Status == ToolRunning && m.ID != "" {
		if l.index == nil {
			l.index, l.open = map[string]int{}, map[string]Message{}
		}
		l.index[m.ID] = len(l.out)
		l.open[m.ID] = m
	}
	l.out = append(l.out, m)
}

func (l *MessageLog) Finish(result Message) {
	call, ok := l.open[result.ID]
	if !ok {
		l.out = append(l.out, result)
		return
	}
	delete(l.open, result.ID)
	tool := *call.Tool
	tool.Status = result.Tool.Status
	call.Tool = &tool
	call.Text = result.Text
	if i, ok := l.index[result.ID]; ok {
		l.out[i] = call
		delete(l.index, result.ID)
		return
	}
	l.out = append(l.out, call)
}

func (l *MessageLog) Drain() []Message {
	out := l.out
	l.out = nil
	clear(l.index)
	return out
}

func ToolResult(id string, status ToolStatus, text string) Message {
	return Message{ID: id, Role: RoleTool, Text: ClipText(text, ToolTextRunes), Tool: &MessageTool{Status: status}}
}

func ToolSummary(name, arg string) string {
	arg = OneLine(arg, ToolSummaryRunes)
	if arg == "" {
		return name
	}
	return OneLine(name+" "+arg, ToolSummaryRunes)
}

func OneLine(s string, max int) string {
	for _, l := range strings.Split(s, "\n") {
		if l = strings.Join(strings.Fields(l), " "); l != "" {
			return ClipText(l, max)
		}
	}
	return ""
}

func ClipText(s string, max int) string {
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	r := []rune(s)
	return strings.TrimSpace(string(r[:max-1])) + "…"
}
