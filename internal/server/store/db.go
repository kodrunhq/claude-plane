package store

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

// Store provides access to the SQLite database with separate writer and reader
// connection pools. The writer pool is limited to a single connection to enforce
// SQLite's single-writer constraint. The reader pool allows concurrent reads.
type Store struct {
	writer *sql.DB
	reader *sql.DB
}

// NewStore creates a new Store backed by a SQLite database at dbPath.
// It initializes WAL mode, enables foreign keys, sets busy timeout, and runs
// schema migrations. Pragmas are embedded in the DSN so that every new pool
// connection is automatically configured (no per-connection init needed).
func NewStore(dbPath string) (*Store, error) {
	// Writer: single connection, IMMEDIATE transactions.
	// Pragmas in the DSN are applied to every connection the pool creates.
	writerDSN := fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(wal)&_pragma=foreign_keys(1)&_pragma=synchronous(normal)", dbPath)
	writer, err := sql.Open("sqlite", writerDSN)
	if err != nil {
		return nil, fmt.Errorf("open writer: %w", err)
	}
	writer.SetMaxOpenConns(1)

	// Run schema migrations on the writer
	if err := RunMigrations(writer); err != nil {
		writer.Close()
		return nil, fmt.Errorf("run migrations: %w", err)
	}

	// Reader: multiple connections for concurrent reads.
	// WAL mode is already set at the database level by the writer.
	// Pragmas in the DSN ensure every new reader connection is configured.
	readerDSN := fmt.Sprintf("file:%s?mode=ro&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)&_pragma=synchronous(normal)", dbPath)
	reader, err := sql.Open("sqlite", readerDSN)
	if err != nil {
		writer.Close()
		return nil, fmt.Errorf("open reader: %w", err)
	}
	reader.SetMaxOpenConns(4)

	return &Store{writer: writer, reader: reader}, nil
}

// Writer returns the write-only database pool (MaxOpenConns=1).
func (s *Store) Writer() *sql.DB {
	return s.writer
}

// Reader returns the read-only database pool (MaxOpenConns=4).
func (s *Store) Reader() *sql.DB {
	return s.reader
}

// Close closes both the writer and reader database pools.
func (s *Store) Close() error {
	wErr := s.writer.Close()
	rErr := s.reader.Close()
	if wErr != nil {
		return fmt.Errorf("close writer: %w", wErr)
	}
	if rErr != nil {
		return fmt.Errorf("close reader: %w", rErr)
	}
	return nil
}
