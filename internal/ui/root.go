package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Mode is the top-level screen: the launcher menu, or one of the two apps.
type Mode int

const (
	ModeLauncher Mode = iota
	ModeEOD
	ModeTodo
)

// backToLauncherMsg is how a child app asks to return to the mode picker.
type backToLauncherMsg struct{}

func toLauncher() tea.Msg { return backToLauncherMsg{} }

type menuEntry struct {
	mode  Mode
	key   string
	title string
	desc  string
	icon  string
	color string
}

var menu = []menuEntry{
	{ModeTodo, "1", "TODO", "sticky notes, checklists, due dates", "▤", "mauve"},
	{ModeEOD, "2", "EOD", "end-of-day log, monthly export", "▦", "yellow"},
}

// Root owns both apps and the launcher that switches between them.
type Root struct {
	mode          Mode
	sel           int
	width, height int

	eod  *App
	todo *TodoApp
}

func NewRoot(eod *App, todo *TodoApp, start Mode) *Root {
	// Cross-wiring: the EOD editor can pull open todos, and the todo board can
	// push what got done today into today's EOD entry.
	eod.todoText = todo.OpenTasksText
	todo.eodAppend = eod.appendToToday

	r := &Root{mode: start, eod: eod, todo: todo}
	if start == ModeTodo {
		todo.Enter()
	}
	return r
}

func (r *Root) Init() tea.Cmd { return nil }

func (r *Root) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {
	case tea.WindowSizeMsg:
		r.width, r.height = m.Width, m.Height
		r.eod.Update(msg)
		r.todo.Update(msg)
		return r, nil

	case backToLauncherMsg:
		r.mode = ModeLauncher
		return r, nil

	case todoTickMsg:
		// Focus-timer ticks keep flowing even while the EOD side is on screen,
		// so the clock is still right when the user comes back.
		_, cmd := r.todo.Update(msg)
		return r, cmd

	case tea.KeyMsg:
		if r.mode == ModeLauncher {
			return r.updateLauncher(m)
		}
	}

	switch r.mode {
	case ModeTodo:
		_, cmd := r.todo.Update(msg)
		return r, cmd
	case ModeEOD:
		_, cmd := r.eod.Update(msg)
		return r, cmd
	}
	return r, nil
}

func (r *Root) updateLauncher(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c", "esc":
		return r, tea.Quit

	case "up", "k", "left", "h":
		if r.sel > 0 {
			r.sel--
		}
	case "down", "j", "right", "l", "tab":
		if r.sel < len(menu)-1 {
			r.sel++
		}
	case "home", "g":
		r.sel = 0
	case "end", "G":
		r.sel = len(menu) - 1

	case "enter", " ":
		r.start(menu[r.sel].mode)

	case "1":
		r.sel = 0
		r.start(menu[0].mode)
	case "2":
		r.sel = 1
		r.start(menu[1].mode)
	}
	return r, nil
}

func (r *Root) start(m Mode) {
	r.mode = m
	if m == ModeTodo {
		r.todo.Enter()
	}
}

func (r *Root) View() string {
	switch r.mode {
	case ModeTodo:
		return r.todo.View()
	case ModeEOD:
		return r.eod.View()
	}
	return r.viewLauncher()
}

func (r *Root) viewLauncher() string {
	if r.width == 0 {
		return "Loading…"
	}

	var rows []string
	rows = append(rows, styleLogo.Render("eod"), styleMuted.Render("workday companion"), "")

	for i, e := range menu {
		line := e.icon + "  " + e.title
		if i == r.sel {
			rows = append(rows, styleMenuSelected.Render(pad(line, 34)))
		} else {
			rows = append(rows,
				styleMenuItem.Foreground(accentColor(e.color)).Render(pad(line, 34)))
		}
		rows = append(rows, styleMenuItem.Render(styleMuted.Render(pad(e.desc, 34))))
		if i < len(menu)-1 {
			rows = append(rows, "")
		}
	}

	rows = append(rows, "", styleMuted.Render("  [↑/↓] select   [Enter] open   [1/2] jump   [q] quit"))

	box := styleMenuBox.Render(strings.Join(rows, "\n"))
	return lipgloss.Place(r.width, r.height, lipgloss.Center, lipgloss.Center, box)
}
