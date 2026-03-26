package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/danbiagini/freedb-tui/internal/check"
	"github.com/danbiagini/freedb-tui/internal/config"
	"github.com/danbiagini/freedb-tui/internal/deploy"
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
		case "deploy":
			os.Exit(runDeploy(os.Args[2:]))
		case "--help", "-h", "help":
			printHelp()
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

func runDeploy(args []string) int {
	opts := deploy.Options{}

	i := 0
	for i < len(args) {
		switch args[i] {
		case "--image":
			if i+1 < len(args) {
				opts.Image = args[i+1]
				i += 2
			} else {
				fmt.Fprintf(os.Stderr, "Error: --image requires a value\n")
				return 1
			}
		case "--tag":
			if i+1 < len(args) {
				opts.Tag = args[i+1]
				i += 2
			} else {
				fmt.Fprintf(os.Stderr, "Error: --tag requires a value\n")
				return 1
			}
		case "--json":
			opts.JSON = true
			i++
		case "--dry-run":
			opts.DryRun = true
			i++
		case "--help", "-h":
			printDeployHelp()
			return 0
		default:
			if opts.AppName == "" && !startsWith(args[i], "-") {
				opts.AppName = args[i]
				i++
			} else {
				fmt.Fprintf(os.Stderr, "Error: unknown argument %q\n", args[i])
				printDeployHelp()
				return 1
			}
		}
	}

	if opts.AppName == "" {
		fmt.Fprintf(os.Stderr, "Error: app name is required\n")
		printDeployHelp()
		return 1
	}

	return deploy.Run(opts)
}

func startsWith(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

func printHelp() {
	fmt.Println("freedb — FreeDB app manager")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  sudo freedb              Launch the TUI dashboard")
	fmt.Println("  sudo freedb deploy       Deploy/update an app (for CI/CD)")
	fmt.Println("  sudo freedb check        Run health checks")
	fmt.Println("  freedb --version         Print version")
	fmt.Println("  freedb --help            Show this help")
	fmt.Println()
	fmt.Println("Run 'freedb deploy --help' for deploy options.")
}

func printDeployHelp() {
	fmt.Println("Usage: sudo freedb deploy <app-name> [options]")
	fmt.Println()
	fmt.Println("Deploy or update an existing app with a new container image.")
	fmt.Println()
	fmt.Println("Options:")
	fmt.Println("  --image <ref>    Full image reference (e.g., ecr:myapp:v1.2.3)")
	fmt.Println("  --tag <tag>      Tag only (uses existing image base from registry)")
	fmt.Println("  --json           Output result as JSON")
	fmt.Println("  --dry-run        Show what would happen without executing")
	fmt.Println("  --help           Show this help")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  sudo freedb deploy myapp --image ecr:myapp:v1.2.3")
	fmt.Println("  sudo freedb deploy myapp --tag latest")
	fmt.Println("  sudo freedb deploy myapp --tag v1.2.3 --json")
	fmt.Println()
	fmt.Println("Exit codes:")
	fmt.Println("  0  Deploy succeeded")
	fmt.Println("  1  Deploy failed")
	fmt.Println("  2  App not found in registry")
	fmt.Println("  3  Deploy lock timeout")
}
