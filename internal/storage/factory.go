package storage

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"

	"telegram_feed_summarizer/internal/config"
	"telegram_feed_summarizer/internal/domain"
)

// NewRepository creates and initializes a database repository based on configuration.
func NewRepository(ctx context.Context, cfg config.StorageConfig) (domain.Repository, error) {
	driver := cfg.Driver
	dsn := cfg.DSN

	if driver == "auto" || driver == "" {
		if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
			driver = "postgres"
		} else {
			driver = "sqlite"
		}
	}

	var db *sql.DB
	var err error

	switch driver {
	case "sqlite":
		if dsn != ":memory:" && !strings.HasPrefix(dsn, "file:") {
			dir := filepath.Dir(dsn)
			if err := os.MkdirAll(dir, 0755); err != nil {
				return nil, fmt.Errorf("creating database directory %s: %w", dir, err)
			}
		}

		db, err = sql.Open("sqlite", dsn)
		if err != nil {
			return nil, fmt.Errorf("opening sqlite database: %w", err)
		}

		// Enforce WAL mode, timeout, and foreign keys
		pragmas := []string{
			"PRAGMA journal_mode = WAL;",
			"PRAGMA busy_timeout = 5000;",
			"PRAGMA synchronous = NORMAL;",
			"PRAGMA foreign_keys = ON;",
		}
		for _, pragma := range pragmas {
			if _, err := db.ExecContext(ctx, pragma); err != nil {
				// In-memory sqlite might warn on WAL, ignore if memory
				if dsn != ":memory:" {
					db.Close()
					return nil, fmt.Errorf("setting sqlite pragma [%s]: %w", pragma, err)
				}
			}
		}

		// Single connection for writes ensures no lock contention with modernc sqlite
		db.SetMaxOpenConns(1)
		db.SetConnMaxLifetime(time.Hour)

	case "postgres":
		db, err = sql.Open("pgx", dsn)
		if err != nil {
			return nil, fmt.Errorf("opening postgres database: %w", err)
		}
		db.SetMaxOpenConns(10)
		db.SetMaxIdleConns(5)
		db.SetConnMaxLifetime(time.Hour)

		pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		if err := db.PingContext(pingCtx); err != nil {
			db.Close()
			return nil, fmt.Errorf("pinging postgres database: %w", err)
		}

	default:
		return nil, fmt.Errorf("unsupported database driver: %s", driver)
	}

	// Auto-apply migrations
	if err := RunMigrations(ctx, db, driver); err != nil {
		db.Close()
		return nil, fmt.Errorf("running migrations: %w", err)
	}

	repo := NewSQLRepository(db, driver)
	seedDefaultSettings(ctx, repo)
	return repo, nil
}

func seedDefaultSettings(ctx context.Context, repo domain.Repository) {
	val, err := repo.GetSetting(ctx, domain.SettingKeyDigestPrompt, "")
	if err == nil && strings.TrimSpace(val) == "" {
		_ = repo.SetSetting(ctx, domain.SettingKeyDigestPrompt, domain.DefaultDigestPrompt)
	}
}
