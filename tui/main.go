package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/danbiagini/freedb-tui/internal/check"
	"github.com/danbiagini/freedb-tui/internal/config"
	"github.com/danbiagini/freedb-tui/internal/incus"
	"github.com/danbiagini/freedb-tui/internal/registry"
	"github.com/danbiagini/freedb-tui/internal/tui"
)

var version = "dev"

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "--version", "-v":
			fmt.Printf("freedb %s\n", version)
			os.Exit(0)
		case "--check", "check":
			cfg, err := config.Load()
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
				os.Exit(1)
			}
			results := check.RunChecks(cfg)
			check.PrintResults(results)
			for _, r := range results {
				if !r.OK {
					os.Exit(1)
				}
			}
			os.Exit(0)
		case "--help", "-h", "help":
			fmt.Println("freedb — FreeDB app manager")
			fmt.Println()
			fmt.Println("Usage:")
			fmt.Println("  sudo freedb              Launch the TUI dashboard")
			fmt.Println("  sudo freedb check        Run health checks")
			fmt.Println("  freedb --version         Print version")
			fmt.Println("  freedb --help            Show this help")
			os.Exit(0)
		}
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}
	cfg.Version = version

	ic, err := incus.Connect(cfg.IncusSocket)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error connecting to incus: %v\n", err)
		fmt.Fprintf(os.Stderr, "Make sure incus is running and you have permission to access the socket.\n")
		fmt.Fprintf(os.Stderr, "Try: sudo freedb\n")
		os.Exit(1)
	}

	reg, err := registry.Load(cfg.RegistryPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading registry: %v\n", err)
		os.Exit(1)
	}

	m := tui.NewModel(cfg, ic, reg)

	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running TUI: %v\n", err)
		os.Exit(1)
	}
}
