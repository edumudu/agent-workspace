package tui

import (
	"errors"
	"fmt"
	"io/fs"

	"github.com/BurntSushi/toml"
)

// Theme is the palette as hex colors. Field names follow Catppuccin's.
type Theme struct {
	Text     string `toml:"text"`
	Subtext  string `toml:"subtext"`
	Overlay  string `toml:"overlay"`
	Surface  string `toml:"surface"`
	Mantle   string `toml:"mantle"`
	Base     string `toml:"base"`
	Blue     string `toml:"blue"`
	Peach    string `toml:"peach"`
	Green    string `toml:"green"`
	Red      string `toml:"red"`
	Teal     string `toml:"teal"`
	Mauve    string `toml:"mauve"`
	Selected string `toml:"selected"`
}

// Latte is Catppuccin Latte, the default.
func Latte() Theme {
	return Theme{
		Text:     "#4c4f69",
		Subtext:  "#6c6f85",
		Overlay:  "#9ca0b0",
		Surface:  "#ccd0da",
		Mantle:   "#e6e9ef",
		Base:     "#eff1f5",
		Blue:     "#1e66f5",
		Peach:    "#fe640b",
		Green:    "#40a02b",
		Red:      "#d20f39",
		Teal:     "#179299",
		Mauve:    "#8839ef",
		Selected: "#dce0e8",
	}
}

// LoadTheme reads the [theme] table of a config.toml over Latte. A missing
// file gives Latte.
func LoadTheme(path string) (Theme, error) {
	cfg := struct {
		Theme Theme `toml:"theme"`
	}{Theme: Latte()}
	_, err := toml.DecodeFile(path, &cfg)
	if errors.Is(err, fs.ErrNotExist) {
		return Latte(), nil
	}
	if err != nil {
		return Theme{}, fmt.Errorf("tui: %s: %w", path, err)
	}
	return cfg.Theme, nil
}
