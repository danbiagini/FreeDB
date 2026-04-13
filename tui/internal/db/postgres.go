package db

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/danbiagini/FreeDB/tui/internal/incus"
)

const dbContainer = "db1"

// GeneratePassword creates a random 24-character hex password
func GeneratePassword() string {
	b := make([]byte, 12)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// CreateDatabase creates a PostgreSQL user with a password and a database.
// Returns the generated password.
func CreateDatabase(ctx context.Context, ic *incus.Client, name string) (string, error) {
	password := GeneratePassword()

	// Create user with password
	_, err := ic.Exec(ctx, dbContainer, []string{
		"sudo", "-u", "postgres", "psql", "-c",
		fmt.Sprintf("CREATE USER %s WITH PASSWORD '%s' CREATEDB", name, password),
	})
	if err != nil {
		if !strings.Contains(err.Error(), "already exists") {
			return "", fmt.Errorf("creating user %s: %w", name, err)
		}
		// User exists — update password
		_, err = ic.Exec(ctx, dbContainer, []string{
			"sudo", "-u", "postgres", "psql", "-c",
			fmt.Sprintf("ALTER USER %s WITH PASSWORD '%s'", name, password),
		})
		if err != nil {
			return "", fmt.Errorf("updating password for %s: %w", name, err)
		}
	}

	// Create database
	_, err = ic.Exec(ctx, dbContainer, []string{
		"sudo", "-u", "postgres", "createdb", "-O", name, name,
	})
	if err != nil {
		if !strings.Contains(err.Error(), "already exists") {
			return "", fmt.Errorf("creating database %s: %w", name, err)
		}
	}

	return password, nil
}

func DropDatabase(ctx context.Context, ic *incus.Client, name string) error {
	// Drop database first
	_, err := ic.Exec(ctx, dbContainer, []string{
		"sudo", "-u", "postgres", "dropdb", "--if-exists", name,
	})
	if err != nil {
		return fmt.Errorf("dropping database %s: %w", name, err)
	}

	// Drop user
	_, err = ic.Exec(ctx, dbContainer, []string{
		"sudo", "-u", "postgres", "dropuser", "--if-exists", name,
	})
	if err != nil {
		return fmt.Errorf("dropping user %s: %w", name, err)
	}

	return nil
}

// DatabaseInfo holds information about a PostgreSQL database
type DatabaseInfo struct {
	Name  string
	Owner string
	Size  string
}

// ListDatabases returns all non-system databases in db1
func ListDatabases(ctx context.Context, ic *incus.Client) ([]DatabaseInfo, error) {
	output, err := ic.Exec(ctx, dbContainer, []string{
		"sudo", "-u", "postgres", "psql", "-t", "-A", "-F", "|", "-c",
		"SELECT d.datname, u.usename, pg_size_pretty(pg_database_size(d.datname)) FROM pg_database d JOIN pg_user u ON d.datdba = u.usesysid WHERE d.datistemplate = false AND d.datname != 'postgres' ORDER BY d.datname",
	})
	if err != nil {
		return nil, fmt.Errorf("listing databases: %w", err)
	}

	var dbs []DatabaseInfo
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		parts := strings.SplitN(line, "|", 3)
		if len(parts) != 3 {
			continue
		}
		dbs = append(dbs, DatabaseInfo{
			Name:  strings.TrimSpace(parts[0]),
			Owner: strings.TrimSpace(parts[1]),
			Size:  strings.TrimSpace(parts[2]),
		})
	}

	return dbs, nil
}

// BackupFile represents a local backup file for a database.
type BackupFile struct {
	Name string // filename
	Date string // extracted date (YYYYMMDD)
	Path string // full path
	Size int64
}

// ListBackupFiles returns available backup files for a given database name,
// sorted newest first.
func ListBackupFiles(dbName string) ([]BackupFile, error) {
	backupDir := "/var/lib/freedb/backups"
	entries, err := os.ReadDir(backupDir)
	if err != nil {
		return nil, fmt.Errorf("reading backup directory: %w", err)
	}

	prefix := dbName + "_"
	var files []BackupFile
	for _, e := range entries {
		if e.IsDir() || !strings.HasPrefix(e.Name(), prefix) || !strings.HasSuffix(e.Name(), ".sql.gz") {
			continue
		}
		// Extract timestamp from filename: dbname_YYYYMMDD_HHMMSSZ.sql.gz
		datePart := strings.TrimPrefix(e.Name(), prefix)
		datePart = strings.TrimSuffix(datePart, ".sql.gz")

		info, _ := e.Info()
		size := int64(0)
		if info != nil {
			size = info.Size()
		}

		files = append(files, BackupFile{
			Name: e.Name(),
			Date: datePart,
			Path: backupDir + "/" + e.Name(),
			Size: size,
		})
	}

	// Sort newest first (dates are YYYYMMDD so reverse string sort works)
	for i := 0; i < len(files); i++ {
		for j := i + 1; j < len(files); j++ {
			if files[j].Date > files[i].Date {
				files[i], files[j] = files[j], files[i]
			}
		}
	}

	return files, nil
}

// RestoreDatabase restores a database from a gzipped SQL dump file.
// It drops and recreates the database before restoring.
func RestoreDatabase(ctx context.Context, ic *incus.Client, dbName, backupPath string) error {
	// Drop existing database (ignore error if it doesn't exist)
	_, _ = ic.Exec(ctx, dbContainer, []string{
		"sudo", "-u", "postgres", "dropdb", "--if-exists", dbName,
	})

	// Recreate the database owned by the same user
	_, err := ic.Exec(ctx, dbContainer, []string{
		"sudo", "-u", "postgres", "createdb", "-O", dbName, dbName,
	})
	if err != nil {
		return fmt.Errorf("recreating database %s: %w", dbName, err)
	}

	// Restore: gunzip the backup and pipe into psql via incus exec
	// We run this on the host since the backup file is on the host filesystem
	cmd := fmt.Sprintf("gunzip -c %s | sudo incus exec %s -- sudo -u postgres psql -d %s", backupPath, dbContainer, dbName)
	execCmd := exec.CommandContext(ctx, "bash", "-c", cmd)
	output, err := execCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("restoring %s: %s", dbName, strings.TrimSpace(string(output)))
	}

	return nil
}

func GetDBConnectionString(dbIP, name, password string) string {
	return fmt.Sprintf("postgresql://%s:%s@%s:5432/%s?sslmode=disable", name, password, dbIP, name)
}
