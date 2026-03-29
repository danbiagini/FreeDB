package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/danbiagini/freedb-tui/internal/check"
	"github.com/danbiagini/freedb-tui/internal/config"
	"github.com/danbiagini/freedb-tui/internal/db"
	"github.com/danbiagini/freedb-tui/internal/deploy"
	"github.com/danbiagini/freedb-tui/internal/incus"
	"github.com/danbiagini/freedb-tui/internal/registry"
	"github.com/danbiagini/freedb-tui/internal/traefik"
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
		case "list", "ls":
			os.Exit(runList(os.Args[2:]))
		case "destroy", "rm":
			os.Exit(runDestroy(os.Args[2:]))
		case "status":
			os.Exit(runStatus(os.Args[2:]))
		case "upgrade":
			os.Exit(runUpgrade(os.Args[2:]))
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

func runList(args []string) int {
	jsonOutput := false
	for _, a := range args {
		if a == "--json" {
			jsonOutput = true
		}
		if a == "--help" || a == "-h" {
			fmt.Println("Usage: sudo freedb list [--json]")
			fmt.Println()
			fmt.Println("List all deployed apps.")
			return 0
		}
	}

	reg, err := registry.Load("/etc/freedb/registry.json")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading registry: %v\n", err)
		return 1
	}

	ic, err := incus.Connect("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error connecting to incus: %v\n", err)
		return 1
	}

	apps := reg.List()
	if len(apps) == 0 {
		if !jsonOutput {
			fmt.Println("No apps deployed.")
		} else {
			fmt.Println("[]")
		}
		return 0
	}

	if jsonOutput {
		type appInfo struct {
			Name      string `json:"name"`
			Image     string `json:"image"`
			Domain    string `json:"domain"`
			Status    string `json:"status"`
			Container string `json:"container"`
			IP        string `json:"ip"`
		}
		var infos []appInfo
		for _, app := range apps {
			cName := app.Name
			if app.ContainerName != "" {
				cName = app.ContainerName
			}
			status := "unknown"
			ip := ""
			if containers, err := ic.ListContainers(context.Background()); err == nil {
				for _, c := range containers {
					if c.Name == cName {
						status = c.Status
						ip = c.IP
						break
					}
				}
			}
			infos = append(infos, appInfo{
				Name:      app.Name,
				Image:     app.Image,
				Domain:    app.Domain,
				Status:    status,
				Container: cName,
				IP:        ip,
			})
		}
		json.NewEncoder(os.Stdout).Encode(infos)
		return 0
	}

	// Get container states
	containers, _ := ic.ListContainers(context.Background())
	containerMap := make(map[string]incus.ContainerInfo)
	for _, c := range containers {
		containerMap[c.Name] = c
	}

	fmt.Printf("%-20s %-10s %-30s %s\n", "NAME", "STATUS", "DOMAIN", "IMAGE")
	fmt.Printf("%-20s %-10s %-30s %s\n", "----", "------", "------", "-----")
	for _, app := range apps {
		cName := app.Name
		if app.ContainerName != "" {
			cName = app.ContainerName
		}
		status := "unknown"
		if c, ok := containerMap[cName]; ok {
			status = c.Status
		}
		// Short image name
		img := app.Image
		if parts := strings.Split(img, "/"); len(parts) > 1 {
			img = parts[len(parts)-1]
		}
		fmt.Printf("%-20s %-10s %-30s %s\n", app.Name, status, app.Domain, img)
	}
	return 0
}

func runStatus(args []string) int {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		fmt.Println("Usage: sudo freedb status <app-name> [--json]")
		return 0
	}

	appName := args[0]
	jsonOutput := false
	for _, a := range args[1:] {
		if a == "--json" {
			jsonOutput = true
		}
	}

	reg, err := registry.Load("/etc/freedb/registry.json")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading registry: %v\n", err)
		return 1
	}

	app, ok := reg.Get(appName)
	if !ok {
		fmt.Fprintf(os.Stderr, "Error: app %q not found\n", appName)
		return 2
	}

	ic, err := incus.Connect("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error connecting to incus: %v\n", err)
		return 1
	}

	cName := app.Name
	if app.ContainerName != "" {
		cName = app.ContainerName
	}

	detail, _ := ic.GetContainerDetail(context.Background(), cName)
	envVars, _ := ic.GetEnvVars(context.Background(), cName)

	if jsonOutput {
		info := map[string]any{
			"name":      app.Name,
			"container": cName,
			"image":     app.Image,
			"domain":    app.Domain,
			"port":      app.Port,
			"tls":       app.TLS,
			"has_db":    app.HasDB,
			"db_name":   app.DBName,
			"env_vars":  envVars,
		}
		if detail != nil {
			info["status"] = detail.Status
			info["ip"] = detail.IP
			info["memory_mb"] = detail.MemUsageMB
			info["disk_mb"] = detail.DiskUsageMB
			info["uptime_seconds"] = int(detail.Uptime.Seconds())
			info["processes"] = detail.Processes
		}
		json.NewEncoder(os.Stdout).Encode(info)
		return 0
	}

	fmt.Printf("App:        %s\n", app.Name)
	fmt.Printf("Container:  %s\n", cName)
	fmt.Printf("Image:      %s\n", app.Image)
	fmt.Printf("Domain:     %s\n", app.Domain)
	fmt.Printf("Port:       %d\n", app.Port)
	fmt.Printf("TLS:        %v\n", app.TLS)
	if app.HasDB {
		fmt.Printf("Database:   %s\n", app.DBName)
	}

	if detail != nil {
		fmt.Println()
		fmt.Printf("Status:     %s\n", detail.Status)
		fmt.Printf("IP:         %s\n", detail.IP)
		if detail.MemUsageMB > 0 {
			fmt.Printf("Memory:     %d MB\n", detail.MemUsageMB)
		}
		if detail.DiskUsageMB > 0 {
			fmt.Printf("Disk:       %d MB\n", detail.DiskUsageMB)
		}
		if detail.Uptime > 0 {
			fmt.Printf("Uptime:     %s\n", detail.Uptime.Truncate(time.Second))
		}
		if detail.Processes > 0 {
			fmt.Printf("Processes:  %d\n", detail.Processes)
		}
	}

	if len(envVars) > 0 {
		fmt.Println()
		fmt.Println("Environment:")
		for k, v := range envVars {
			fmt.Printf("  %s=%s\n", k, v)
		}
	}

	return 0
}

func runDestroy(args []string) int {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		fmt.Println("Usage: sudo freedb destroy <app-name> [--yes]")
		fmt.Println()
		fmt.Println("Delete an app and all its resources (container, route, database).")
		fmt.Println()
		fmt.Println("Options:")
		fmt.Println("  --yes    Skip confirmation prompt")
		return 0
	}

	appName := args[0]
	skipConfirm := false
	for _, a := range args[1:] {
		if a == "--yes" || a == "-y" {
			skipConfirm = true
		}
	}

	reg, err := registry.Load("/etc/freedb/registry.json")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading registry: %v\n", err)
		return 1
	}

	app, ok := reg.Get(appName)
	if !ok {
		fmt.Fprintf(os.Stderr, "Error: app %q not found\n", appName)
		return 2
	}

	cName := appName
	if app.ContainerName != "" {
		cName = app.ContainerName
	}

	if !skipConfirm {
		fmt.Printf("Delete app %q?\n", appName)
		fmt.Printf("  Container: %s\n", cName)
		fmt.Printf("  Domain:    %s\n", app.Domain)
		if app.HasDB {
			fmt.Printf("  Database:  %s (WILL BE DROPPED)\n", app.DBName)
		}
		fmt.Print("\nType 'yes' to confirm: ")
		var confirm string
		fmt.Scanln(&confirm)
		if confirm != "yes" {
			fmt.Println("Aborted.")
			return 0
		}
	}

	ic, err := incus.Connect("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error connecting to incus: %v\n", err)
		return 1
	}

	ctx := context.Background()

	// Delete container
	fmt.Printf("Deleting container %s...\n", cName)
	if err := ic.DeleteContainer(ctx, cName); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
	}

	// Delete Traefik route
	fmt.Println("Removing Traefik route...")
	_ = traefik.DeleteRoute(ic, appName)

	// Drop database
	if app.HasDB && app.DBName != "" {
		fmt.Printf("Dropping database %s...\n", app.DBName)
		_ = db.DropDatabase(ctx, ic, app.DBName)
	}

	// Remove from registry
	fmt.Println("Removing from registry...")
	if err := reg.Remove(appName); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
	}

	fmt.Println("Done.")
	return 0
}

func runUpgrade(args []string) int {
	dryRun := false
	for _, a := range args {
		if a == "--dry-run" {
			dryRun = true
		}
		if a == "--help" || a == "-h" {
			fmt.Println("Usage: sudo freedb upgrade [--dry-run]")
			fmt.Println()
			fmt.Println("Run pending platform migrations to upgrade FreeDB.")
			return 0
		}
	}

	// Read current installed version
	versionFile := "/etc/freedb/version"
	currentVersion := "v0.2" // default for pre-upgrade installations
	if data, err := os.ReadFile(versionFile); err == nil {
		v := strings.TrimSpace(string(data))
		// Extract the version tag (strip git describe suffix like -N-gabcdef)
		if parts := strings.SplitN(v, "-", 2); len(parts) > 0 && strings.HasPrefix(parts[0], "v") {
			currentVersion = parts[0]
		}
	}

	fmt.Printf("Current version: %s\n", currentVersion)
	fmt.Printf("Target version:  v0.3\n")
	fmt.Println()

	// Define migration order
	migrations := []struct {
		version string
		script  string
	}{
		{"v0.3", "platform/migrations/v0.3.sh"},
	}

	// Find pending migrations
	var pending []struct {
		version string
		script  string
	}
	found := false
	for _, m := range migrations {
		if m.version == currentVersion {
			found = true
			continue // skip current version
		}
		if found || currentVersion < m.version {
			pending = append(pending, m)
		}
	}

	if len(pending) == 0 {
		fmt.Println("Already up to date.")
		return 0
	}

	fmt.Printf("Pending migrations: %d\n", len(pending))
	for _, m := range pending {
		fmt.Printf("  %s — %s\n", m.version, m.script)
	}
	fmt.Println()

	if dryRun {
		fmt.Println("(dry run — no changes made)")
		return 0
	}

	// Find the repo root (look for install.sh)
	repoRoot := ""
	for _, candidate := range []string{
		"/home/" + os.Getenv("SUDO_USER") + "/FreeDB",
		os.Getenv("HOME") + "/FreeDB",
		"/root/FreeDB",
	} {
		if _, err := os.Stat(candidate + "/install.sh"); err == nil {
			repoRoot = candidate
			break
		}
	}
	if repoRoot == "" {
		fmt.Fprintf(os.Stderr, "Error: cannot find FreeDB repo. Clone it to ~/FreeDB first.\n")
		return 1
	}

	// Run pending migrations
	for _, m := range pending {
		scriptPath := repoRoot + "/" + m.script
		if _, err := os.Stat(scriptPath); os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "Error: migration script not found: %s\n", scriptPath)
			return 1
		}

		fmt.Printf("Running migration %s...\n", m.version)
		cmd := exec.CommandContext(context.Background(), "bash", scriptPath)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Dir = repoRoot
		if err := cmd.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: migration %s failed: %v\n", m.version, err)
			return 1
		}

		// Update version file
		os.MkdirAll("/etc/freedb", 0755)
		os.WriteFile(versionFile, []byte(m.version+"\n"), 0644)
		fmt.Printf("Updated version to %s\n\n", m.version)
	}

	fmt.Println("Upgrade complete.")
	return 0
}

func printHelp() {
	fmt.Println("freedb — FreeDB app manager")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  sudo freedb              Launch the TUI dashboard")
	fmt.Println("  sudo freedb list         List deployed apps")
	fmt.Println("  sudo freedb status APP   Show detailed app status")
	fmt.Println("  sudo freedb deploy       Deploy/update an app (for CI/CD)")
	fmt.Println("  sudo freedb destroy APP  Delete an app and all its resources")
	fmt.Println("  sudo freedb upgrade      Run pending platform migrations")
	fmt.Println("  sudo freedb check        Run health checks")
	fmt.Println("  freedb --version         Print version")
	fmt.Println("  freedb --help            Show this help")
	fmt.Println()
	fmt.Println("Run 'freedb <command> --help' for command-specific options.")
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
