package db

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"

	"github.com/beacon-stack/pilot/internal/config"
)

// DB wraps the underlying sql.DB.
type DB struct {
	SQL *sql.DB
}

// pragmas tune SQLite for a small single-process service: WAL so reads do not
// block the writer, a busy timeout so concurrent access waits instead of
// failing, and foreign-key enforcement.
const pragmas = "?_pragma=busy_timeout(5000)" +
	"&_pragma=journal_mode(WAL)" +
	"&_pragma=foreign_keys(ON)" +
	"&_pragma=synchronous(NORMAL)"

// Open opens the SQLite database at the configured path.
func Open(cfg config.DatabaseConfig) (*DB, error) {
	if cfg.Path == "" {
		return nil, fmt.Errorf("database path must not be empty (set PILOT_DATABASE_PATH)")
	}

	sqlDB, err := sql.Open("sqlite", "file:"+cfg.Path+pragmas)
	if err != nil {
		return nil, fmt.Errorf("opening sqlite database: %w", err)
	}

	// SQLite permits only one writer; a single connection serializes access
	// and removes SQLITE_BUSY contention for a service of this size.
	sqlDB.SetMaxOpenConns(1)

	if err := sqlDB.Ping(); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("pinging sqlite database: %w", err)
	}

	return &DB{SQL: sqlDB}, nil
}

// Close closes the underlying database connection.
func (d *DB) Close() error {
	return d.SQL.Close()
}
