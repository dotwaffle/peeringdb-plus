package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// DataVersionProbe reports a key that changes each time another connection
// commits a change to the database. It is the change signal for nodes that
// run without LiteFS, where no "-pos" file exists.
//
// SQLite's PRAGMA data_version is a property of one connection: its value
// changes when a different connection commits, and it ignores commits made
// through the same connection. The probe therefore pins one connection from
// the pool and runs only the pragma on it. It never writes, so every commit
// in the process or in another process shows up as a change.
//
// The pinned connection holds 1 of the pool's 10 slots for the life of the
// probe. database/sql does not close a held connection when
// ConnMaxLifetime passes, so the pin stays stable. When a query fails, the
// probe releases the connection and pins a new one on the next call; the
// key then includes a new epoch, because data_version values from two
// connections cannot be compared.
//
// A caller polls the probe outside any request, so on a traced handle each
// query would start its own root trace. Open's span filter drops the span
// of the probe query; otelsql still records the query in its metrics.
//
// A DataVersionProbe is not safe for concurrent use. Call Version and Close
// from one goroutine.
type DataVersionProbe struct {
	db    *sql.DB
	conn  *sql.Conn
	boot  int64
	epoch uint64
}

// dataVersionQuery is the only statement that a DataVersionProbe runs.
// keepQuerySpan matches it by text.
const dataVersionQuery = "PRAGMA data_version"

// NewDataVersionProbe returns a probe for db. It pins no connection until
// the first call to Version.
func NewDataVersionProbe(db *sql.DB) *DataVersionProbe {
	return &DataVersionProbe{db: db, boot: time.Now().UnixNano()}
}

// Version returns the current change key. Two calls return the same key
// only if no other connection committed a change between them. The boot
// time and the epoch keep keys unique across restarts and re-pins.
func (p *DataVersionProbe) Version(ctx context.Context) (string, error) {
	if p.conn == nil {
		conn, err := p.db.Conn(ctx)
		if err != nil {
			return "", fmt.Errorf("pin data_version connection: %w", err)
		}
		p.conn = conn
		p.epoch++
	}
	var v int64
	if err := p.conn.QueryRowContext(ctx, dataVersionQuery).Scan(&v); err != nil {
		_ = p.Close() // The query error is the one to report.
		return "", fmt.Errorf("read data_version: %w", err)
	}
	return fmt.Sprintf("sqlite/%d/%d/%d", p.boot, p.epoch, v), nil
}

// Close releases the pinned connection. A later call to Version pins a new
// connection and returns a key that differs from every earlier key.
func (p *DataVersionProbe) Close() error {
	if p.conn == nil {
		return nil
	}
	err := p.conn.Close()
	p.conn = nil
	if err != nil {
		return fmt.Errorf("release data_version connection: %w", err)
	}
	return nil
}
