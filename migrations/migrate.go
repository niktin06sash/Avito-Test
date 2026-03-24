package migrate

import (
	"embed"
	"fmt"

	mig "github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

//go:embed 001_init.up.sql
var Migrations embed.FS

func Migrate(dsn string, fs embed.FS) error {
	d, err := iofs.New(fs, ".")
	if err != nil {
		return fmt.Errorf("failed to create iofs driver: %w", err)
	}
	m, err := mig.NewWithSourceInstance("iofs", d, dsn)
	if err != nil {
		return fmt.Errorf("failed to create migrate instance: %w", err)
	}
	if err := m.Up(); err != nil && err != mig.ErrNoChange {
		return fmt.Errorf("failed to apply migrations: %w", err)
	}

	return nil
}
