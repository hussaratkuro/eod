package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestRootCommandPaletteFiltersLauncherActions(t *testing.T) {
	root := &Root{mode: ModeLauncher, width: 100, height: 30, eod: &App{}, todo: &TodoApp{}}
	root.openCommandPalette()
	root.commands.query = "eod jr"
	items := root.filteredCommands()
	if len(items) != 1 || items[0].kind != rootCommandEOD {
		t.Fatalf("launcher command match = %#v", items)
	}
	updated, _ := root.updateCommandPalette(tea.KeyMsg{Type: tea.KeyEsc})
	if updated.(*Root).commands.open {
		t.Fatal("Esc did not close command palette")
	}
}
