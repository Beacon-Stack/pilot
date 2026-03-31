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

		dialect := driver
		if driver == "sqlite" {
			dialect = "sqlite3"
		}

		initErr = goose.SetDialect(dialect)
	})
	if initErr != nil {
		return fmt.Errorf("setting goose dialect: %w", initErr)
	}

	if err := goose.Up(sqlDB, "migrations"); err != nil {
		return fmt.Errorf("running migrations: %w", err)
	}

	return nil
}
