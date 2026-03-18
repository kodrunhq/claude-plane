package logging

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// LogRecord is a single log entry.
type LogRecord struct {
	ID        int64     `json:"id"`
	Timestamp time.Time `json:"timestamp"`
	Level     string    `json:"level"`
	Component string    `json:"component"`
	Message   string    `json:"message"`
	MachineID string    `json:"machine_id,omitempty"`
	SessionID string    `json:"session_id,omitempty"`
	Error     string    `json:"error,omitempty"`
	Metadata  string    `json:"metadata,omitempty"`
	Source    string    `json:"source"`
}

// LogFilter specifies query filters for log retrieval.
type LogFilter struct {
	Level     string
	Component string
	Source    string
	MachineID string
	SessionID string
	Since     time.Time
	Until     time.Time
	Search    string
	Limit     int
	Offset    int
}

// LogStats holds aggregated log counts.
type LogStats struct {
	ByLevel     map[string]int `json:"by_level"`
	ByComponent map[string]int `json:"by_component"`
	Total       int            `json:"total"`
}

// LogStore manages a separate SQLite database for structured logs.
type LogStore struct {
	writer *sql.DB
	reader *sql.DB
}

const createLogsSQL = `
CREATE TABLE IF NOT EXISTS server_logs (
	id         INTEGER PRIMARY KEY AUTOINCREMENT,
	timestamp  DATETIME NOT NULL,
	level      TEXT NOT NULL,
	component  TEXT NOT NULL DEFAULT '',
	message    TEXT NOT NULL,
	machine_id TEXT,
	session_id TEXT,
	error      TEXT,
	metadata   TEXT,
	source     TEXT NOT NULL DEFAULT 'server'
);
CREATE INDEX IF NOT EXISTS idx_logs_primary_query ON server_logs(level, component, source, timestamp);
CREATE INDEX IF NOT EXISTS idx_logs_machine_id ON server_logs(machine_id) WHERE machine_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_logs_session_id ON server_logs(session_id) WHERE session_id IS NOT NULL;
`

// NewLogStore creates a LogStore backed by a separate SQLite database at dbPath.
// It follows the same WAL-mode, writer/reader pool pattern as the main store.
func NewLogStore(dbPath string) (*LogStore, error) {
	writerDSN := fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(wal)&_pragma=foreign_keys(0)&_pragma=synchronous(normal)", dbPath)
	writer, err := sql.Open("sqlite", writerDSN)
	if err != nil {
		return nil, fmt.Errorf("open log writer: %w", err)
	}
	writer.SetMaxOpenConns(1)

	if _, err := writer.Exec(createLogsSQL); err != nil {
		writer.Close()
		return nil, fmt.Errorf("create log tables: %w", err)
	}

	readerDSN := fmt.Sprintf("file:%s?mode=ro&_pragma=busy_timeout(5000)&_pragma=synchronous(normal)", dbPath)
	reader, err := sql.Open("sqlite", readerDSN)
	if err != nil {
		writer.Close()
		return nil, fmt.Errorf("open log reader: %w", err)
	}
	reader.SetMaxOpenConns(4)

	return &LogStore{writer: writer, reader: reader}, nil
}

// InsertBatch writes a batch of log records in a single transaction.
func (ls *LogStore) InsertBatch(records []LogRecord) error {
	if len(records) == 0 {
		return nil
	}
	tx, err := ls.writer.Begin()
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	stmt, err := tx.Prepare(`INSERT INTO server_logs (timestamp, level, component, message, machine_id, session_id, error, metadata, source) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("prepare: %w", err)
	}
	defer stmt.Close()

	for _, r := range records {
		_, err := stmt.Exec(r.Timestamp.UTC().Format(time.RFC3339Nano), r.Level, r.Component, r.Message,
			nullStr(r.MachineID), nullStr(r.SessionID), nullStr(r.Error), nullStr(r.Metadata), r.Source)
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("exec: %w", err)
		}
	}
	return tx.Commit()
}

func nullStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// Query retrieves log records matching the given filter. Returns matching
// records and the total count (for pagination).
func (ls *LogStore) Query(f LogFilter) ([]LogRecord, int, error) {
	if f.Limit <= 0 {
		f.Limit = 100
	}
	if f.Limit > 500 {
		f.Limit = 500
	}

	var where []string
	var args []any

	if f.Level != "" {
		where = append(where, "level = ?")
		args = append(args, f.Level)
	}
	if f.Component != "" {
		where = append(where, "component = ?")
		args = append(args, f.Component)
	}
	if f.Source != "" {
		where = append(where, "source = ?")
		args = append(args, f.Source)
	}
	if f.MachineID != "" {
		where = append(where, "machine_id = ?")
		args = append(args, f.MachineID)
	}
	if f.SessionID != "" {
		where = append(where, "session_id = ?")
		args = append(args, f.SessionID)
	}
	if !f.Since.IsZero() {
		where = append(where, "timestamp >= ?")
		args = append(args, f.Since.UTC().Format(time.RFC3339Nano))
	}
	if !f.Until.IsZero() {
		where = append(where, "timestamp <= ?")
		args = append(args, f.Until.UTC().Format(time.RFC3339Nano))
	}
	if f.Search != "" {
		where = append(where, "(message LIKE ? OR error LIKE ?)")
		term := "%" + f.Search + "%"
		args = append(args, term, term)
	}

	whereClause := ""
	if len(where) > 0 {
		whereClause = "WHERE " + strings.Join(where, " AND ")
	}

	var total int
	if err := ls.reader.QueryRow("SELECT COUNT(*) FROM server_logs "+whereClause, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count: %w", err)
	}

	dataArgs := make([]any, len(args)+2)
	copy(dataArgs, args)
	dataArgs[len(args)] = f.Limit
	dataArgs[len(args)+1] = f.Offset

	rows, err := ls.reader.Query(
		fmt.Sprintf("SELECT id, timestamp, level, component, message, machine_id, session_id, error, metadata, source FROM server_logs %s ORDER BY timestamp DESC LIMIT ? OFFSET ?", whereClause),
		dataArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("query: %w", err)
	}
	defer rows.Close()

	var results []LogRecord
	for rows.Next() {
		var r LogRecord
		var ts string
		var machineID, sessionID, errStr, metadata sql.NullString
		if err := rows.Scan(&r.ID, &ts, &r.Level, &r.Component, &r.Message, &machineID, &sessionID, &errStr, &metadata, &r.Source); err != nil {
			return nil, 0, fmt.Errorf("scan: %w", err)
		}
		r.Timestamp, _ = time.Parse(time.RFC3339Nano, ts)
		r.MachineID = machineID.String
		r.SessionID = sessionID.String
		r.Error = errStr.String
		r.Metadata = metadata.String
		results = append(results, r)
	}
	return results, total, rows.Err()
}

// Stats returns aggregated log counts grouped by level and component
// for records since the given time.
func (ls *LogStore) Stats(since time.Time) (LogStats, error) {
	stats := LogStats{
		ByLevel:     make(map[string]int),
		ByComponent: make(map[string]int),
	}
	sinceStr := since.UTC().Format(time.RFC3339Nano)

	rows, err := ls.reader.Query("SELECT level, COUNT(*) FROM server_logs WHERE timestamp >= ? GROUP BY level", sinceStr)
	if err != nil {
		return stats, fmt.Errorf("level stats: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var level string
		var count int
		if err := rows.Scan(&level, &count); err != nil {
			return stats, err
		}
		stats.ByLevel[level] = count
		stats.Total += count
	}
	if err := rows.Err(); err != nil {
		return stats, err
	}

	rows2, err := ls.reader.Query("SELECT component, COUNT(*) FROM server_logs WHERE timestamp >= ? GROUP BY component", sinceStr)
	if err != nil {
		return stats, fmt.Errorf("component stats: %w", err)
	}
	defer rows2.Close()
	for rows2.Next() {
		var comp string
		var count int
		if err := rows2.Scan(&comp, &count); err != nil {
			return stats, err
		}
		stats.ByComponent[comp] = count
	}
	return stats, rows2.Err()
}

// PurgeBefore deletes log records older than the given time in batches
// to avoid long-running transactions. Returns total rows deleted.
func (ls *LogStore) PurgeBefore(t time.Time) (int64, error) {
	cutoff := t.UTC().Format(time.RFC3339Nano)
	var totalDeleted int64
	for {
		result, err := ls.writer.Exec("DELETE FROM server_logs WHERE rowid IN (SELECT rowid FROM server_logs WHERE timestamp < ? LIMIT 10000)", cutoff)
		if err != nil {
			return totalDeleted, fmt.Errorf("purge batch: %w", err)
		}
		n, _ := result.RowsAffected()
		totalDeleted += n
		if n < 10000 {
			break
		}
	}
	if totalDeleted > 10000 {
		ls.writer.Exec("PRAGMA incremental_vacuum")
	}
	return totalDeleted, nil
}

// Close closes both writer and reader database connections.
func (ls *LogStore) Close() error {
	wErr := ls.writer.Close()
	rErr := ls.reader.Close()
	if wErr != nil {
		return wErr
	}
	return rErr
}
