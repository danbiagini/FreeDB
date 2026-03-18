package db

import (
	"context"
	"fmt"
	"strings"

	"github.com/danbiagini/freedb-tui/internal/incus"
)

const dbContainer = "db1"

func CreateDatabase(ctx context.Context, ic *incus.Client, name string) error {
	// Create user
	_, err := ic.Exec(ctx, dbContainer, []string{
		"sudo", "-u", "postgres", "createuser", "-d", name,
	})
	if err != nil {
		// User might already exist
		if !strings.Contains(err.Error(), "already exists") {
			return fmt.Errorf("creating user %s: %w", name, err)
		}
	}

	// Create database
	_, err = ic.Exec(ctx, dbContainer, []string{
		"sudo", "-u", "postgres", "createdb", "-O", name, name,
	})
	if err != nil {
		if !strings.Contains(err.Error(), "already exists") {
			return fmt.Errorf("creating database %s: %w", name, err)
		}
	}

	return nil
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

func GetDBConnectionString(dbIP string, name string) string {
	return fmt.Sprintf("postgresql://%s@%s:5432/%s?sslmode=disable", name, dbIP, name)
}
