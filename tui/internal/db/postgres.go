package db

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/danbiagini/freedb-tui/internal/incus"
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

func GetDBConnectionString(dbIP, name, password string) string {
	return fmt.Sprintf("postgresql://%s:%s@%s:5432/%s?sslmode=disable", name, password, dbIP, name)
}
