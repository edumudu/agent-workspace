package domain

import (
	"reflect"
	"testing"
)

const codexModelScreen = `• Ran the tests
  1. flaky output line

  Select Model and Effort
  Access legacy models by running codex -m <model_name> or in your config.toml

› 1. GPT-6.1-Sol (current)  Latest frontier model.
  2. GPT-6-Astra            Strong model for long tasks.
  3. GPT-6-Luna             Fast and cheap.

  Press enter to confirm or esc to go back`

const codexEffortScreen = `  Select Reasoning Level for GPT-6-Astra

  1. Low                 Fast responses
› 2. Medium (default)    Balanced
  3. High                Deeper reasoning
  4. Extra high          Slowest

  Press enter to confirm or esc to go back`

func TestCodexPickerParsesTheLastPickerOnScreen(t *testing.T) {
	cases := []struct {
		name   string
		screen string
		want   Picker
		ok     bool
	}{
		{"model picker", codexModelScreen, Picker{Title: "Select Model and Effort", Rows: []PickerRow{
			{Label: "GPT-6.1-Sol (current)", Selected: true}, {Label: "GPT-6-Astra"}, {Label: "GPT-6-Luna"},
		}}, true},
		{"effort picker", codexEffortScreen, Picker{Title: "Select Reasoning Level for GPT-6-Astra", Rows: []PickerRow{
			{Label: "Low"}, {Label: "Medium (default)", Selected: true}, {Label: "High"}, {Label: "Extra high"},
		}}, true},
		{"numbered list without a picker title", "• Plan\n  1. read\n  2. write\n› ", Picker{}, false},
		{"title with no rows", "  Select Model\n\n› ", Picker{}, false},
		{"a closed picker left in the scrollback, under the prompt", codexEffortScreen + "\n• earlier output\n\n› ", Picker{}, false},
	}
	for _, c := range cases {
		got, ok := ParsePicker(c.screen)
		if ok != c.ok || !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: %+v %v", c.name, got, ok)
		}
	}
}

func TestCodexPickerKeysReachTheTargetRow(t *testing.T) {
	model, _ := ParsePicker(codexModelScreen)
	effort, _ := ParsePicker(codexEffortScreen)
	quick := Picker{Title: "Select Model", Rows: []PickerRow{{Label: "Auto", Selected: true}, {Label: "All models"}}}
	noneSelected := Picker{Title: "Select Model", Rows: []PickerRow{{Label: "a"}, {Label: "gpt-6-luna"}}}
	cases := []struct {
		name          string
		p             Picker
		sw            Switch
		model, effort string
		want          []string
	}{
		{"model below the highlight", model, Switch{Kind: SwitchModel, Value: "gpt-6-luna"}, "gpt-6.1-sol", "", []string{"Down", "Down", "Enter"}},
		{"model already highlighted, ignoring its tag", model, Switch{Kind: SwitchModel, Value: "gpt-6.1-sol"}, "", "", []string{"Enter"}},
		{"effort switch keeps the current model", model, Switch{Kind: SwitchEffort, Value: "high"}, "gpt-6-astra", "", []string{"Down", "Enter"}},
		{"effort above the highlight", effort, Switch{Kind: SwitchEffort, Value: "low"}, "", "", []string{"Up", "Enter"}},
		{"xhigh is shown as Extra high", effort, Switch{Kind: SwitchEffort, Value: "xhigh"}, "", "", []string{"Down", "Down", "Enter"}},
		{"model switch keeps the current effort", effort, Switch{Kind: SwitchModel, Value: "gpt-6-astra"}, "", "high", []string{"Down", "Enter"}},
		{"model switch takes the highlighted effort when the current one is not offered", effort, Switch{Kind: SwitchModel, Value: "gpt-6-astra"}, "", "ultra", []string{"Enter"}},
		{"a quick picker opens the full list", quick, Switch{Kind: SwitchModel, Value: "gpt-6-luna"}, "", "", []string{"Down", "Enter"}},
		{"no highlight counts from the top", noneSelected, Switch{Kind: SwitchModel, Value: "gpt-6-luna"}, "", "", []string{"Down", "Enter"}},
	}
	for _, c := range cases {
		got, err := CodexPickerKeys(c.p, c.sw, c.model, c.effort)
		if err != nil || !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: %q %v", c.name, got, err)
		}
	}
}

func TestCodexPickerKeysRefuseATargetThatIsNotListed(t *testing.T) {
	model, _ := ParsePicker(codexModelScreen)
	effort, _ := ParsePicker(codexEffortScreen)
	cases := []struct {
		name          string
		p             Picker
		sw            Switch
		model, effort string
	}{
		{"unknown model", model, Switch{Kind: SwitchModel, Value: "gpt-9"}, "", ""},
		{"current model missing for an effort switch", model, Switch{Kind: SwitchEffort, Value: "high"}, "gpt-9", ""},
		{"unknown effort", effort, Switch{Kind: SwitchEffort, Value: "ultra"}, "", ""},
	}
	for _, c := range cases {
		if keys, err := CodexPickerKeys(c.p, c.sw, c.model, c.effort); err == nil {
			t.Errorf("%s: keys %q", c.name, keys)
		}
	}
}
