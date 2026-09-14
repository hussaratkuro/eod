package ui

import (
	"strings"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type rootCommandKind uint8

const (
	rootCommandTodo rootCommandKind = iota
	rootCommandEOD
	rootCommandLauncher
	rootCommandChildKey
	rootCommandQuit
)

type rootCommandItem struct {
	kind  rootCommandKind
	label string
	hint  string
	key   tea.KeyMsg
}

type rootCommandPalette struct {
	open   bool
	query  string
	cursor int
	items  []rootCommandItem
}

func (r *Root) commandPaletteAvailable() bool {
	switch r.mode {
	case ModeLauncher:
		return true
	case ModeEOD:
		return r.eod.view == viewMain || r.eod.view == viewSummary || r.eod.view == viewHelp
	case ModeTodo:
		return r.todo.view == tvWall || r.todo.view == tvNote || r.todo.view == tvSummary || r.todo.view == tvHelp
	}
	return false
}

func (r *Root) openCommandPalette() {
	items := []rootCommandItem{
		{kind: rootCommandTodo, label: "Open Todo board", hint: "Todo"},
		{kind: rootCommandEOD, label: "Open EOD journal", hint: "EOD"},
		{kind: rootCommandLauncher, label: "Return to app launcher", hint: "Esc"},
		{kind: rootCommandQuit, label: "Quit eod", hint: "Ctrl+C"},
	}
	if r.mode == ModeEOD && r.eod.view == viewMain {
		items = append([]rootCommandItem{
			childCommand("New day", "n", 'n'),
			childCommand("Edit selected day", "e", 'e'),
			childCommand("Search this month", "/", '/'),
			childCommand("Open monthly summary", "s", 's'),
			childCommand("Export month as Markdown and CSV", "x", 'x'),
			childCommand("Copy selected day", "y", 'y'),
			childCommand("Open EOD help", "?", '?'),
		}, items...)
	}
	if r.mode == ModeTodo && (r.todo.view == tvWall || r.todo.view == tvNote) {
		contextItems := []rootCommandItem{
			childCommand("Add task", "a", 'a'),
			childCommand("Search every task", "/", '/'),
			childCommand("Show Todo statistics", "s", 's'),
			childCommand("Undo last Todo change", "u", 'u'),
			childCommand("Push today's finished tasks to EOD", "Ctrl+E", 0),
			childCommand("Open Todo help", "?", '?'),
		}
		contextItems[4].key = tea.KeyMsg{Type: tea.KeyCtrlE}
		if r.todo.view == tvNote {
			contextItems = append([]rootCommandItem{
				childCommand("Toggle selected task", "Space", ' '),
				childCommand("Edit selected task", "e", 'e'),
				childCommand("Move selected task", "m", 'm'),
				childCommand("Toggle focus timer", "f", 'f'),
			}, contextItems...)
		} else {
			contextItems = append([]rootCommandItem{
				childCommand("Create Todo note", "n", 'n'),
				childCommand("Open selected Todo note", "Enter", 0),
			}, contextItems...)
			contextItems[1].key = tea.KeyMsg{Type: tea.KeyEnter}
		}
		items = append(contextItems, items...)
	}
	r.commands = rootCommandPalette{open: true, items: items}
}

func childCommand(label, hint string, character rune) rootCommandItem {
	key := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{character}}
	return rootCommandItem{kind: rootCommandChildKey, label: label, hint: hint, key: key}
}

func (r *Root) filteredCommands() []rootCommandItem {
	var result []rootCommandItem
	for _, item := range r.commands.items {
		if rootFuzzyMatch(item.label+" "+item.hint, r.commands.query) {
			result = append(result, item)
		}
	}
	return result
}

func rootFuzzyMatch(label, query string) bool {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return true
	}
	label = strings.ToLower(label)
	position := 0
	for _, character := range query {
		found := strings.IndexRune(label[position:], character)
		if found < 0 {
			return false
		}
		position += found + 1
	}
	return true
}

func (r *Root) updateCommandPalette(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	items := r.filteredCommands()
	switch key.String() {
	case "esc", "ctrl+p", "ctrl+shift+p":
		r.commands = rootCommandPalette{}
	case "ctrl+c":
		return r, tea.Quit
	case "up", "ctrl+k":
		r.commands.cursor = max(0, r.commands.cursor-1)
	case "down", "ctrl+j", "tab":
		r.commands.cursor = min(max(0, len(items)-1), r.commands.cursor+1)
	case "backspace":
		query := []rune(r.commands.query)
		if len(query) > 0 {
			r.commands.query = string(query[:len(query)-1])
			r.commands.cursor = 0
		}
	case "enter":
		if len(items) == 0 {
			return r, nil
		}
		item := items[min(r.commands.cursor, len(items)-1)]
		r.commands = rootCommandPalette{}
		switch item.kind {
		case rootCommandTodo:
			r.start(ModeTodo)
		case rootCommandEOD:
			r.start(ModeEOD)
		case rootCommandLauncher:
			r.mode = ModeLauncher
		case rootCommandChildKey:
			if r.mode == ModeTodo {
				_, cmd := r.todo.Update(item.key)
				return r, cmd
			}
			if r.mode == ModeEOD {
				_, cmd := r.eod.Update(item.key)
				return r, cmd
			}
		case rootCommandQuit:
			return r, tea.Quit
		}
	default:
		if key.Type == tea.KeyRunes && !key.Alt {
			for _, character := range key.Runes {
				if unicode.IsPrint(character) {
					r.commands.query += string(character)
				}
			}
			r.commands.cursor = 0
		}
	}
	return r, nil
}

func (r *Root) viewCommandPalette() string {
	width := min(max(62, r.width*3/4), max(20, r.width-4))
	items := r.filteredCommands()
	limit := min(len(items), max(3, r.height-10))
	start := max(0, min(r.commands.cursor-limit/2, len(items)-limit))
	rows := []string{styleLogo.Render("Command palette"), styleMuted.Render("Fuzzy-search EOD and Todo actions"), "", "› " + r.commands.query + "_", ""}
	if len(items) == 0 {
		rows = append(rows, styleMuted.Render("No matching commands"))
	}
	for index := start; index < start+limit; index++ {
		item := items[index]
		labelWidth := max(8, width-22)
		label := truncateRootCommand(item.label, labelWidth)
		line := label + strings.Repeat(" ", max(1, labelWidth-lipgloss.Width(label))) + item.hint
		if index == r.commands.cursor {
			line = styleMenuSelected.Width(width - 6).Render(line)
		}
		rows = append(rows, line)
	}
	rows = append(rows, "", styleMuted.Render("Enter runs · ↑/↓ selects · Esc closes"))
	box := styleMenuBox.Width(width).Render(strings.Join(rows, "\n"))
	return lipgloss.Place(r.width, r.height, lipgloss.Center, lipgloss.Center, box)
}

func truncateRootCommand(value string, width int) string {
	if lipgloss.Width(value) <= width {
		return value
	}
	if width <= 1 {
		return "…"
	}
	runes := []rune(value)
	for len(runes) > 0 && lipgloss.Width(string(runes))+1 > width {
		runes = runes[:len(runes)-1]
	}
	return string(runes) + "…"
}
