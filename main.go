package main

import (
	"fmt"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"

	"eod/internal/storage"
	"eod/internal/todo"
	"eod/internal/ui"
)

const usage = `eod – workday companion: end-of-day log + todo board

Usage:
  eod                       Open the launcher (TODO / EOD)
  eod todo                  Go straight to the todo board
  eod eod                   Go straight to the EOD log
  eod <directory>           Open from a specific directory
  eod import <file|dir>     Import existing txt files

Data directory (default): $XDG_DATA_HOME/eod  (~/.local/share/eod)
  EOD logs   <data>/eod_YYYY_MM.txt
  Todo notes <data>/todo/<note>.md
Override with: EOD_DATA_DIR environment variable
`

func main() {
	dataDir := storage.DefaultDir()
	if d := os.Getenv("EOD_DATA_DIR"); d != "" {
		dataDir = d
	}

	args := os.Args[1:]
	mode := ui.ModeLauncher

	// Handle subcommands first
	if len(args) >= 1 {
		switch args[0] {
		case "todo":
			mode = ui.ModeTodo
			args = args[1:]

		case "eod":
			mode = ui.ModeEOD
			args = args[1:]

		case "import":
			if len(args) < 2 {
				fmt.Fprintln(os.Stderr, "Usage: eod import <file|directory>")
				os.Exit(1)
			}
			runImport(dataDir, args[1])
			return

		case "help", "-h", "--help":
			fmt.Print(usage)
			return

		default:
			// Treat as data directory override
			dataDir = args[0]
			args = nil
		}
	}

	// A directory may still follow "todo" / "eod".
	if len(args) >= 1 {
		dataDir = args[0]
	}

	store, err := storage.New(dataDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: cannot open data directory: %v\n", err)
		os.Exit(1)
	}

	file, err := store.CurrentFile()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: cannot load file: %v\n", err)
		os.Exit(1)
	}

	// Load all available months
	allFiles, _ := store.LoadAll()

	eodApp := ui.New(store, file, allFiles)

	todoStore, err := todo.NewStore(dataDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: cannot open todo directory: %v\n", err)
		os.Exit(1)
	}
	todoApp, err := ui.NewTodoApp(todoStore)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: cannot load todo notes: %v\n", err)
		os.Exit(1)
	}

	root := ui.NewRoot(eodApp, todoApp, mode)

	p := tea.NewProgram(
		root,
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func runImport(dataDir, src string) {
	store, err := storage.New(dataDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	info, err := os.Stat(src)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if info.IsDir() {
		imported, errs := store.ImportDir(src)
		for _, e := range errs {
			fmt.Fprintf(os.Stderr, "warning: %v\n", e)
		}
		fmt.Printf("Imported %d file(s) → %s\n", len(imported), dataDir)
		for _, f := range imported {
			fmt.Printf("  ✓ %s\n", filepath.Base(f.Path))
		}
	} else {
		f, err := store.Import(src)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Imported: %s → %s\n", src, f.Path)
	}
}
