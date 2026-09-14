// Package theme maps HyDE Wallbash colors to a shared TUI palette.
package theme

import (
	"bufio"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const refreshInterval = 2 * time.Second

type Palette struct {
	Base, Mantle, Crust                    lipgloss.Color
	Surface0, Surface1, Surface2           lipgloss.Color
	Overlay0, Overlay1, Overlay2           lipgloss.Color
	Subtext0, Subtext1, Text               lipgloss.Color
	Lavender, Blue, Sapphire, Sky, Teal    lipgloss.Color
	Green, Yellow, Peach, Red, Mauve, Pink lipgloss.Color
	Rosewater, OnAccent                    lipgloss.Color
}

type ChangedMsg struct{ Palette Palette }

func Watch() tea.Cmd {
	return tea.Tick(refreshInterval, func(time.Time) tea.Msg { return ChangedMsg{Palette: Current()} })
}

func Current() Palette {
	fallback := CatppuccinMocha()
	if strings.EqualFold(strings.TrimSpace(os.Getenv("TUI_THEME")), "catppuccin") {
		return fallback
	}
	values, ok := readColors(themePath())
	if !ok || values["wallbash_pry1"] == "" || values["wallbash_txt1"] == "" {
		return fallback
	}
	p := Palette{
		Base: color(values, "wallbash_pry1", fallback.Base), Mantle: color(values, "wallbash_pry2", fallback.Mantle), Crust: color(values, "wallbash_1xa1", fallback.Crust),
		Surface0: color(values, "wallbash_1xa2", fallback.Surface0), Surface1: color(values, "wallbash_1xa3", fallback.Surface1), Surface2: color(values, "wallbash_1xa4", fallback.Surface2),
		Overlay0: color(values, "wallbash_1xa5", fallback.Overlay0), Overlay1: color(values, "wallbash_1xa6", fallback.Overlay1), Overlay2: color(values, "wallbash_1xa7", fallback.Overlay2),
		Subtext0: color(values, "wallbash_1xa8", fallback.Subtext0), Subtext1: color(values, "wallbash_1xa9", fallback.Subtext1), Text: color(values, "wallbash_txt1", fallback.Text),
		Lavender: color(values, "wallbash_3xa8", fallback.Lavender), Blue: color(values, "wallbash_3xa7", fallback.Blue), Sapphire: color(values, "wallbash_3xa8", fallback.Sapphire),
		Sky: color(values, "wallbash_2xa8", fallback.Sky), Teal: color(values, "wallbash_2xa7", fallback.Teal), Green: color(values, "wallbash_2xa9", fallback.Green),
		Yellow: color(values, "wallbash_1xa8", fallback.Yellow), Peach: color(values, "wallbash_1xa9", fallback.Peach), Red: color(values, "wallbash_4xa8", fallback.Red),
		Mauve: color(values, "wallbash_3xa8", fallback.Mauve), Pink: color(values, "wallbash_2xa8", fallback.Pink), Rosewater: color(values, "wallbash_4xa9", fallback.Rosewater),
	}
	p.OnAccent = contrasting(string(p.Mauve))
	return p
}

func CatppuccinMocha() Palette {
	return Palette{
		Base: "#1e1e2e", Mantle: "#181825", Crust: "#11111b", Surface0: "#313244", Surface1: "#45475a", Surface2: "#585b70",
		Overlay0: "#6c7086", Overlay1: "#7f849c", Overlay2: "#9399b2", Subtext0: "#a6adc8", Subtext1: "#bac2de", Text: "#cdd6f4",
		Lavender: "#b4befe", Blue: "#89b4fa", Sapphire: "#74c7ec", Sky: "#89dceb", Teal: "#94e2d5", Green: "#a6e3a1",
		Yellow: "#f9e2af", Peach: "#fab387", Red: "#f38ba8", Mauve: "#cba6f7", Pink: "#f5c2e7", Rosewater: "#f5e0dc", OnAccent: "#1e1e2e",
	}
}

func themePath() string {
	if path := strings.TrimSpace(os.Getenv("TUI_THEME_FILE")); path != "" {
		return path
	}
	cache := strings.TrimSpace(os.Getenv("XDG_CACHE_HOME"))
	if cache == "" {
		if home, err := os.UserHomeDir(); err == nil {
			cache = filepath.Join(home, ".cache")
		}
	}
	return filepath.Join(cache, "hyde", "wallbash", "shell-colors")
}

func readColors(path string) (map[string]string, bool) {
	file, err := os.Open(path)
	if err != nil {
		return nil, false
	}
	defer file.Close()
	values := make(map[string]string)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, raw, found := strings.Cut(line, "=")
		if !found || strings.HasSuffix(key, "_rgba") {
			continue
		}
		fields := strings.Fields(raw)
		if len(fields) == 0 {
			continue
		}
		raw = strings.Trim(fields[0], "'\"")
		if normalized, valid := normalizeHex(raw); valid {
			values[strings.TrimSpace(key)] = normalized
		}
	}
	return values, scanner.Err() == nil
}

func normalizeHex(value string) (string, bool) {
	value = strings.TrimPrefix(strings.TrimSpace(value), "#")
	if len(value) != 6 {
		return "", false
	}
	if _, err := hex.DecodeString(value); err != nil {
		return "", false
	}
	return "#" + strings.ToUpper(value), true
}
func color(values map[string]string, key string, fallback lipgloss.Color) lipgloss.Color {
	if value := values[key]; value != "" {
		return lipgloss.Color(value)
	}
	return fallback
}
func contrasting(value string) lipgloss.Color {
	bytes, err := hex.DecodeString(strings.TrimPrefix(value, "#"))
	if err != nil || len(bytes) != 3 {
		return "#111111"
	}
	if (299*int(bytes[0])+587*int(bytes[1])+114*int(bytes[2]))/1000 >= 150 {
		return "#111111"
	}
	return "#ffffff"
}
