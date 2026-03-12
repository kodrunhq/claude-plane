package store

import (
	"context"
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

// pragmas contains the PRAGMA statements applied to every database connection.
// WAL mode is set on the writer (it's a database-level setting that persists).
// Foreign keys and busy timeout must be set per-connection.
const pragmas = `
PRAGMA busy_timeout = 5000;
PRAGMA synchronous = NORMAL;
PRAGMA foreign_keys = ON;
`

// NewStore creates a new Store backed by a SQLite database at dbPath.
// It initializes WAL mode, enables foreign keys, sets busy timeout, and runs
// schema migrations.
func NewStore(dbPath string) (*Store, error) {
	// Writer: single connection, IMMEDIATE transactions
	writerDSN := fmt.Sprintf("file:%s", dbPath)
	writer, err := sql.Open("sqlite", writerDSN)
	if err != nil {
		return nil, fmt.Errorf("open writer: %w", err)
	}
	writer.SetMaxOpenConns(1)

	// Set WAL mode (persists at the database level)
	if _, err := writer.Exec("PRAGMA journal_mode=WAL"); err != nil {
		writer.Close()
		return nil, fmt.Errorf("set WAL mode: %w", err)
	}

	// Set per-connection pragmas on the writer
	if _, err := writer.Exec(pragmas); err != nil {
		writer.Close()
		return nil, fmt.Errorf("set writer pragmas: %w", err)
	}

	// Run schema migrations on the writer
	if err := RunMigrations(writer); err != nil {
		writer.Close()
		return nil, fmt.Errorf("run migrations: %w", err)
	}

	// Reader: multiple connections for concurrent reads
	// WAL mode is already set at the database level by the writer.
	readerDSN := fmt.Sprintf("file:%s?mode=ro", dbPath)
	reader, err := sql.Open("sqlite", readerDSN)
	if err != nil {
		writer.Close()
		return nil, fmt.Errorf("open reader: %w", err)
	}
	reader.SetMaxOpenConns(4)

	// Initialize all reader connections with per-connection pragmas.
	// sql.DB lazily creates connections, so we explicitly acquire each one
	// and run pragmas on it to ensure all pooled connections are configured.
	if err := initAllConns(reader, 4); err != nil {
		writer.Close()
		reader.Close()
		return nil, fmt.Errorf("init reader conns: %w", err)
	}

	return &Store{writer: writer, reader: reader}, nil
}

// initAllConns acquires n connections from the pool and runs pragmas on each.
// Connections are returned to the pool after initialization.
func initAllConns(db *sql.DB, n int) error {
	ctx := context.Background()
	conns := make([]*sql.Conn, 0, n)

	// Acquire all connections to force the pool to create them
	for i := 0; i < n; i++ {
		conn, err := db.Conn(ctx)
		if err != nil {
			// Release already-acquired connections
			for _, c := range conns {
				c.Close()
			}
			return fmt.Errorf("acquire conn %d: %w", i, err)
		}
		// Run pragmas on this specific connection
		if _, err := conn.ExecContext(ctx, pragmas); err != nil {
			conn.Close()
			for _, c := range conns {
				c.Close()
			}
			return fmt.Errorf("set pragmas on conn %d: %w", i, err)
		}
		conns = append(conns, conn)
	}

	// Return all connections to the pool
	for _, c := range conns {
		c.Close()
	}
	return nil
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
