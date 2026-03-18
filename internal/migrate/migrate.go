package migrate

import (
	"embed"
	"fmt"
	"log/slog"
	"net/url"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	_ "github.com/lib/pq"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// ensureSSLMode appends sslmode=disable to the URL if no sslmode is set (lib/pq defaults to require SSL).
func ensureSSLMode(databaseURL string) string {
	u, err := url.Parse(databaseURL)
	if err != nil {
		return databaseURL
	}
	q := u.Query()
	if q.Get("sslmode") == "" {
		q.Set("sslmode", "disable")
		u.RawQuery = q.Encode()
		return u.String()
	}
	return databaseURL
}

// Run runs all pending up migrations. Idempotent: safe to call on every startup.
func Run(databaseURL string) error {
	// lib/pq defaults to SSL; in-cluster Postgres often has SSL disabled
	databaseURL = ensureSSLMode(databaseURL)
	src, err := iofs.New(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("migrate source: %w", err)
	}
	m, err := migrate.NewWithSourceInstance("iofs", src, databaseURL)
	if err != nil {
		return fmt.Errorf("migrate new: %w", err)
	}
	defer m.Close()
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("migrate up: %w", err)
	}
	if err == migrate.ErrNoChange {
		slog.Debug("migrations: already up to date")
	} else {
		slog.Info("migrations: applied successfully")
	}
	return nil
}
