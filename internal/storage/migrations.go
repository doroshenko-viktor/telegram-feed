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

	// Split statements by semicolon while respecting string literals
	statements := splitSQLStatements(string(content))
	for _, stmt := range statements {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("executing migration statement [%s]: %w", stmt, err)
		}
	}

	return nil
}

func splitSQLStatements(content string) []string {
	var statements []string
	var current strings.Builder
	inString := false

	runes := []rune(content)
	n := len(runes)

	for i := 0; i < n; i++ {
		r := runes[i]
		if r == '\'' {
			// Handle escaped quote ''
			if inString && i+1 < n && runes[i+1] == '\'' {
				current.WriteRune(r)
				current.WriteRune('\'')
				i++
				continue
			}
			inString = !inString
		}

		if r == ';' && !inString {
			stmt := cleanSQL(current.String())
			if stmt != "" {
				statements = append(statements, stmt)
			}
			current.Reset()
			continue
		}

		current.WriteRune(r)
	}

	tail := cleanSQL(current.String())
	if tail != "" {
		statements = append(statements, tail)
	}

	return statements
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
