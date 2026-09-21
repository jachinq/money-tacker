package db

import (
	"database/sql"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"
)

func Open(path string, migrations fs.FS, dir string) (*sql.DB, error) {
	if path != ":memory:" && path != "" {
		dirn := filepath.Dir(path)
		if dirn != "." && dirn != "" {
			if err := os.MkdirAll(dirn, 0o755); err != nil {
				return nil, err
			}
		}
	}
	dsn := path
	switch path {
	case ":memory:", "":
		dsn = "file:memdb1?mode=memory&cache=shared&_pragma=foreign_keys(ON)"
	default:
		dsn = path + "?_pragma=busy_timeout(5000)&_pragma=foreign_keys(ON)&_pragma=journal_mode(WAL)"
	}
	sqlDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxOpenConns(1)
	if _, err := sqlDB.Exec(`PRAGMA foreign_keys = ON; PRAGMA busy_timeout = 5000;`); err != nil {
		return nil, err
	}
	goose.SetBaseFS(migrations)
	if err := goose.SetDialect("sqlite3"); err != nil {
		return nil, err
	}
	if err := goose.Up(sqlDB, dir); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return sqlDB, nil
}
