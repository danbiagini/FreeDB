package upgrade

import (
	"embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

//go:embed migrations/*.sh
var migrationFS embed.FS

//go:embed files/backup-db.sh
var backupScript []byte

//go:embed files/registry-auth.sh
var registryAuthScript []byte

const versionFile = "/etc/freedb/version"

type Migration struct {
	Version string
	Script  string // filename in migrations/
}

// All migrations in order
var migrations = []Migration{
	{Version: "v0.3", Script: "v0.3.sh"},
	{Version: "v0.4", Script: "v0.4.sh"},
	{Version: "v0.5", Script: "v0.5.sh"},
	{Version: "v0.6", Script: "v0.6.sh"},
	{Version: "v0.7", Script: "v0.7.sh"},
}

// InstallBackupScript writes the embedded backup script to /opt/freedb/
func InstallBackupScript() error {
	dir := "/opt/freedb"
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "backup-db.sh"), backupScript, 0755)
}

// InstallAuthScript writes the embedded registry auth script to /usr/local/bin/
func InstallAuthScript() error {
	return os.WriteFile("/usr/local/bin/freedb-registry-auth.sh", registryAuthScript, 0755)
}

func CurrentVersion() string {
	data, err := os.ReadFile(versionFile)
	if err != nil {
		return "v0.2" // default for pre-upgrade installations
	}
	v := strings.TrimSpace(string(data))
	// Extract version tag (strip git describe suffix like -N-gabcdef)
	if parts := strings.SplitN(v, "-", 2); len(parts) > 0 && strings.HasPrefix(parts[0], "v") {
		return parts[0]
	}
	return v
}

func LatestVersion() string {
	if len(migrations) == 0 {
		return "v0.2"
	}
	return migrations[len(migrations)-1].Version
}

func PendingFrom(from string) []Migration {
	var pending []Migration
	for _, m := range migrations {
		if m.Version > from {
			pending = append(pending, m)
		}
	}
	return pending
}

func PendingMigrations() []Migration {
	return PendingFrom(CurrentVersion())
}

func Run(dryRun bool, fromVersion string) int {
	current := CurrentVersion()
	if fromVersion != "" {
		current = fromVersion
		fmt.Printf("Overriding detected version with: %s\n", current)
	}
	latest := LatestVersion()

	fmt.Printf("Current version: %s\n", current)
	fmt.Printf("Target version:  %s\n", latest)
	fmt.Println()

	pending := PendingFrom(current)
	if len(pending) == 0 {
		fmt.Println("Already up to date.")
		return 0
	}

	fmt.Printf("Pending migrations: %d\n", len(pending))
	for _, m := range pending {
		fmt.Printf("  %s\n", m.Version)
	}
	fmt.Println()

	if dryRun {
		fmt.Println("(dry run — no changes made)")
		return 0
	}

	// Run each migration
	for _, m := range pending {
		// Extract embedded script to a temp file
		scriptData, err := migrationFS.ReadFile("migrations/" + m.Script)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: reading migration %s: %v\n", m.Version, err)
			return 1
		}

		tmpFile := filepath.Join(os.TempDir(), "freedb-migration-"+m.Version+".sh")
		if err := os.WriteFile(tmpFile, scriptData, 0755); err != nil {
			fmt.Fprintf(os.Stderr, "Error: writing migration script: %v\n", err)
			return 1
		}
		defer os.Remove(tmpFile)

		fmt.Printf("Running migration %s...\n", m.Version)
		cmd := exec.Command("bash", tmpFile)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "\nError: migration %s failed: %v\n", m.Version, err)
			fmt.Fprintf(os.Stderr, "\nThe migration scripts are idempotent and safe to re-run.\n")
			fmt.Fprintf(os.Stderr, "To retry this migration:\n")
			fmt.Fprintf(os.Stderr, "  sudo freedb upgrade --from %s\n", current)
			fmt.Fprintf(os.Stderr, "\nTo skip it and continue from the next version:\n")
			fmt.Fprintf(os.Stderr, "  sudo freedb upgrade --from %s\n", m.Version)
			return 1
		}

		// Update version file after each successful migration
		os.MkdirAll(filepath.Dir(versionFile), 0755)
		os.WriteFile(versionFile, []byte(m.Version+"\n"), 0644)
		fmt.Printf("Updated version to %s\n\n", m.Version)

		// Update current for error messages in subsequent iterations
		current = m.Version
	}

	fmt.Println("Upgrade complete.")
	return 0
}
