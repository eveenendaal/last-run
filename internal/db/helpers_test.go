package db_test

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/eveenendaal/last-run/internal/db"
	"github.com/eveenendaal/last-run/internal/model"
)

// newTestDB opens a fresh, schema-initialized SQLite database backed by a
// temporary file (cleaned up automatically when the test finishes).
func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	database, err := db.Open(path)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := db.InitDB(database); err != nil {
		t.Fatalf("init db: %v", err)
	}
	return database
}

func makeTask(id string) *model.Task {
	return &model.Task{ID: id}
}

func ptr(s string) *string { return &s }
