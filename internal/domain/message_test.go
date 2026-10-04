package domain

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestTranscriptFromHook(t *testing.T) {
	cases := []struct {
		name    string
		payload string
		want    string
	}{
		{"claude and codex hooks", `{"session_id":"s1","transcript_path":"/home/dev/.claude/projects/api/s1.jsonl"}`, "/home/dev/.claude/projects/api/s1.jsonl"},
		{"no path", `{"session_id":"s1"}`, ""},
		{"not json", `nope`, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := TranscriptFromHook([]byte(c.payload)); got != c.want {
				t.Fatalf("TranscriptFromHook = %q, want %q", got, c.want)
			}
		})
	}
}

func TestTranscriptLinesReturnsOnlyCompleteLinesWithTheirOffsets(t *testing.T) {
	cases := []struct {
		name  string
		data  string
		base  int64
		lines []TranscriptLine
		end   int64
	}{
		{"empty", "", 7, nil, 7},
		{"only a partial line", `{"a":1`, 7, nil, 7},
		{
			"complete lines from an offset, a blank one skipped, a partial one held back",
			"a\n\nbb\r\nccc", 10,
			[]TranscriptLine{{Start: 10, End: 12, Data: []byte("a")}, {Start: 13, End: 17, Data: []byte("bb")}},
			17,
		},
		{"trailing blank lines still move the end", "a\n\n\n", 0, []TranscriptLine{{Start: 0, End: 2, Data: []byte("a")}}, 4},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			lines, end := TranscriptLines([]byte(c.data), c.base)
			if !reflect.DeepEqual(lines, c.lines) || end != c.end {
				t.Fatalf("TranscriptLines = %+v, %d; want %+v, %d", lines, end, c.lines, c.end)
			}
		})
	}
}

func TestMessageLogFinishUpdatesTheCallItAnswersInPlace(t *testing.T) {
	at := time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)
	var log MessageLog
	log.Add(Message{ID: "u1", Cursor: 10, Turn: "p1", Role: RoleUser, Text: "run the tests"})
	log.Add(Message{ID: "call_1", Cursor: 20, Turn: "p1", Role: RoleTool, At: at, Tool: &MessageTool{Name: "Bash", Summary: "Bash go test ./...", Status: ToolRunning}})
	log.Add(Message{ID: "a1", Cursor: 30, Turn: "p1", Role: RoleAssistant, Text: "waiting"})
	result := ToolResult("call_1", ToolFailed, "FAIL")
	result.Cursor, result.Turn = 40, "p2"
	log.Finish(result)
	want := []Message{
		{ID: "u1", Cursor: 10, Turn: "p1", Role: RoleUser, Text: "run the tests"},
		{ID: "call_1", Cursor: 20, Turn: "p1", Role: RoleTool, Text: "FAIL", At: at, Tool: &MessageTool{Name: "Bash", Summary: "Bash go test ./...", Status: ToolFailed}},
		{ID: "a1", Cursor: 30, Turn: "p1", Role: RoleAssistant, Text: "waiting"},
	}
	if got := log.Drain(); !reflect.DeepEqual(got, want) {
		t.Fatalf("messages = %+v\nwant %+v", got, want)
	}
}

func TestMessageLogFinishDoesNotChangeAToolMessageAlreadyHandedOut(t *testing.T) {
	var log MessageLog
	log.Add(Message{ID: "call_1", Role: RoleTool, Tool: &MessageTool{Name: "Bash", Status: ToolRunning}})
	before := log.Drain()[0]
	log.Finish(ToolResult("call_1", ToolDone, "ok"))
	if before.Tool.Status != ToolRunning {
		t.Fatalf("earlier copy changed to %q", before.Tool.Status)
	}
}

func TestMessageLogFinishWithoutItsCallKeepsTheResultAsItsOwnMessage(t *testing.T) {
	var log MessageLog
	log.Add(Message{ID: "a1", Role: RoleAssistant, Text: "hi"})
	result := ToolResult("call_9", ToolDone, "ok")
	result.Cursor = 50
	log.Finish(result)
	want := []Message{
		{ID: "a1", Role: RoleAssistant, Text: "hi"},
		{ID: "call_9", Cursor: 50, Role: RoleTool, Text: "ok", Tool: &MessageTool{Status: ToolDone}},
	}
	if got := log.Drain(); !reflect.DeepEqual(got, want) {
		t.Fatalf("messages = %+v\nwant %+v", got, want)
	}
}

func TestMessageLogTracksOnlyRunningToolMessagesByID(t *testing.T) {
	var log MessageLog
	log.Add(Message{ID: "x", Role: RoleAssistant, Text: "same id as a call"})
	log.Add(Message{ID: "y", Role: RoleTool, Tool: &MessageTool{Name: "web_search", Status: ToolDone}})
	log.Finish(ToolResult("x", ToolFailed, "late"))
	log.Finish(ToolResult("y", ToolFailed, "late"))
	got := log.Drain()
	if len(got) != 4 || got[0].Tool != nil || got[1].Tool.Status != ToolDone || got[2].Tool.Name != "" || got[3].Tool.Name != "" {
		t.Fatalf("messages = %+v", got)
	}
}

func TestMessageLogFinishesACallFromAnEarlierDrainAsAnUpdatedCopy(t *testing.T) {
	var log MessageLog
	log.Add(Message{ID: "call_1", Cursor: 20, Turn: "p1", Role: RoleTool, Tool: &MessageTool{Name: "Bash", Summary: "Bash ls", Status: ToolRunning}})
	first := log.Drain()
	log.Add(Message{ID: "a1", Cursor: 30, Role: RoleAssistant, Text: "still here"})
	log.Finish(ToolResult("call_1", ToolDone, "main.go"))
	log.Finish(ToolResult("call_1", ToolFailed, "twice"))
	want := []Message{
		{ID: "a1", Cursor: 30, Role: RoleAssistant, Text: "still here"},
		{ID: "call_1", Cursor: 20, Turn: "p1", Role: RoleTool, Text: "main.go", Tool: &MessageTool{Name: "Bash", Summary: "Bash ls", Status: ToolDone}},
		{ID: "call_1", Role: RoleTool, Text: "twice", Tool: &MessageTool{Status: ToolFailed}},
	}
	if got := log.Drain(); !reflect.DeepEqual(got, want) {
		t.Fatalf("messages = %+v\nwant %+v", got, want)
	}
	if first[0].Tool.Status != ToolRunning {
		t.Fatalf("first drain changed to %q", first[0].Tool.Status)
	}
	if got := log.Drain(); got != nil {
		t.Fatalf("third drain = %+v", got)
	}
}

func TestToolResultClipsLongOutput(t *testing.T) {
	got := ToolResult("c", ToolDone, strings.Repeat("x", ToolTextRunes+10)).Text
	if n := len([]rune(got)); n != ToolTextRunes {
		t.Fatalf("clipped to %d runes, want %d", n, ToolTextRunes)
	}
}

func TestToolSummaryIsOneShortLine(t *testing.T) {
	cases := []struct {
		name, tool, arg, want string
	}{
		{"name and argument", "Edit", "internal/config/load.go", "Edit internal/config/load.go"},
		{"no argument", "TodoWrite", "", "TodoWrite"},
		{"blank argument", "Bash", " \n ", "Bash"},
		{"first non-blank line, spaces collapsed", "Bash", "\n  go   test ./...\ngo vet ./...", "Bash go test ./..."},
		{"clipped", "Bash", "echo " + strings.Repeat("a", 200), "Bash echo " + strings.Repeat("a", ToolSummaryRunes-len("Bash echo ")-1) + "…"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ToolSummary(c.tool, c.arg); got != c.want {
				t.Fatalf("ToolSummary = %q, want %q", got, c.want)
			}
		})
	}
}

func TestClipTextCountsRunesAndTrims(t *testing.T) {
	cases := []struct {
		in   string
		max  int
		want string
	}{
		{"  héllo  ", 5, "héllo"},
		{"héllo world", 6, "héllo…"},
		{"ab cd", 4, "ab…"},
	}
	for _, c := range cases {
		if got := ClipText(c.in, c.max); got != c.want {
			t.Errorf("ClipText(%q, %d) = %q, want %q", c.in, c.max, got, c.want)
		}
	}
}
