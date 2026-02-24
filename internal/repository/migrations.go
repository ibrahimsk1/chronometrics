package repository

import (
	"context"
	"os"
	"path/filepath"
	"sort"

	clickhouse "github.com/ClickHouse/clickhouse-go/v2"
)

// ApplyMigrations reads all .sql files in the given directory (non-recursive),
// sorts them by filename, and executes them against the provided ClickHouse connection.
func ApplyMigrations(ctx context.Context, conn clickhouse.Conn, dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	var files []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if filepath.Ext(e.Name()) == ".sql" {
			files = append(files, filepath.Join(dir, e.Name()))
		}
	}
	sort.Strings(files)
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			return err
		}
		if err := conn.Exec(ctx, string(data)); err != nil {
			return err
		}
	}
	return nil
}

