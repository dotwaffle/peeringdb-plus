---
status: resolved
trigger: "OTLP spans rejected (max trace size) and batching inquiry"
created: 2026-04-22
updated: 2026-04-22
---

# Symptoms

- **Expected behavior**: OTLP spans should be accepted by the collector. Batching should be enabled (e.g., 5s intervals or specific batch size) to improve efficiency.
- **Actual behavior**: Some OTLP spans are rejected because the max trace size is too large. The current batching configuration (or presence thereof) is unknown.
- **Error messages**: Info level log at 15:12:01 today: "rejected as the max trace size is too large".
- **Timeline**: Started Wednesday, April 22, 2026. Fixed in commit f7da22d.
- **Reproduction**: Triggered during large synchronization actions with PeeringDB (especially netixlan).

# Current Focus

- **hypothesis**: Current synchronization logic creates trace spans that exceed the maximum size allowed by the OTLP collector.
- **test**: Inspect the telemetry/OTLP configuration and sync logic to identify span creation patterns.
- **expecting**: Find that sync actions are not sufficiently broken down or that batching/size limits are not explicitly configured.
- **next_action**: "closed"
- **reasoning_checkpoint**: Identified that `netixlan` sync creates ~1600 spans due to pagination, exceeding typical 2000-span trace limits.

# Evidence

- **timestamp**: 2026-04-22T15:30:00Z
  **observation**: `internal/peeringdb/client.go` used `otelhttp` and started a `peeringdb.request` span for every page.
- **timestamp**: 2026-04-22T15:31:00Z
  **observation**: `netixlan` fetch involves ~800 pages (250 items each for 200k items), resulting in ~1600 spans in a single trace.
- **timestamp**: 2026-04-22T15:32:00Z
  **observation**: Many OTLP backends (like Honeycomb) have a default limit of 2000 spans per trace.

# Eliminated Hypotheses

# Resolution

- **root_cause**: Synchronization of large entity types (especially `netixlan`) generated too many spans due to per-page HTTP instrumentation and explicit request spans, exceeding OTLP collector trace limits.
- **fix**: Removed redundant `peeringdb.request` spans and `otelhttp` transport from the internal PeeringDB client in commit f7da22d; moved retry/wait events to the parent `peeringdb.stream` span. Explicitly configured OTel batcher settings in `internal/otel/provider.go`.
- **verification**: `TestFetchAll_OmitsPerRequestSpans` in `internal/peeringdb/client_test.go` asserts zero `peeringdb.request` spans are produced. Full sync cycle verified against OTLP collector.
- **files_changed**: `internal/peeringdb/client.go`, `internal/peeringdb/stream.go`, `internal/otel/provider.go`
