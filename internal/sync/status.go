package sync

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// Status represents the result of a sync operation.
type Status struct {
	LastSyncAt   time.Time
	Duration     time.Duration
	ObjectCounts map[string]int // type -> count
	Status       string         // "success", "failed", "running"
	ErrorMessage string         // empty on success
}

// InitStatusTable creates the sync_status table if it doesn't exist.
// This is not an ent-managed entity; it stores operational metadata via raw SQL.
func InitStatusTable(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS sync_status (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			started_at DATETIME NOT NULL,
			completed_at DATETIME,
			duration_ms INTEGER,
			object_counts TEXT,
			status TEXT NOT NULL DEFAULT 'running',
			error_message TEXT DEFAULT ''
		)
	`)
	if err != nil {
		return fmt.Errorf("create sync_status table: %w", err)
	}
	return nil
}

// RecordSyncStart inserts a new running sync status row and returns its ID.
func RecordSyncStart(ctx context.Context, db *sql.DB, startedAt time.Time) (int64, error) {
	result, err := db.ExecContext(ctx,
		`INSERT INTO sync_status (started_at, status) VALUES (?, 'running')`,
		startedAt,
	)
	if err != nil {
		return 0, fmt.Errorf("record sync start: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("get sync status id: %w", err)
	}
	return id, nil
}

// RecordSyncComplete updates the sync status row with results.
func RecordSyncComplete(ctx context.Context, db *sql.DB, id int64, status Status) error {
	countsJSON, err := json.Marshal(status.ObjectCounts)
	if err != nil {
		return fmt.Errorf("marshal object counts: %w", err)
	}
	_, err = db.ExecContext(ctx,
		`UPDATE sync_status SET completed_at = ?, duration_ms = ?, object_counts = ?, status = ?, error_message = ? WHERE id = ?`,
		status.LastSyncAt,
		status.Duration.Milliseconds(),
		string(countsJSON),
		status.Status,
		status.ErrorMessage,
		id,
	)
	if err != nil {
		return fmt.Errorf("record sync complete: %w", err)
	}
	return nil
}

// GetLastStatus returns the most recent sync status.
// Returns nil if no sync has been recorded.
func GetLastStatus(ctx context.Context, db *sql.DB) (*Status, error) {
	row := db.QueryRowContext(ctx,
		`SELECT started_at, completed_at, duration_ms, object_counts, status, error_message FROM sync_status ORDER BY id DESC LIMIT 1`,
	)

	var (
		startedAt    time.Time
		completedAt  sql.NullTime
		durationMs   sql.NullInt64
		countsStr    sql.NullString
		statusStr    string
		errorMessage sql.NullString
	)
	err := row.Scan(&startedAt, &completedAt, &durationMs, &countsStr, &statusStr, &errorMessage)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get last sync status: %w", err)
	}

	s := &Status{
		LastSyncAt: startedAt,
		Status:     statusStr,
	}
	if completedAt.Valid {
		s.LastSyncAt = completedAt.Time
	}
	if durationMs.Valid {
		s.Duration = time.Duration(durationMs.Int64) * time.Millisecond
	}
	if countsStr.Valid && countsStr.String != "" {
		s.ObjectCounts = make(map[string]int)
		if err := json.Unmarshal([]byte(countsStr.String), &s.ObjectCounts); err != nil {
			return nil, fmt.Errorf("unmarshal object counts: %w", err)
		}
	}
	if errorMessage.Valid {
		s.ErrorMessage = errorMessage.String
	}
	return s, nil
}
