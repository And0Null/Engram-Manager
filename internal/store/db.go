package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite" // pure-Go driver: no cgo, so the build stays portable
)

// DefaultPath is where Engram keeps its local database.
const DefaultPath = "~/.engram/engram.db"

// Store is a read-only handle on the Engram database.
type Store struct {
	db   *sql.DB
	path string
}

// Open opens the Engram database read-only.
//
// The connection is opened through a file: URI with mode=ro so that a bug in
// this program cannot write to, or migrate, the user's memory store. Reads
// still see committed WAL content, which matters because Engram is normally
// running and writing at the same time.
func Open(path string) (*Store, error) {
	abs, err := filepath.Abs(expandPath(path))
	if err != nil {
		return nil, fmt.Errorf("resolve engram path: %w", err)
	}
	if _, err := os.Stat(abs); err != nil {
		return nil, fmt.Errorf("engram database not found at %s: %w", abs, err)
	}

	dsn := "file:" + abs + "?mode=ro&_pragma=busy_timeout(5000)&_pragma=query_only(1)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open engram database: %w", err)
	}
	// One connection keeps reads consistent and avoids contending with
	// Engram's own writers for the WAL.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(time.Hour)

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping engram database: %w", err)
	}
	return &Store{db: db, path: abs}, nil
}

// Path is the absolute path of the opened database.
func (s *Store) Path() string { return s.path }

// Close releases the database handle.
func (s *Store) Close() error { return s.db.Close() }

func expandPath(p string) string {
	if p == "" {
		p = DefaultPath
	}
	if p == "~" || strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, strings.TrimPrefix(p, "~"))
		}
	}
	return p
}

// fileSize stats a file, treating a missing file as zero: the WAL legitimately
// does not exist until Engram writes.
func fileSize(path string, out *int64) error {
	fi, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			*out = 0
			return nil
		}
		return fmt.Errorf("stat %s: %w", path, err)
	}
	*out = fi.Size()
	return nil
}

// ftsPhrase turns raw user input into a single safe FTS5 phrase.
//
// Engram indexes observations_fts with a trigram tokenizer, so an unquoted
// term would be parsed as FTS5 syntax and a stray quote or dash would fail the
// whole query. Quoting every token as one phrase keeps search literal — which
// is also how mem_search itself behaves.
func ftsPhrase(q string) string {
	q = strings.TrimSpace(q)
	if q == "" {
		return ""
	}
	return `"` + strings.ReplaceAll(q, `"`, `""`) + `"`
}
