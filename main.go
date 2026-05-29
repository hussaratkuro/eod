package main

import (
	"fmt"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"

	"eod/internal/storage"
	"eod/internal/ui"
)

const usage = `eod – End-of-Day log manager

Usage:
  eod                       Open the current month's log
  eod <directory>           Open from a specific directory
  eod import <file|dir>     Import existing txt files

Data directory (default): $XDG_DATA_HOME/eod  (~/.local/share/eod)
Override with: EOD_DATA_DIR environment variable
`

func main() {
	dataDir := storage.DefaultDir()
	if d := os.Getenv("EOD_DATA_DIR"); d != "" {
		dataDir = d
	}

	args := os.Args[1:]

	// Handle subcommands first
	if len(args) >= 1 {
		switch args[0] {
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
		}
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

	app := ui.New(store, file, allFiles)

	p := tea.NewProgram(
		app,
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
