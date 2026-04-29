package db

import (
	"database/sql"
	"embed"
	"fmt"
	"sync"

	"github.com/pressly/goose/v3"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

var gooseInit sync.Once

// Migrate runs all pending database migrations.
func Migrate(sqlDB *sql.DB, driver string) error {
	var initErr error
	gooseInit.Do(func() {
		goose.SetBaseFS(migrationsFS)
		initErr = goose.SetDialect("postgres")
	})
	if initErr != nil {
		return fmt.Errorf("setting goose dialect: %w", initErr)
	}

	// WithAllowMissing lets goose apply migrations whose version is lower
	// than the current database max — needed when migrations land on main
	// out of numerical order (e.g. 00006/00007 committed after 00008/00009
	// were already applied to existing databases).
	if err := goose.Up(sqlDB, "migrations", goose.WithAllowMissing()); err != nil {
		return fmt.Errorf("running migrations: %w", err)
	}

	return nil
}
