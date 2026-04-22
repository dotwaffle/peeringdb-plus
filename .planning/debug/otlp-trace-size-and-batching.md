---
status: investigating
trigger: "OTLP spans rejected (max trace size) and batching inquiry"
created: 2026-04-22
updated: 2026-04-22
---

# Symptoms

- **Expected behavior**: OTLP spans should be accepted by the collector. Batching should be enabled (e.g., 5s intervals or specific batch size) to improve efficiency.
- **Actual behavior**: Some OTLP spans are rejected because the max trace size is too large. The current batching configuration (or presence thereof) is unknown.
- **Error messages**: Info level log at 15:12:01 today: "rejected as the max trace size is too large".
- **Timeline**: Started Wednesday, April 22, 2026.
- **Reproduction**: Likely triggered during large synchronization actions with PeeringDB.

# Current Focus

- **hypothesis**: Current synchronization logic creates trace spans that exceed the maximum size allowed by the OTLP collector.
- **test**: Inspect the telemetry/OTLP configuration and sync logic to identify span creation patterns.
- **expecting**: Find that sync actions are not sufficiently broken down or that batching/size limits are not explicitly configured.
- **next_action**: "apply fix to reduce span bloat and improve batcher configuration"
- **reasoning_checkpoint**: Identified that `netixlan` sync creates ~1600 spans due to pagination, exceeding typical 2000-span trace limits.

# Evidence

- **timestamp**: 2026-04-22T15:30:00Z
  **observation**: `internal/peeringdb/client.go` uses `otelhttp` and starts a `peeringdb.request` span for every page.
- **timestamp**: 2026-04-22T15:31:00Z
  **observation**: `netixlan` fetch involves ~800 pages (250 items each for 200k items), resulting in ~1600 spans in a single trace.
- **timestamp**: 2026-04-22T15:32:00Z
  **observation**: Many OTLP backends (like Honeycomb) have a default limit of 2000 spans per trace.

# Eliminated Hypotheses

# Resolution

- **root_cause**: Synchronization of large entity types (especially `netixlan`) generates too many spans due to per-page HTTP instrumentation and explicit request spans, exceeding OTLP collector trace limits.
- **fix**: Remove redundant `peeringdb.request` spans and `otelhttp` transport from the internal PeeringDB client; move retry/wait events to the parent `peeringdb.stream` span. Explicitly configure OTel batcher settings in `internal/otel/provider.go`.
- **verification**: 
- **files_changed**: 
