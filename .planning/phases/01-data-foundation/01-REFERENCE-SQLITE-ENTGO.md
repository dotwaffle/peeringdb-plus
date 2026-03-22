# Reference: modernc.org/sqlite + entgo Integration

**Source:** User-provided code snippets (2026-03-22)
**Phase:** 01 — Data Foundation
**Why this exists:** modernc.org/sqlite does not work with entgo out of the box. These snippets are required for correct integration.

## 1. Memory Limit (Fly.io)

Import `github.com/KimMachineGun/automemlimit/memlimit` and call in the `main` package's `init()`:

```go
func init() {
    memlimit.SetGoMemLimitWithOpts(
        memlimit.WithProvider(
            memlimit.ApplyFallback(
                memlimit.FromCgroup,
                memlimit.FromSystem,
            ),
        ),
    )
}
```

This helps significantly when running in Fly.io's cgroup-limited environments.

## 2. Driver Registration

entgo expects the driver name `"sqlite3"`, not `"sqlite"`. Register the modernc driver under the expected name in `init()`:

```go
import "modernc.org/sqlite"

func init() {
    sql.Register("sqlite3", &sqlite.Driver{})
}
```

## 3. Production Client Setup (main)

```go
import (
    "database/sql"
    "fmt"

    "entgo.io/ent/dialect"
    entsql "entgo.io/ent/dialect/sql"
    _ "modernc.org/sqlite"
)

func main() {
    dsn := fmt.Sprintf("file:%s?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)", dbPath)
    db, err := sql.Open("sqlite", dsn)
    if err != nil {
        return fmt.Errorf("opening database: %w", err)
    }
    entClient := ent.NewClient(ent.Driver(entsql.OpenDB(dialect.SQLite, db)))
    defer entClient.Close()
}
```

Key pragmas:
- `foreign_keys(1)` — enforce FK constraints
- `journal_mode(WAL)` — enable WAL for concurrent reads during sync
- `busy_timeout(5000)` — wait up to 5s on lock contention

## 4. Test Setup

Common test helper using in-memory SQLite:

```go
import (
    "database/sql"
    "log/slog"
    "os"
    "testing"

    "entgo.io/ent/dialect"
    "entgo.io/ent/enttest"
    "modernc.org/sqlite"
)

func init() {
    sql.Register("sqlite3", &sqlite.Driver{})
}

func setup(t *testing.T) (*api.Server, *ent.Client) {
    t.Helper()
    client := enttest.Open(t, dialect.SQLite, "file:ent?mode=memory&cache=shared&_pragma=foreign_keys(1)")
    t.Cleanup(func() { client.Close() })
    logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
    return api.New(client, logger, nil), client
}
```

## 5. Required Imports Summary

```go
import (
    "database/sql"

    "entgo.io/ent/dialect"
    entsql "entgo.io/ent/dialect/sql"
    "modernc.org/sqlite"
)
```

## Key Gotchas

1. **Driver name mismatch:** `sql.Open` uses `"sqlite"` but entgo expects `"sqlite3"` — must register manually
2. **Pragma syntax:** Uses `_pragma=key(value)` in DSN query params, not `PRAGMA` SQL statements
3. **Test DSN:** In-memory with `cache=shared` so multiple connections see the same data
4. **automemlimit:** Essential for Fly.io — without it, Go's GC doesn't know the cgroup memory limit
