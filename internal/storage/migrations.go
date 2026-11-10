package storage

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"strings"
)

//go:embed schema/*.sql
var schemaFS embed.FS

// RunMigrations applies the appropriate dialect schema to the database.
func RunMigrations(ctx context.Context, db *sql.DB, driver string) error {
	var filename string
	switch driver {
	case "sqlite":
		filename = "schema/sqlite.sql"
	case "postgres":
		filename = "schema/postgres.sql"
	default:
		return fmt.Errorf("unsupported database driver: %s", driver)
	}

	content, err := schemaFS.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("reading embedded migration %s: %w", filename, err)
	}

	// Split by semicolon and strip comment lines
	statements := strings.Split(string(content), ";")
	for _, stmt := range statements {
		clean := cleanSQL(stmt)
		if clean == "" {
			continue
		}
		if _, err := db.ExecContext(ctx, clean); err != nil {
			return fmt.Errorf("executing migration statement [%s]: %w", clean, err)
		}
	}

	return nil
}

func cleanSQL(stmt string) string {
	lines := strings.Split(stmt, "\n")
	var cleanLines []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "--") {
			continue
		}
		cleanLines = append(cleanLines, line)
	}
	return strings.TrimSpace(strings.Join(cleanLines, "\n"))
}
