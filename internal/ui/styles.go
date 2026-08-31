package ui

import "github.com/charmbracelet/lipgloss"

// Catppuccin Mocha palette
const (
	moBase     = "#1e1e2e"
	moMantle   = "#181825"
	moSurface0 = "#313244"
	moSurface1 = "#45475a"
	moSurface2 = "#585b70"
	moOverlay0 = "#6c7086"
	moOverlay1 = "#7f849c"
	moOverlay2 = "#9399b2"
	moSubtext0 = "#a6adc8"
	moSubtext1 = "#bac2de"
	moText     = "#cdd6f4"
	moLavender = "#b4befe"
	moBlue     = "#89b4fa"
	moSapphire = "#74c7ec"
	moSky      = "#89dceb"
	moTeal     = "#94e2d5"
	moGreen    = "#a6e3a1"
	moYellow   = "#f9e2af"
	moPeach    = "#fab387"
	moMaroon   = "#eba0ac"
	moRed      = "#f38ba8"
	moMauve    = "#cba6f7"
	moPink     = "#f5c2e7"
)

var (
	styleHeader = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color(moYellow)).
			Padding(0, 1)

	stylePanelTitle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color(moMauve)).
			Padding(0, 1)

	styleBorder = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color(moSurface1))

	styleSelected = lipgloss.NewStyle().
			Background(lipgloss.Color(moSurface1)).
			Foreground(lipgloss.Color(moLavender)).
			Bold(true).
			Padding(0, 1)

	styleNormal = lipgloss.NewStyle().
			Foreground(lipgloss.Color(moText)).
			Padding(0, 1)

	styleMuted = lipgloss.NewStyle().
			Foreground(lipgloss.Color(moOverlay1))

	styleCategory = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color(moGreen))

	styleCategoryIcon = "▶ "
	styleItemIcon     = "· "

	styleStatusBar = lipgloss.NewStyle().
			Foreground(lipgloss.Color(moOverlay1)).
			Padding(0, 1)

	styleKey = lipgloss.NewStyle().
			Foreground(lipgloss.Color(moMauve)).
			Bold(true)

	styleError = lipgloss.NewStyle().
			Foreground(lipgloss.Color(moRed)).
			Bold(true)

	styleSuccess = lipgloss.NewStyle().
			Foreground(lipgloss.Color(moGreen))

	styleWarn = lipgloss.NewStyle().
			Foreground(lipgloss.Color(moPeach)).
			Bold(true)

	styleEditorTitle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color(moYellow)).
				MarginBottom(1)

	styleHelp = lipgloss.NewStyle().
			Foreground(lipgloss.Color(moOverlay1)).
			Italic(true)

	styleVacation = lipgloss.NewStyle().
			Foreground(lipgloss.Color(moPeach)).
			Bold(true)

	styleVacationRow = lipgloss.NewStyle().
				Foreground(lipgloss.Color(moPeach)).
				Padding(0, 1)
)

func keyHint(key, desc string) string {
	return styleKey.Render(key) + styleStatusBar.Render(":"+desc)
}

// ── accents ────────────────────────────────────────────────────────────────
// Notes pick an accent from this list; [c] cycles through it in order.

var accentNames = []string{
	"lavender", "blue", "sapphire", "sky", "teal",
	"green", "yellow", "peach", "maroon", "red", "mauve", "pink",
}

var accentHex = map[string]string{
	"lavender": moLavender,
	"blue":     moBlue,
	"sapphire": moSapphire,
	"sky":      moSky,
	"teal":     moTeal,
	"green":    moGreen,
	"yellow":   moYellow,
	"peach":    moPeach,
	"maroon":   moMaroon,
	"red":      moRed,
	"mauve":    moMauve,
	"pink":     moPink,
}

func accentColor(name string) lipgloss.Color {
	if hex, ok := accentHex[name]; ok {
		return lipgloss.Color(hex)
	}
	return lipgloss.Color(moLavender)
}

// nextAccent returns the accent after name, wrapping around.
func nextAccent(name string) string {
	for i, n := range accentNames {
		if n == name {
			return accentNames[(i+1)%len(accentNames)]
		}
	}
	return accentNames[0]
}

// accentFor picks a starting colour for a new note so a fresh board is varied.
func accentFor(i int) string {
	return accentNames[i%len(accentNames)]
}

var (
	styleWall = lipgloss.NewStyle().
			Foreground(lipgloss.Color(moText))

	styleCardTitle = lipgloss.NewStyle().Bold(true)

	styleDone = lipgloss.NewStyle().
			Foreground(lipgloss.Color(moOverlay0)).
			Strikethrough(true)

	styleTag = lipgloss.NewStyle().
			Foreground(lipgloss.Color(moMauve))

	styleDueOverdue = lipgloss.NewStyle().
			Foreground(lipgloss.Color(moRed)).
			Bold(true)

	styleDueToday = lipgloss.NewStyle().
			Foreground(lipgloss.Color(moPeach)).
			Bold(true)

	styleDueSoon = lipgloss.NewStyle().
			Foreground(lipgloss.Color(moYellow))

	styleDueLater = lipgloss.NewStyle().
			Foreground(lipgloss.Color(moOverlay1))

	stylePrio = map[int]lipgloss.Style{
		1: lipgloss.NewStyle().Foreground(lipgloss.Color(moRed)).Bold(true),
		2: lipgloss.NewStyle().Foreground(lipgloss.Color(moPeach)),
		3: lipgloss.NewStyle().Foreground(lipgloss.Color(moYellow)),
	}

	styleRepeat = lipgloss.NewStyle().
			Foreground(lipgloss.Color(moSapphire))

	styleFocus = lipgloss.NewStyle().
			Foreground(lipgloss.Color(moTeal)).
			Bold(true)

	styleCursor = lipgloss.NewStyle().
			Foreground(lipgloss.Color(moLavender)).
			Bold(true)

	styleProgress = lipgloss.NewStyle().
			Foreground(lipgloss.Color(moGreen))

	styleProgressBG = lipgloss.NewStyle().
			Foreground(lipgloss.Color(moSurface1))

	styleLogo = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color(moMauve))

	styleMenuItem = lipgloss.NewStyle().
			Foreground(lipgloss.Color(moText)).
			Padding(0, 2)

	styleMenuSelected = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color(moBase)).
				Background(lipgloss.Color(moLavender)).
				Padding(0, 2)

	styleMenuBox = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color(moSurface2)).
			Padding(1, 4)
)
