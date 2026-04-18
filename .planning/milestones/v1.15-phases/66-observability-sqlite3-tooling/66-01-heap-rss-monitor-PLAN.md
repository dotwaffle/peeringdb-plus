---
phase: 66-observability-sqlite3-tooling
plan: 01
type: execute
wave: 1
depends_on: []
files_modified:
  - internal/config/config.go
  - internal/config/config_test.go
  - internal/sync/worker.go
  - internal/sync/worker_test.go
  - cmd/peeringdb-plus/main.go
autonomous: true
requirements: [OBS-05]
tags: [observability, config, sync, otel]

must_haves:
  truths:
    - "Operators can set PDBPLUS_HEAP_WARN_MIB (default 400) and PDBPLUS_RSS_WARN_MIB (default 384) to tune threshold"
    - "At end of every sync cycle, runtime.MemStats.HeapInuse is read and converted to MiB"
    - "On Linux, /proc/self/status VmHWM is parsed to MiB; on non-Linux the RSS read is skipped cleanly"
    - "OTel span attrs pdbplus.sync.peak_heap_mib and pdbplus.sync.peak_rss_mib are attached to the sync-cycle span"
    - "slog.Warn(\"heap threshold crossed\", ...) fires every cycle the heap OR RSS crosses its configured threshold"
    - "Setting either env var to 0 disables the corresponding threshold warn (attrs are still emitted)"
  artifacts:
    - path: "internal/config/config.go"
      provides: "HeapWarnBytes + RSSWarnBytes int64 config fields + parseMiB helper + validate() bounds checks"
      contains: "HeapWarnBytes"
    - path: "internal/config/config_test.go"
      provides: "Table-driven tests for parseMiB (valid, default, invalid non-numeric, negative, zero-disable)"
      contains: "TestLoad_HeapWarnMiB_Parse"
    - path: "internal/sync/worker.go"
      provides: "emitMemoryTelemetry helper + /proc/self/status VmHWM parser + call site in recordSuccess (and failure paths)"
      contains: "emitMemoryTelemetry"
    - path: "internal/sync/worker_test.go"
      provides: "Unit tests: attrs emitted on every sync, slog.Warn fires on heap/RSS threshold cross, no warn when below"
      contains: "TestEmitMemoryTelemetry"
    - path: "cmd/peeringdb-plus/main.go"
      provides: "Wire cfg.HeapWarnBytes/RSSWarnBytes into WorkerConfig"
      contains: "HeapWarnBytes:"
  key_links:
    - from: "internal/config/config.go"
      to: "internal/sync/worker.go"
      via: "WorkerConfig.HeapWarnBytes / RSSWarnBytes plumbed from cmd/peeringdb-plus/main.go"
      pattern: "HeapWarnBytes"
    - from: "internal/sync/worker.go recordSuccess"
      to: "OTel span + slog.Warn"
      via: "span.SetAttributes(attribute.Int64(\"pdbplus.sync.peak_heap_mib\", ...)) + w.logger.LogAttrs(ctx, slog.LevelWarn, \"heap threshold crossed\", ...)"
      pattern: "pdbplus\\.sync\\.peak_heap_mib"
---

<objective>
Implement end-of-sync-cycle peak heap + RSS observability per Phase 66 CONTEXT D-02, D-03, D-09, D-10.

Purpose: Make SEED-001's "peak heap >threshold" trigger observable. Both OTel span attrs (for dashboards / timeseries) AND slog.Warn (for log-pipeline alerting) — operators choose their signal path. Does NOT flip SEED-001; just observes its trigger.

Output:
- Two new env vars (HEAP_WARN_MIB default 400, RSS_WARN_MIB default 384) parsed into int64 byte counts
- One new span attribute pair on every sync-cycle span
- One new slog.Warn line when either threshold is breached
- Linux-only RSS read via /proc/self/status VmHWM with graceful fallback on other OSes
</objective>

<execution_context>
@$HOME/.claude/get-shit-done/workflows/execute-plan.md
@$HOME/.claude/get-shit-done/templates/summary.md
</execution_context>

<context>
@.planning/PROJECT.md
@.planning/ROADMAP.md
@.planning/STATE.md
@.planning/phases/66-observability-sqlite3-tooling/66-CONTEXT.md
@.planning/seeds/SEED-001-incremental-sync-evaluation.md
@CLAUDE.md

<interfaces>
<!-- Key types the executor needs. Extracted from the codebase so the executor does not need to scavenger-hunt. -->

Existing config parser pattern (internal/config/config.go, Phase 59 parsePublicTier is the template for this phase):
```go
// parsePublicTier reads key from the environment, maps its value to a privctx.Tier.
// Empty / unset -> defaultVal. Invalid value -> hard error (fail-fast per GO-CFG-1).
func parsePublicTier(key string, defaultVal privctx.Tier) (privctx.Tier, error) {
    v := os.Getenv(key)
    switch v {
    case "": return defaultVal, nil
    case "public": return privctx.TierPublic, nil
    case "users": return privctx.TierUsers, nil
    default: return 0, fmt.Errorf("invalid value %q for %s: must be 'public' or 'users'", v, key)
    }
}
```

Existing Config struct fields to mirror (internal/config/config.go ~line 53-131):
```go
type Config struct {
    // ... existing fields ...
    SyncMemoryLimit int64 // parsed via parseByteSize, validated non-negative
}
```

Existing WorkerConfig (internal/sync/worker.go line 44-64):
```go
type WorkerConfig struct {
    IncludeDeleted  bool
    IsPrimary       func() bool
    SyncMode        config.SyncMode
    OnSyncComplete  func(counts map[string]int, syncTime time.Time)
    SyncMemoryLimit int64
}
```

Existing sync-cycle span lifecycle (internal/sync/worker.go line 267-268):
```go
ctx, span := otel.Tracer("sync").Start(ctx, "sync-"+string(mode))
defer span.End()
```
This is the span that OBS-05 attributes attach to. It is in scope throughout recordSuccess, rollbackAndRecord, and recordFailure.

Existing OTel attribute pattern (Phase 61 pdbplus.privacy.tier precedent):
```go
import "go.opentelemetry.io/otel/attribute"
import "go.opentelemetry.io/otel/trace"

trace.SpanFromContext(ctx).SetAttributes(
    attribute.Int64("pdbplus.sync.peak_heap_mib", heapMiB),
    attribute.Int64("pdbplus.sync.peak_rss_mib", rssMiB),
)
```

Existing slog pattern (typed attrs per GO-OBS-5):
```go
w.logger.LogAttrs(ctx, slog.LevelWarn, "heap threshold crossed",
    slog.Int64("peak_heap_mib", heapMiB),
    slog.Int64("heap_warn_mib", heapWarnMiB),
    slog.Int64("peak_rss_mib", rssMiB),
    slog.Int64("rss_warn_mib", rssWarnMiB),
)
```

Call sites in worker.go where the sync-cycle span is still live and mem sampling should happen:
- recordSuccess (line 420) — emit attrs + warn here on success
- rollbackAndRecord (line 408) — emit attrs + warn here on rollback (optional but cheap — heap is interesting regardless of sync outcome)
- recordFailure — emit attrs on failure too

Do NOT add a periodic background sampler (D-09 forbids it). Sync cycle frequency is the sampling granularity.
</interfaces>
</context>

<tasks>

<task type="auto" tdd="true">
  <name>Task 1: Add HeapWarnBytes + RSSWarnBytes to Config with parseMiB helper and table-driven tests</name>
  <files>internal/config/config.go, internal/config/config_test.go</files>
  <read_first>
    - internal/config/config.go (read the entire file — Config struct, Load(), parseByteSize, parsePublicTier are the reference patterns)
    - internal/config/config_test.go lines 580-650 (read TestLoad_SyncMemoryLimit_Parse and TestLoad_PublicTierDefault — copy the table-driven shape verbatim)
  </read_first>
  <behavior>
    - parseMiB("PDBPLUS_HEAP_WARN_MIB", 400) on unset env returns 400*1024*1024 bytes, no error
    - parseMiB on "0" returns 0 with no error (explicit disable)
    - parseMiB on "300" returns 300*1024*1024 bytes with no error (bare integer IS accepted here — unlike parseByteSize, the unit is fixed by the variable name)
    - parseMiB on "-5" returns error mentioning the env var name
    - parseMiB on "abc" returns error mentioning the env var name
    - parseMiB on "100MB" returns error (suffixes NOT accepted — MiB is implied)
    - Load() populates Config.HeapWarnBytes and Config.RSSWarnBytes with correct defaults (400 MiB / 384 MiB) when env vars unset
    - validate() rejects negative values for either field (matches existing SyncMemoryLimit validation)
  </behavior>
  <action>
    1. In internal/config/config.go add two new exported fields to Config struct (place AFTER SyncMemoryLimit, before the closing brace):

       ```go
       // HeapWarnBytes is the peak Go heap threshold (bytes) above which the
       // sync worker emits slog.Warn("heap threshold crossed", ...) at the
       // end of each sync cycle. The OTel span attribute
       // pdbplus.sync.peak_heap_mib is emitted regardless; only the Warn is
       // threshold-gated. Configured via PDBPLUS_HEAP_WARN_MIB (integer MiB,
       // no unit suffix). Default 400 MiB matches the Fly 512 MB VM cap minus
       // a 112 MB safety margin (D-04). Zero disables the warn (attr still
       // emitted so dashboards retain timeseries).
       //
       // SEED-001: sustained breach is the escalation signal for considering
       // PDBPLUS_SYNC_MODE=incremental.
       HeapWarnBytes int64

       // RSSWarnBytes is the peak OS RSS threshold (bytes) above which the
       // sync worker emits slog.Warn at the end of each sync cycle. Read from
       // /proc/self/status VmHWM on Linux; skipped with a one-time
       // slog.Info at startup on non-Linux (no /proc). Configured via
       // PDBPLUS_RSS_WARN_MIB (integer MiB, no unit suffix). Default 384 MiB.
       // Zero disables the warn. The threshold is lower than HeapWarnBytes
       // because RSS measures committed pages (heap + stack + binary), which
       // is a stricter bound than runtime.MemStats.HeapInuse (D-03).
       RSSWarnBytes int64
       ```

    2. Add a new helper parseMiB ABOVE parseByteSize (so file reads top-down: trivial units first, byte-size parser last):

       ```go
       // parseMiB parses an env var as a non-negative integer count of MiB
       // (mebibytes; 1 MiB = 1024*1024 bytes). Returns the value in BYTES
       // (MiB * 1024 * 1024) so callers can compare directly against
       // runtime.MemStats fields.
       //
       // Unlike parseByteSize, no unit suffix is accepted — the variable
       // name encodes the unit (PDBPLUS_HEAP_WARN_MIB, PDBPLUS_RSS_WARN_MIB).
       // Operators who attempt "400MB" get a clear error rather than silent
       // coercion.
       //
       // Accepted: "", "0", "400", "16" (bare non-negative integers).
       // Rejected: "-5", "abc", "400MB", "1.5" — all fail-fast.
       func parseMiB(key string, defaultMiB int64) (int64, error) {
           v := os.Getenv(key)
           if v == "" {
               return defaultMiB * 1024 * 1024, nil
           }
           mib, err := strconv.ParseInt(v, 10, 64)
           if err != nil {
               return 0, fmt.Errorf("invalid MiB value %q for %s: must be a non-negative integer (no unit suffix)", v, key)
           }
           if mib < 0 {
               return 0, fmt.Errorf("invalid MiB value %q for %s: must be non-negative", v, key)
           }
           // Overflow guard mirrors parseByteSize.
           const bytesPerMiB = int64(1024 * 1024)
           if mib > math.MaxInt64/bytesPerMiB {
               return 0, fmt.Errorf("invalid MiB value %q for %s: overflows int64", v, key)
           }
           return mib * bytesPerMiB, nil
       }
       ```

    3. In Load() add parsing for both env vars (place AFTER the existing SyncMemoryLimit block so the order in Load mirrors the struct order):

       ```go
       heapWarn, err := parseMiB("PDBPLUS_HEAP_WARN_MIB", 400)
       if err != nil {
           return nil, fmt.Errorf("parsing PDBPLUS_HEAP_WARN_MIB: %w", err)
       }
       cfg.HeapWarnBytes = heapWarn

       rssWarn, err := parseMiB("PDBPLUS_RSS_WARN_MIB", 384)
       if err != nil {
           return nil, fmt.Errorf("parsing PDBPLUS_RSS_WARN_MIB: %w", err)
       }
       cfg.RSSWarnBytes = rssWarn
       ```

    4. In validate() add two non-negative checks after the existing SyncMemoryLimit check (mirror that line verbatim):

       ```go
       if c.HeapWarnBytes < 0 {
           return errors.New("PDBPLUS_HEAP_WARN_MIB must be non-negative (0 = disabled)")
       }
       if c.RSSWarnBytes < 0 {
           return errors.New("PDBPLUS_RSS_WARN_MIB must be non-negative (0 = disabled)")
       }
       ```

    5. In internal/config/config_test.go append a table-driven test TestLoad_HeapWarnMiB_Parse copying the shape of TestLoad_SyncMemoryLimit_Parse (lines 581-631). Cases:

       - name "default_unset", envVal "", want 400*1024*1024
       - name "explicit_zero_disable", envVal "0", want 0
       - name "custom_300", envVal "300", want 300*1024*1024
       - name "custom_500", envVal "500", want 500*1024*1024
       - name "bare_negative_rejected", envVal "-100", wantErr true
       - name "unit_suffix_rejected", envVal "400MB", wantErr true
       - name "non_numeric_rejected", envVal "abc", wantErr true
       - name "float_rejected", envVal "1.5", wantErr true

    6. Add an identical TestLoad_RSSWarnMiB_Parse test (same table, envVar PDBPLUS_RSS_WARN_MIB, default 384*1024*1024). Keep the two tests separate rather than DRY-ing — matches the one-test-per-env-var house style.

    7. Run go generate ./ent (NOT ./...) only if the executor's editor touched any ent-schema-adjacent file; for this task it should not be necessary. Mentioned defensively per v1.15 Phase 63 CLAUDE.md note.

    Traceability: implements OBS-05 config substrate per D-03/D-04.
  </action>
  <acceptance_criteria>
    - `grep -n 'HeapWarnBytes\s*int64' internal/config/config.go` returns a match
    - `grep -n 'RSSWarnBytes\s*int64' internal/config/config.go` returns a match
    - `grep -n 'func parseMiB' internal/config/config.go` returns a match
    - `grep -n 'PDBPLUS_HEAP_WARN_MIB' internal/config/config.go` returns at least two matches (Load + error message)
    - `grep -n 'TestLoad_HeapWarnMiB_Parse\|TestLoad_RSSWarnMiB_Parse' internal/config/config_test.go` returns two matches
    - `TMPDIR=/tmp/claude-1000 go test -race ./internal/config/...` passes
    - `TMPDIR=/tmp/claude-1000 go vet ./internal/config/...` clean
  </acceptance_criteria>
  <verify>
    <automated>TMPDIR=/tmp/claude-1000 go test -race -run 'TestLoad_HeapWarnMiB_Parse|TestLoad_RSSWarnMiB_Parse' ./internal/config/...</automated>
  </verify>
  <done>Config.HeapWarnBytes and Config.RSSWarnBytes are populated from env vars with documented defaults; parseMiB rejects unit suffixes and negatives; tests cover all happy + sad paths; go vet clean.</done>
</task>

<task type="auto" tdd="true">
  <name>Task 2: Emit heap/RSS telemetry at end-of-sync-cycle in worker.go + wire WorkerConfig</name>
  <files>internal/sync/worker.go, internal/sync/worker_test.go, cmd/peeringdb-plus/main.go</files>
  <read_first>
    - internal/sync/worker.go lines 44-117 (WorkerConfig + NewWorker)
    - internal/sync/worker.go lines 260-470 (Sync, checkMemoryLimit, rollbackAndRecord, recordSuccess — the span lifecycle + call sites)
    - internal/sync/worker.go lines 380-402 (checkMemoryLimit — reference for the heap-clamp int64 cast + MaxInt64 guard)
    - cmd/peeringdb-plus/main.go lines 220-245 (NewWorker call site with WorkerConfig literal)
    - internal/sync/worker_test.go (top of file — import pattern and Worker construction pattern for tests)
  </read_first>
  <behavior>
    - emitMemoryTelemetry(ctx, heapWarnBytes, rssWarnBytes) reads runtime.MemStats.HeapInuse, parses /proc/self/status VmHWM (Linux only), attaches OTel span attrs pdbplus.sync.peak_heap_mib + pdbplus.sync.peak_rss_mib (if available), and fires slog.Warn when either value exceeds its configured threshold
    - On non-Linux (no /proc/self/status), only the heap attr fires; RSS attr is omitted entirely (NOT set to 0 — 0 is a valid metric value and would lie on dashboards)
    - When heapWarnBytes == 0 the heap attr still fires but the warn does NOT
    - When rssWarnBytes == 0 likewise
    - The helper is called unconditionally at the end of Sync: recordSuccess path (success) AND rollbackAndRecord path (failure) AND recordFailure path
    - Unit test uses a fake span recorder (tracetest.NewSpanRecorder) to assert attrs and a bytes.Buffer logger to assert the Warn line
  </behavior>
  <action>
    1. Add to WorkerConfig struct in internal/sync/worker.go (after SyncMemoryLimit, before the closing brace):

       ```go
       // HeapWarnBytes is the peak Go heap threshold (bytes) above which
       // the end-of-sync-cycle emitter fires slog.Warn("heap threshold
       // crossed", ...). The OTel span attr pdbplus.sync.peak_heap_mib is
       // attached regardless. Zero disables only the Warn (not the attr).
       // Wired from config.Config.HeapWarnBytes by main.go.
       //
       // SEED-001 escalation signal: sustained breach triggers the
       // incremental-sync evaluation path documented in
       // .planning/seeds/SEED-001-incremental-sync-evaluation.md.
       HeapWarnBytes int64

       // RSSWarnBytes is the peak OS RSS threshold (bytes) above which
       // the emitter fires slog.Warn. Read from /proc/self/status VmHWM on
       // Linux; skipped on other OSes (the RSS attr is then omitted — it
       // is not set to zero). Zero disables only the Warn.
       RSSWarnBytes int64
       ```

    2. Add a new helper emitMemoryTelemetry BELOW checkMemoryLimit in internal/sync/worker.go:

       ```go
       // emitMemoryTelemetry samples the Go runtime heap and (on Linux) the
       // OS RSS high-water mark at the end of a sync cycle, attaches them as
       // OTel attributes to the current sync-cycle span, and emits
       // slog.Warn("heap threshold crossed") when either value exceeds its
       // configured threshold.
       //
       // Attribute naming follows the pdbplus.* convention established in
       // Phase 61 (pdbplus.privacy.tier): pdbplus.sync.peak_heap_mib and
       // pdbplus.sync.peak_rss_mib. Units are MiB (not bytes) so dashboards
       // can plot them directly without a divisor.
       //
       // On non-Linux the RSS attr is OMITTED entirely — zero is a valid
       // metric value and would produce misleading flat lines on dashboards.
       // The one-time startup slog.Info for this case lives at NewWorker.
       //
       // heapWarnBytes == 0 disables the heap Warn (attr still fires);
       // rssWarnBytes == 0 likewise. Attribute emission is unconditional so
       // dashboards retain timeseries even when alerting is disabled.
       //
       // D-09: sampling frequency is sync cycle frequency (default 1h via
       // PDBPLUS_SYNC_INTERVAL). No periodic background sampler.
       func (w *Worker) emitMemoryTelemetry(ctx context.Context, heapWarnBytes, rssWarnBytes int64) {
           var ms runtime.MemStats
           runtime.ReadMemStats(&ms)
           heapBytes := int64(ms.HeapInuse)
           if ms.HeapInuse > uint64(1<<63-1) {
               heapBytes = int64(1<<63 - 1)
           }
           heapMiB := heapBytes / (1024 * 1024)

           rssBytes, rssOK := readLinuxVmHWM()

           span := trace.SpanFromContext(ctx)
           attrs := []attribute.KeyValue{
               attribute.Int64("pdbplus.sync.peak_heap_mib", heapMiB),
           }
           var rssMiB int64
           if rssOK {
               rssMiB = rssBytes / (1024 * 1024)
               attrs = append(attrs, attribute.Int64("pdbplus.sync.peak_rss_mib", rssMiB))
           }
           span.SetAttributes(attrs...)

           heapOver := heapWarnBytes > 0 && heapBytes > heapWarnBytes
           rssOver := rssOK && rssWarnBytes > 0 && rssBytes > rssWarnBytes
           if !heapOver && !rssOver {
               return
           }
           logAttrs := []slog.Attr{
               slog.Int64("peak_heap_mib", heapMiB),
               slog.Int64("heap_warn_mib", heapWarnBytes/(1024*1024)),
           }
           if rssOK {
               logAttrs = append(logAttrs,
                   slog.Int64("peak_rss_mib", rssMiB),
                   slog.Int64("rss_warn_mib", rssWarnBytes/(1024*1024)),
               )
           }
           logAttrs = append(logAttrs,
               slog.Bool("heap_over", heapOver),
               slog.Bool("rss_over", rssOver),
           )
           w.logger.LogAttrs(ctx, slog.LevelWarn, "heap threshold crossed", logAttrs...)
       }

       // readLinuxVmHWM reads /proc/self/status and returns the VmHWM
       // (peak resident set size high-water mark) in bytes. The second
       // return is false on non-Linux or when the file is absent/unreadable
       // — callers MUST treat this as "RSS not available" rather than zero.
       //
       // VmHWM format: "VmHWM:\t  345216 kB" — tab/space-separated, value
       // in kB (base 1024 on Linux). Multiply by 1024 to get bytes.
       //
       // VmHWM is the peak-RSS high-water mark, not the instantaneous RSS;
       // it only decreases when an operator resets it via
       // `echo 5 > /proc/self/clear_refs` or the process restarts. This is
       // the correct signal for SEED-001 escalation — a single burst is
       // what matters, not the steady-state value.
       func readLinuxVmHWM() (int64, bool) {
           data, err := os.ReadFile("/proc/self/status")
           if err != nil {
               return 0, false
           }
           for _, line := range strings.Split(string(data), "\n") {
               if !strings.HasPrefix(line, "VmHWM:") {
                   continue
               }
               fields := strings.Fields(line)
               if len(fields) < 2 {
                   return 0, false
               }
               kb, parseErr := strconv.ParseInt(fields[1], 10, 64)
               if parseErr != nil {
                   return 0, false
               }
               if kb < 0 || kb > math.MaxInt64/1024 {
                   return 0, false
               }
               return kb * 1024, true
           }
           return 0, false
       }
       ```

    3. Add the required new imports to internal/sync/worker.go: `"math"`, `"os"`, `"strconv"`, `"strings"`. (`runtime`, `log/slog`, `go.opentelemetry.io/otel/attribute`, `go.opentelemetry.io/otel/trace` are already imported.)

    4. Call emitMemoryTelemetry in THREE places (always while the sync-cycle span is still current in ctx — i.e. BEFORE span.End() runs via the defer at line 268):

       a. At the top of recordSuccess (first line after the opening brace): `w.emitMemoryTelemetry(ctx, w.config.HeapWarnBytes, w.config.RSSWarnBytes)`. Add a one-line comment `// OBS-05: emit heap + RSS span attrs and (if over threshold) slog.Warn.`

       b. At the top of rollbackAndRecord (first line after the opening brace, BEFORE the tx.Rollback call): same line + comment. Justification: heap pressure is interesting regardless of sync outcome — a rollback under heap pressure is a meaningful signal.

       c. In recordFailure — find the function (grep `func (w \*Worker) recordFailure`) and add the same call as the first line. The sync-cycle span is still alive throughout recordFailure; the emitter is the right consumer.

    5. In cmd/peeringdb-plus/main.go line 226-242 NewWorker call, append two new WorkerConfig fields after SyncMemoryLimit:

       ```go
       HeapWarnBytes:   cfg.HeapWarnBytes,
       RSSWarnBytes:    cfg.RSSWarnBytes,
       ```

    6. Add unit test TestEmitMemoryTelemetry_Attrs in internal/sync/worker_test.go:

       - Use go.opentelemetry.io/otel/sdk/trace/tracetest (tracetest.NewSpanRecorder) to capture span attrs
       - Construct a minimal Worker with a bytes.Buffer-backed slog.Logger (slog.New(slog.NewJSONHandler(&buf, nil)))
       - Case 1 "below threshold": heapWarnBytes = huge, rssWarnBytes = huge; assert span has pdbplus.sync.peak_heap_mib attr; assert log buffer does NOT contain "heap threshold crossed"
       - Case 2 "heap over": heapWarnBytes = 1; assert span has the attr; assert log buffer DOES contain "heap threshold crossed" AND a heap_over=true attr (assert via json.Unmarshal of the buffered line)
       - Case 3 "both disabled": heapWarnBytes = 0, rssWarnBytes = 0; assert attrs present; assert NO warn
       - Case 4 "rss availability": if runtime.GOOS == "linux" assert rss attr present; else assert rss attr absent (use t.Skip for the else branch rather than build tags — CI is Linux anyway)

       Skeleton:
       ```go
       func TestEmitMemoryTelemetry_Attrs(t *testing.T) {
           tests := []struct { name string; heapWarn, rssWarn int64; wantWarn bool }{
               {"below_threshold", 1 << 40, 1 << 40, false},
               {"heap_over", 1, 1 << 40, true},
               {"both_disabled", 0, 0, false},
           }
           for _, tt := range tests {
               t.Run(tt.name, func(t *testing.T) {
                   var buf bytes.Buffer
                   logger := slog.New(slog.NewJSONHandler(&buf, nil))
                   w := &Worker{logger: logger}
                   sr := tracetest.NewSpanRecorder()
                   tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sr))
                   ctx, span := tp.Tracer("t").Start(context.Background(), "sync-test")
                   w.emitMemoryTelemetry(ctx, tt.heapWarn, tt.rssWarn)
                   span.End()
                   // Assert span attrs
                   spans := sr.Ended()
                   if len(spans) != 1 { t.Fatalf("want 1 span, got %d", len(spans)) }
                   // Extract attrs into a map keyed by attribute.Key
                   // Assert pdbplus.sync.peak_heap_mib exists
                   // If linux, assert pdbplus.sync.peak_rss_mib exists
                   // Assert warn presence matches tt.wantWarn
                   warned := strings.Contains(buf.String(), "heap threshold crossed")
                   if warned != tt.wantWarn { t.Errorf("warn=%v want %v; log=%s", warned, tt.wantWarn, buf.String()) }
               })
           }
       }
       ```

    7. Add a focused test TestReadLinuxVmHWM on Linux (skip on non-Linux):
       - Assert readLinuxVmHWM returns (n > 0, true) on Linux
       - No negative-path test needed (helper is pure — reading /proc/self/status is guaranteed on Linux)

    Traceability: implements OBS-05 runtime surface per D-02/D-09/D-10.
  </action>
  <acceptance_criteria>
    - `grep -n 'HeapWarnBytes\s*int64' internal/sync/worker.go` matches (in WorkerConfig)
    - `grep -n 'func (w \*Worker) emitMemoryTelemetry' internal/sync/worker.go` matches exactly once
    - `grep -n 'func readLinuxVmHWM' internal/sync/worker.go` matches exactly once
    - `grep -n 'pdbplus.sync.peak_heap_mib' internal/sync/worker.go` matches
    - `grep -n 'pdbplus.sync.peak_rss_mib' internal/sync/worker.go` matches
    - `grep -n 'heap threshold crossed' internal/sync/worker.go` matches
    - `grep -n 'w.emitMemoryTelemetry(ctx' internal/sync/worker.go` matches at least 3 times (recordSuccess, rollbackAndRecord, recordFailure)
    - `grep -n 'HeapWarnBytes:\|RSSWarnBytes:' cmd/peeringdb-plus/main.go` returns both
    - `grep -n 'TestEmitMemoryTelemetry_Attrs' internal/sync/worker_test.go` matches
    - `TMPDIR=/tmp/claude-1000 go test -race ./internal/sync/... ./internal/config/...` passes
    - `TMPDIR=/tmp/claude-1000 go build ./...` clean (no drift, no unused imports)
    - `TMPDIR=/tmp/claude-1000 go vet ./...` clean
    - `TMPDIR=/tmp/claude-1000 golangci-lint run ./internal/sync/... ./internal/config/... ./cmd/peeringdb-plus/...` clean
  </acceptance_criteria>
  <verify>
    <automated>TMPDIR=/tmp/claude-1000 go test -race ./internal/sync/... ./internal/config/... && TMPDIR=/tmp/claude-1000 go vet ./... && TMPDIR=/tmp/claude-1000 golangci-lint run ./internal/sync/... ./internal/config/... ./cmd/peeringdb-plus/...</automated>
  </verify>
  <done>emitMemoryTelemetry is wired into all three sync-terminal paths; OTel span carries pdbplus.sync.peak_heap_mib on every cycle + pdbplus.sync.peak_rss_mib on Linux; slog.Warn fires when either threshold is breached; unit tests assert both branches; main.go wires the config values; race tests + lint + vet + build all clean.</done>
</task>

</tasks>

<threat_model>
## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| operator config → process | PDBPLUS_HEAP_WARN_MIB / PDBPLUS_RSS_WARN_MIB parsed at startup; trusted input under GO-CFG-1 fail-fast. Not a runtime attack surface. |
| /proc/self filesystem → process | Own-process /proc read; not a crossing — same UID, same PID. Informational only. |

## STRIDE Threat Register

| Threat ID | Category | Component | Disposition | Mitigation Plan |
|-----------|----------|-----------|-------------|-----------------|
| T-66-01 | Information-disclosure | slog.Warn output may surface memory numbers | accept | Heap + RSS in MiB is not sensitive; published via OTel already (existing Go runtime metrics). No PII. |
| T-66-02 | Denial-of-service | Malicious config (huge MiB value) disables warn | accept | Threshold set higher than reality just skips the warn; OTel attrs still fire — dashboards catch it. Requires local env-var access (root-equivalent) to exploit. |
| T-66-03 | Tampering | Non-integer MiB value triggers startup crash | mitigate | parseMiB returns fail-fast error with env-var-named message per GO-CFG-1; config.Load() wraps and returns before sync worker starts. Validate() adds defense-in-depth for negative values. |
| T-66-04 | Spoofing | /proc/self/status contents cannot be spoofed from outside the process | accept | Same-UID read; kernel guarantees correctness. Non-Linux OS degrades gracefully (attr omitted). |
</threat_model>

<verification>
**Automated gates (enforced in CI):**
- `TMPDIR=/tmp/claude-1000 go test -race ./internal/config/... ./internal/sync/...` passes
- `TMPDIR=/tmp/claude-1000 go build ./...` clean
- `TMPDIR=/tmp/claude-1000 golangci-lint run` clean
- `TMPDIR=/tmp/claude-1000 go vet ./...` clean
- `TMPDIR=/tmp/claude-1000 govulncheck ./...` clean

**Behavioral checks (grep-verifiable):**
- Config.HeapWarnBytes + Config.RSSWarnBytes fields exist
- parseMiB rejects unit suffixes and negatives
- WorkerConfig.HeapWarnBytes + WorkerConfig.RSSWarnBytes fields exist
- emitMemoryTelemetry called in recordSuccess, rollbackAndRecord, recordFailure (three sites)
- cmd/peeringdb-plus/main.go wires both fields into WorkerConfig
</verification>

<success_criteria>
- OBS-05 satisfied: OTel span attrs emitted every sync cycle; slog.Warn fires on threshold cross; both env vars parse, validate, and wire end-to-end
- No regression in existing tests (sync integration, config tests)
- All new tests race-clean
</success_criteria>

<output>
After completion, create `.planning/phases/66-observability-sqlite3-tooling/66-01-SUMMARY.md`.
</output>
