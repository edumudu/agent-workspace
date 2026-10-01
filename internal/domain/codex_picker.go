package domain

import (
	"fmt"
	"regexp"
	"strings"
)

type Picker struct {
	Title string
	Rows  []PickerRow
}

type PickerRow struct {
	Label    string
	Selected bool
}

var (
	pickerRow      = regexp.MustCompile(`^\s*(›)?\s*\d+\.\s+(.+?)\s*$`)
	pickerColumns  = regexp.MustCompile(`\s{2,}`)
	pickerTag      = regexp.MustCompile(`\s*\([^)]*\)$`)
	pickerAllModel = "all models"
)

// why: only numbered rows under a title starting with "Select " count, so a
// numbered list in the transcript is not taken for a picker. The capture
// includes scrollback, so a picker with Codex's `›` input prompt below it has
// already closed.
func ParsePicker(screen string) (Picker, bool) {
	lines := strings.Split(screen, "\n")
	title := -1
	for i, l := range lines {
		if strings.HasPrefix(strings.TrimSpace(l), "Select ") {
			title = i
		}
	}
	if title < 0 {
		return Picker{}, false
	}
	p := Picker{Title: strings.TrimSpace(lines[title])}
	for _, l := range lines[title+1:] {
		m := pickerRow.FindStringSubmatch(l)
		if m == nil && strings.HasPrefix(strings.TrimSpace(l), "›") {
			return Picker{}, false
		}
		if m == nil {
			continue
		}
		label := pickerColumns.Split(m[2], 2)[0]
		p.Rows = append(p.Rows, PickerRow{Label: label, Selected: m[1] != ""})
	}
	if len(p.Rows) == 0 {
		return Picker{}, false
	}
	return p, true
}

// why: Codex asks for a model first and then its reasoning level, so an
// effort switch re-picks the current model and a model switch keeps the
// current effort, or the highlighted one when the new model does not offer it.
func CodexPickerKeys(p Picker, sw Switch, model, effort string) ([]string, error) {
	if strings.HasPrefix(p.Title, "Select Reasoning Level") {
		if sw.Kind == SwitchEffort {
			return keysTo(p, sw.Value)
		}
		if keys, err := keysTo(p, effort); err == nil {
			return keys, nil
		}
		return []string{"Enter"}, nil
	}
	want := sw.Value
	if sw.Kind == SwitchEffort {
		want = model
	}
	if keys, err := keysTo(p, want); err == nil {
		return keys, nil
	}
	return keysTo(p, pickerAllModel)
}

func keysTo(p Picker, want string) ([]string, error) {
	from, to := 0, -1
	for i, r := range p.Rows {
		if r.Selected {
			from = i
		}
		if to < 0 && want != "" && pickerLabel(r.Label) == pickerLabel(want) {
			to = i
		}
	}
	if to < 0 {
		return nil, fmt.Errorf("%q is not in the Codex picker %q", want, p.Title)
	}
	var keys []string
	for ; from < to; from++ {
		keys = append(keys, "Down")
	}
	for ; from > to; from-- {
		keys = append(keys, "Up")
	}
	return append(keys, "Enter"), nil
}

// bug: Codex shows display names and tags such as "(current)": "GPT-6-Luna"
// is gpt-6-luna, "Extra high" is xhigh.
func pickerLabel(s string) string {
	s = strings.ToLower(pickerTag.ReplaceAllString(strings.TrimSpace(s), ""))
	if s == "extra high" {
		return "xhigh"
	}
	return s
}
