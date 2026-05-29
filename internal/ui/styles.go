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
