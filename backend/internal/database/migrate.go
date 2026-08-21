package database

import (
	"errors"
	"fmt"
	"io/fs"
	"strings"

	"github.com/golang-migrate/migrate/v4"
	// Driver de base de datos pgx/v5 (registra el esquema "pgx5").
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

// RunMigrations aplica todas las migraciones pendientes embebidas en migFS.
// databaseURL es el DSN estándar (postgres://...); se traduce al esquema pgx5.
func RunMigrations(migFS fs.FS, databaseURL string) error {
	src, err := iofs.New(migFS, ".")
	if err != nil {
		return fmt.Errorf("database: iofs: %w", err)
	}
	defer src.Close()

	m, err := migrate.NewWithSourceInstance("iofs", src, toPgx5(databaseURL))
	if err != nil {
		return fmt.Errorf("database: init migrate: %w", err)
	}
	defer m.Close()

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("database: migrate up: %w", err)
	}
	return nil
}

// toPgx5 traduce el esquema del DSN al que espera el driver de golang-migrate.
func toPgx5(url string) string {
	switch {
	case strings.HasPrefix(url, "postgresql://"):
		return "pgx5://" + strings.TrimPrefix(url, "postgresql://")
	case strings.HasPrefix(url, "postgres://"):
		return "pgx5://" + strings.TrimPrefix(url, "postgres://")
	default:
		return url
	}
}
