# Changelog

All notable changes to this project are documented in this file.

The format follows [Keep a Changelog 1.1.0](https://keepachangelog.com/en/1.1.0/)
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

Release notes for v1.0 through v1.15 and for v1.17.0 through v1.18.14
are in the Git history at their tags.

## [Unreleased]

### Added

- Alert rule `PdbPlusReplicaMemoryHigh` (warning): the Go runtime
  memory of a replica above 180 MiB for 15 minutes. The heap and RSS
  rules read gauges that only the primary sets, so the 256 MB replicas
  had no memory alert.
- SLO definitions in `deploy/grafana/slos/` for the Grafana SLO app.
  Availability: 99.9% of routed HTTP requests over 28 days return a
  status below 500 (health probes, `POST /sync` and unrouted requests
  left out), with fast and slow burn-rate alerts that need at least 5
  failed requests. Data freshness: the newest successful sync is less
  than 1 hour old for 99.5% of 28 days (no SLO alerts;
  `PdbPlusSyncFreshnessHigh` stays). Dashboard panel Error Rate (5xx)
  uses the request selector of the availability SLO.
- Synthetic Monitoring check definition in
  `deploy/grafana/synthetics/README.md`: an HTTP check of `/api` through
  the Fly proxy from London, Sydney and the US every 3 minutes (43,200
  executions per month, inside the 100,000 of the free plan).
- Dashboard `pdbplus-overview`: two collapsed rows. "Upstream PeeringDB"
  shows the requests to PeeringDB by status class, retries by cause, and
  the p95 wait for the local rate limiter. "Sync Sweep & Backfill" shows
  the history sweep windows by type and result, FK backfill attempts,
  and FK orphan rows.

### Changed

- `PdbPlusSyncOperationFailed` now fires after at least 2 failed sync
  attempts in 3 hours (`sum(increase(...[3h])) > 1.5`), and
  `keep_firing_for: 1h` keeps it firing between failures that recur
  every few hours. It fired on every failed attempt, also when the retry
  30 seconds later passed, and a failure every 78 minutes made it fire
  and resolve each time.
- The alert rules no longer carry the `receiver: grafana-default-email`
  label: Grafana notification policies route by `severity`, and the
  label had no effect. `deploy/grafana/alerts/README.md` now describes
  the Grafana-managed rules that production runs, and no longer caps the
  rule count at 8. The `PdbPlusSyncFailureRateHigh` and `PdbPlusRssHigh`
  annotations say when the rules fire.
- Dashboard `pdbplus-overview`: the Go Runtime panels show one line per
  machine (`sum by (service_namespace, cloud_region)`); `sum by
  (instance)` summed the fleet, because the `instance` label is empty.
  Sync Success Rate and Fallback Events count over the dashboard range,
  as their 4-minute window was empty between 15-minute cycles. Sync
  Duration (p95) aggregates the buckets per mode over 1 hour. Error Rate
  (5xx) leaves out the health probes, which caused every 5xx of the last
  week.
- A failed sync attempt no longer makes `/readyz` return 503 by itself.
  The check now uses the age of the newest successful sync, as it does
  while a sync runs. Replicas read the same `sync_status` rows, so each
  failed attempt failed the Fly health check of every machine until the
  retry passed, while all of them served the data of the last success.
  The `readyz sync marked failed` log is now DEBUG.
- The ConnectRPC `asn` filter of `ListNetworks`, `StreamNetworks`,
  `ListNetworkIxLans` and `StreamNetworkIxLans` accepts 0. It rejected 0
  as not positive, but the mirror stores upstream tombstones with ASN 0
  since v1.32.2 (net 21510). Negative values still return
  `INVALID_ARGUMENT`.

### Fixed

- The first failed sync attempt after a restart now reaches
  `PdbPlusSyncOperationFailed` and `PdbPlusSyncFailureRateHigh`. The
  `pdbplus_sync_operations_total` series of a new process started at 1,
  so PromQL `increase()` read 0 for its first increment. The primary now
  adds 0 to each status and mode series when it starts or is promoted.
  A failure before the first metric export (one interval after start)
  is still not counted.
- `pdbplus_sync_peak_rss_bytes` is now the peak RSS of the last sync
  cycle. It was VmHWM, the peak since the process started, so after one
  high peak `PdbPlusRssHigh` stayed firing until the next restart. The
  worker now resets VmHWM (`/proc/self/clear_refs`) at the start of each
  cycle.
- A replica outside `PRIMARY_REGION` no longer acts as the primary while
  the primary restarts. LiteFS shows no `/litefs/.primary` file on a
  replica while no node holds the lease, and the role check read that as
  "primary". During three deploys in one week a replica logged
  `promoted to primary`. One of them also started a sync cycle and
  failed to write (`disk I/O error (778)`).
  With LiteFS mounted, a node is now primary only if LiteFS can elect it
  (`FLY_REGION` equals `PRIMARY_REGION`, the `litefs.yml` rule).
- A request whose handler panics now counts in
  `http_server_request_duration_seconds` as a 500 with its `http_route`.
  The Recovery middleware wrapped otelhttp, so the panic unwound past the
  metric record, and the 5xx error rate did not count it. A second
  Recovery now sits inside otelhttp, and the route tag runs in a defer.
  A panic after the response started still records the status already
  sent.

## [1.32.2] - 2026-09-25

### Fixed

- Sync no longer fails on an ixpfx tombstone with a null prefix.
  Upstream sends `"prefix": null` for ixpfx 4185 (deleted 2024-09-10).
  The value decoded to an empty string, and the ent validator of the
  `prefix` field rejected it, so each cycle that fetched the row failed.
  The history sweep fetched it, and in production one cycle failed about
  every 78 minutes. The retry 30 seconds later passed, because the sweep
  did not send the window again for 65 minutes. The sweep did not get
  past ixpfx. The mirror now stores the empty prefix, and `/api/`
  renders it as `null`, as upstream does. REST, GraphQL and ConnectRPC
  send an empty string. The web UI and MCP show only live prefixes.
- Sync no longer fails on a net tombstone with ASN 0. Upstream sends
  `"asn": 0` for net 21510 (deleted 2019-11-22), and the ent validator
  of the `asn` field accepted only positive values. The history sweep
  would have failed in the same way when it got to net. The mirror now
  stores the value as upstream sends it, on net and netixlan. The web
  UI and MCP look up only live networks by ASN.

## [1.32.1] - 2026-09-25

### Changed

- A push of a `v*` tag no longer runs the `ci` job. Before, pushing
  `main` and a release tag together ran `ci` twice for the same commit,
  and the tag's `docker-publish` waited for its own `ci`. Now the first
  step of `docker-publish` on a tag run requires that the tagged commit
  passed the `CI` job in a run for a push to `main`. When that run is
  not registered yet or its `CI` job has not completed, the step waits.
  It checks every 30 seconds and fails after 30 minutes. It fails at
  once when the `CI` job of each such run failed. Tag a commit that a
  push to `main` tested, for example the last commit of a push. A push
  tests only its last commit, so a tag on a commit from the middle of a
  push of more than one commit fails the publish. The tag run still
  builds and pushes its own image, so the image carries the tag as its
  version. See `docs/DEPLOYMENT.md` § Release tags.
- Only a push to `main` publishes `sha-<commit>`. Before, the tag run
  and the `main` run for the same commit both pushed it, and the later
  push set the image. For v1.32.0, `sha-8236a67` is the image of the
  `main` run, not the `1.32.0` image.

### Fixed

- Incremental sync no longer loses a delete when FK backfill lands a
  parent row that upstream changed after the fetch of its type. The
  cursor of each type was the newest `updated` value in its table, so
  the backfilled row moved the cursor past the changes between that
  fetch and the row, and no later `?since=` fetch returned them. The
  cursor is now a watermark in the new `sync_watermark` table. The sync
  transaction writes it without the rows that FK backfill landed, so the
  next cycle fetches the gap again. A cursor that is behind its newest
  row also keeps its watermark when the worker discards its tombstone
  window after a failed incremental fetch. A watermark that is not a
  positive integer fails the cycle before the first request. Without FK
  backfill, the requests and the data writes do not change. The first
  cycle after the upgrade uses the newest `updated` values, logs INFO
  `sync watermark missing, using MAX(updated)` and writes 13 rows. New
  span attributes `pdbplus.sync.cursor`, `pdbplus.sync.cursor.source`,
  `pdbplus.sync.cursor.behind_seconds`, `pdbplus.sync.watermarks_written`
  and `pdbplus.sync.watermarks_held`. This change does not repair the
  deletes that earlier gaps lost. The history sweep fetches them, and
  `POST /sync?mode=history` starts a finished sweep again.

## [1.32.0] - 2026-09-25

### Added

- CI publishes the standalone image (`Dockerfile`) to
  `ghcr.io/dotwaffle/peeringdb-plus` for `linux/amd64` and
  `linux/arm64`. A release tag `vX.Y.Z` publishes the tags `X.Y.Z`,
  `X.Y` and `latest`. A push to `main` publishes `main`. Each published
  commit also gets `sha-<commit>`. Each image has an SBOM, BuildKit
  provenance and a GitHub artifact attestation, which
  `gh attestation verify` checks. The new `docker-publish` job runs only
  on pushes, after the `ci` job passes. Pull requests publish nothing. The Fly image (`Dockerfile.litefs`) is not published.
  See `docs/DEPLOYMENT.md` § Published image.
- A history sweep fetches the tombstones that upstream made before the
  mirror's first sync. A bare list holds only live rows, so these rows
  never reached the mirror. Each incremental cycle sends up to
  `PDBPLUS_HISTORY_MAX_REQUESTS_PER_CYCLE` (default 15, `0` turns the
  sweep off) requests of the form
  `/api/<type>?since=1&status=deleted&id__gte=<from>&id__lt=<to>&depth=0`,
  one type after the other in sync order, and merges the rows in the
  sync transaction. A cycle stops the sweep after the last window of a
  type, so the parents of a type are stored before its children. It
  never deletes a row. It skips `poc`. For `campus` it also fetches the
  pending campuses. A full cycle does not run it. The progress is in the
  new `sync_history_sweep` table. When every type is done, the sweep
  stops. `POST /sync?mode=history` starts it again. With the id ranges
  of 2026-09-24, one sweep is 184 requests (21 cycles at the default)
  and adds at most about 115,000 tombstones. A live row for which
  upstream has a later tombstone becomes a tombstone. This repairs the
  rows that full cycles before v1.28.1 turned back into live rows. A 429
  or WAF block stops the sweep for the cycle with a WARN and does not
  fail the cycle. New span `sync-history-sweep` and counter
  `pdbplus.sync.history.requests`.

### Changed

- The binary version comes from the Go toolchain's VCS stamp in place of
  `git describe`. A release tag still gives `v1.32.0`. Between tags the
  version is a Go pseudo-version such as
  `v1.32.1-0.20260925001332-10674814d276` (was `v1.32.0-3-g1067481`).
  A build from a tree with an uncommitted change, or with an untracked
  file that `.dockerignore` does not exclude, gets a `+dirty` suffix. So
  the Docker build context now holds every tracked file, and
  `.dockerignore` excludes only local state that git does not track. An
  explicit `VERSION` build argument still sets the version. The version
  appears in the PeeringDB User-Agent, the OTel `service.version` and the
  JSON discovery document. The image build log prints it.
- CI builds the standalone image once per push. A push to `main` or a
  `v*` tag no longer runs the `docker-build` job. `docker-publish` builds
  and pushes the image, and writes the build cache of `main`, which pull
  request builds read. Pull requests still run `docker-build`, which
  pushes nothing.
- CI runs the race test suite without coverage and posts no coverage
  comment on pull requests. The `k1LoW/octocov-action` step,
  `.octocov.yml`, the `pull-requests: write` permission of the `ci` job
  and the `mise run coverage` task are removed. CI now runs
  `mise run test`.
- The upstream request rate now follows the limits that PeeringDB
  documents: 20 queries per minute per IP without an API key, and 40
  per minute per user or organization with a key, with at least two
  seconds between queries. With `PDBPLUS_PEERINGDB_API_KEY` set, the
  client sends 30 requests per minute (was 60). Without a key, the
  default `PDBPLUS_PEERINGDB_RPS` is `0.333`, 20 requests per minute
  (was `2.0`, 120 per minute). A full sync cycle takes longer: at the
  default FK backfill cap, backfill adds up to about 40 seconds (was
  about 20). An operator who set `PDBPLUS_PEERINGDB_RPS` keeps that
  value.
- The standalone image (`Dockerfile`) uses the `cgr.dev/chainguard/static`
  runtime base in place of `cgr.dev/chainguard/glibc-dynamic`. The binary
  is `CGO_ENABLED=0` and needs no libc. The image reports the same
  version as `Dockerfile.litefs`, from the VCS stamp above. Before this
  release the standalone image reported only a short commit hash. The
  build stage cross-compiles, so an arm64 build needs no QEMU.
- `Dockerfile.prod` is renamed to `Dockerfile.litefs`. The file builds
  the LiteFS image for the Fly deployment. `fly.toml` builds from the new
  name, so a plain `fly deploy` needs no change. Operators who run
  `fly deploy --dockerfile Dockerfile.prod` must use
  `fly deploy --dockerfile Dockerfile.litefs`.

## [1.31.1] - 2026-09-24

### Fixed

- A facility keeps its `campus_id` when the campus is not in the mirror
  and upstream returns the campus to the FK backfill. Upstream keeps a
  campus `pending` while it has fewer than two facilities. A bare
  `/api/campus` list holds only `ok` campuses, and a `?since=` window
  holds a pending campus only when it changes. Thus the mirror did not
  store a pending campus that did not change after the first sync.
  Before this release, the sync set the `campus_id` of its facilities to
  `NULL` and sent no backfill request. Each full cycle logged these
  facilities in the WARN `fk orphans summary` (3 facilities in
  production). The sync now fetches the missing campuses of all
  facilities of a cycle together with `/api/campus?since=1&id__in=`
  (one request for each 100 campuses), stores them with status
  `pending` and keeps `campus_id`. It still sets `campus_id` to `NULL`
  when backfill is off, a backfill limit is reached, the fetch fails,
  or upstream does not return the campus, and for a facility that the
  FK backfill itself stores. The next full cycle repairs the facilities
  that lost their campus.

## [1.31.0] - 2026-09-24

### Added

- `PDBPLUS_OTEL_SYNC_SAMPLE_RATE` sets the ratio of scheduled sync
  cycles that are traced, from `0.0` to `1.0` (default `1.0`). Set it
  to `0` to trace no scheduled cycle, as before this release.
  `POST /sync` always traces its cycle, and `POST /sync?trace=0` never
  does, whatever the ratio.
- `PDBPLUS_SCRATCH_DIR` sets the directory of the sync scratch database.
  The default is empty, which selects `os.TempDir()`. A set value must be
  an absolute path. On the primary, the scheduler removes the stale
  scratch files in a set directory at start, because a crashed process
  leaves its file. When that sweep does not run, the first cycle of the
  process runs it. `fly.toml` sets `/var/lib/litefs/scratch` on the
  primary volume. Before this release, a full cycle wrote about 100 MB of
  scratch data to `/tmp` on the root file system, which Fly.io limits to
  2000 IOPS and 8 MiB/s. LiteFS does not read or remove files in the
  scratch directory. Each process that stages a cycle in a set directory
  holds a shared `flock` lock on `.pdbplus-scratch.lock` in it, and the
  sweep needs the exclusive lock. Thus the sweep never removes the file
  of another live process. When another process holds a lock, the sweep
  is skipped with a WARN. When a cycle cannot get the shared lock, it
  stages in `os.TempDir()` with a WARN. The directory must not be a
  shared temp dir such as `/tmp`, because a process that stages in
  `os.TempDir()` takes no lock. A cycle also stages in `os.TempDir()`
  with a WARN when the directory has less than 512 MiB of free space, so
  the scratch file does not take the space that LiteFS needs on the
  primary volume. The span attribute `pdbplus.sync.scratch_dir` names the
  directory that the cycle used.

### Changed

- The primary now traces every scheduled sync cycle. Before this
  release, the sampler dropped each scheduled cycle, and only a
  `POST /sync` cycle had a trace. Some signals are only on the sync
  spans: the per-type `sync-fetch-*` and `sync-upsert-*` step spans,
  `pdbplus.sync.status_rows_pruned` and the `tombstone_window.discarded`
  event. The `trace_id` on a log line from inside a cycle now finds its
  trace. The retry and final-failure logs of the scheduler are outside
  the cycle and have no sync `trace_id`. In the prod traces of
  2026-09-24 without their DB spans, a full cycle has about 58 spans and
  an incremental cycle about 44. At the 15m interval, this is about 4.2k
  spans per day.
- A sync cycle emits no DB spans, a `POST /sync` cycle included. A full
  cycle runs thousands of statements, and with their spans its trace
  was larger than the 5 MB per-trace limit of Grafana Cloud Tempo. The
  spans after the limit were lost, among them the upsert step spans of
  net, poc, netfac and netixlan. API request traces keep their DB spans.
- A sync cycle writes each 100-row scratch chunk with two upsert
  statements of 50 rows, not one statement of 100 rows. The SQLite
  driver binds the parameters of a statement in a time that increases
  as the square of their count. In a local test at production row
  counts, the upsert step of a full cycle over a populated database took
  about 8.4 s instead of about 10.7 s.
- A sync cycle stages each type in the scratch database in one
  transaction, not one transaction for each row. In a local test with a
  copy of the production data, the fetch step of a full cycle took
  4.4 s instead of 14.5 s. The scratch database keeps its rollback
  journal in memory, and the journal holds the rows that a tombstone
  window replaces. In the same test, a window that replaced every row
  raised peak RSS by 95 MiB, and a window of 1000 rows for each type by
  9 MiB or less. A failed write to the scratch database now fails the
  cycle. Before this release, such a failure in the incremental attempt
  started a full fetch of the type. On that incremental-fallback path, a
  failure during the tombstone window was logged and skipped, and the
  rows that it did not write were lost.

### Fixed

- `PDBPLUS_OTEL_SAMPLE_RATE` and `PDBPLUS_PEERINGDB_RPS` reject `NaN`
  and infinite values at startup. Before this release, `NaN` passed the
  range check of both variables, and `Inf` passed the
  `PDBPLUS_PEERINGDB_RPS` check. The new `PDBPLUS_OTEL_SYNC_SAMPLE_RATE`
  has the same rule.
- A failed read of the sync cursor (the newest `updated` value of a
  table) now fails the sync cycle before its first upstream request, and
  the next cycle retries. Before this release, the worker logged
  `INFO "failed to get max(updated), using full sync"` and continued
  with a zero cursor, which is the cursor of an empty table. The type
  then fetched the bare list and a `?since=` window from the newest row
  of that list. A bare list holds only live rows, so a row that upstream
  deleted between the real cursor and that newest row was in neither
  fetch. Committing the cycle moved the cursor past the delete, and the
  row stayed live on all surfaces. In incremental mode, the zero cursor
  also skipped the incremental fetch of the type. The cursor read error
  counts in `pdbplus.sync.type.fetch_errors`. A failed fetch step now
  also sets an error status on its `sync-fetch-<type>` span, whatever
  the cause.

## [1.30.0] - 2026-09-24

### Added

- The overview dashboard has a LiteFS Replication row: replica stream
  lag, LTX apply lag, transactions behind the primary, commits on the
  primary, the raw LTX size and connected replicas. It needs
  `PDBPLUS_LITEFS_METRICS_URL`. Re-import
  `deploy/grafana/dashboards/pdbplus-overview.json` in Grafana.

### Changed

- The primary now keeps only the newest 3000 `sync_status` rows, plus
  the newest success row and the newest full success row. Before this
  release, each sync cycle added a row and no row was deleted, so the
  table grew without limit (4.6 MiB in prod on 2026-09-24). At the 15m
  interval, 3000 rows is about 31 days. Each sync cycle deletes up to
  1000 old rows right after it inserts its own row, so the first cycles
  after the deploy delete the old rows in small commits. A failed delete
  logs `WARN "failed to prune sync_status rows"`, and the cycle
  continues. The database file does not shrink: SQLite uses the freed
  pages again for new rows. To keep the old history, export the
  database before you deploy.

### Fixed

- The LiteFS metrics export now works on replicas, and on a primary
  that has not committed since LiteFS started. LiteFS creates
  `litefs_db_commit_count` at the first commit on the node, and the
  export required it. In v1.29.0 every replica logged
  `WARN "litefs metrics scrape failed"` and exported no
  `pdbplus.litefs.*` values, because a replica does not commit. The
  primary exported nothing until its first commit after a restart. The
  export now requires only `litefs_db_txid`, `litefs_lag_seconds` and
  `litefs_subscriber_count`. `pdbplus.litefs.commits` is 0 on a node
  that has not committed since LiteFS started. Until LiteFS creates
  them, `ltx.size`, `ltx.files` and `ltx.lag` have no value.

## [1.29.0] - 2026-09-24

### Added

- The app can export the LiteFS metrics of each node as
  `pdbplus.litefs.*` OTel instruments: transaction ID, commits, LTX file
  size and count, apply lag, replication lag and connected replicas.
  Set `PDBPLUS_LITEFS_METRICS_URL` to the LiteFS metrics endpoint to
  turn this on. The default is off, because LiteFS runs only in the
  Fly.io deployment. `fly.toml` sets `http://localhost:20202/metrics`.

### Changed

- The primary now retries its short writes when SQLite reports a lock
  error: `SQLITE_BUSY` (5) or `SQLITE_PROTOCOL` (15), which failed a
  startup commit with `locking protocol (15)` on 2026-09-24. The retried
  writes are the `sync_status` row writes of each sync cycle, the
  startup reap of stale `running` rows, and the startup poc contact
  scrub and netixlan cascade transactions. A write gets up to 4
  attempts, with waits of 250ms, 500ms and 1s. Before this release, a
  failed startup transaction waited for the next sync cycle, up to one
  interval later. The main sync transaction does not retry. Each retry
  logs `WARN "retrying write after sqlite lock error"` with `op`,
  `attempt` and `error`, and adds 1 to the new counter
  `pdbplus.sync.lock_retries{op}`.
- The sync transaction now checks foreign keys per statement, the SQLite
  default. Before this release, it set `PRAGMA defer_foreign_keys`, so
  SQLite checked them at `COMMIT`. One row with a missing parent then
  rolled back the whole cycle with `FOREIGN KEY constraint failed
  (787)`, no pointer to the row, and the same failure on the next
  cycle. Now the statement that writes the row fails, and the error
  names the type and the batch. FK backfill logs a failed parent upsert as
  `WARN "fk backfill upsert failed"` and the cycle continues.

### Fixed

- The `pdbplus.sync.type.objects` counter now counts the objects of a
  sync cycle only after its transaction commits. Before this release,
  the worker added each type's count before the commit. A cycle that
  rolled back, for example on a failed commit, still added its counts,
  and the next cycle added the same rows again. The Sync Throughput
  panel showed these writes, but no row changed.
- FK backfill now sets the `campus_id` of a backfilled facility to NULL
  when the campus is not in the database, as the facility sync step
  does. Before this release, it wrote the missing campus id, and every
  sync cycle that backfilled that facility failed at `COMMIT`. The
  nulled field adds an orphan with `action=null` to
  `pdbplus.sync.type.orphans`.
- The sync worker now treats a foreign key id of 0 or less as a missing
  parent. A null or absent upstream foreign key decodes to 0. Before
  this release, the worker sent no backfill request for it and wrote
  the row with a reference to parent 0, which failed the cycle at
  `COMMIT`. Now the worker drops a row whose required parent id is 0,
  withholds a backfilled parent with such a reference, and sets an
  optional one to NULL.

## [1.28.5] - 2026-09-24

### Changed

- A full sync writes only the rows that differ from the stored rows.
  It also rewrote unchanged rows before, and SQLite writes the index
  entries of each rewritten column again even when the value is the
  same. So each daily full sync wrote nearly every index page of the
  database, about 53 MiB of 121 MiB, and each replica applied it with
  the WAL locks held. A replica reader that waits more than about 10
  seconds for a lock fails with `locking protocol`.

### Fixed

- The primary now logs `WARN "cascaded network deletes to netixlans"`
  only after the transaction that marks the connections commits. Before
  this release, it logged the line before the commit. When the commit
  failed, the line still reported connections as `deleted`, but they
  stayed live. On 2026-09-24 a startup run logged `count=256` for a
  commit that failed with the SQLite error `locking protocol (15)`. A
  failed commit now logs only the failure, and the next sync cycle does
  the work again. The `WARN "scrubbed contact fields of deleted pocs"`
  line had the same fault. It also comes after the commit now.

## [1.28.4] - 2026-09-24

### Changed

- The GraphQL endpoint rejects a query that selects one response key
  twice with different list or object arguments, as the GraphQL
  specification requires. Before, the two selections could merge into
  one response key. This comes from gqlparser 2.5.58.

### Fixed

- A sync now clears an optional value that upstream removed, such as
  a netixlan IPv6 address, a facility's campus or a network's RIR
  status. The upsert updated only the columns of its INSERT, and a
  batch in which every row had the value unset left the column out,
  so the stored value stayed. Incremental cycles often write batches
  of one row. 33 columns in 8 tables were exposed.
  The next daily full cycle repairs live rows that hold a stale value.

## [1.28.3] - 2026-09-24

### Fixed

- A replica now changes its HTTP ETag when its data changes. Only the
  primary updated the ETag after a sync, so a replica kept the ETag that
  it read at process start. A client or CDN that revalidated with
  `If-None-Match` got `304 Not Modified` from a replica and kept a body
  as old as the replica's last restart. A replica that started before
  the first sync sent no caching headers until it restarted. The ETag is
  now keyed on the version of the local database (the LiteFS position,
  or `PRAGMA data_version` without LiteFS), which every node reads each
  second, so a write outside a sync cycle, such as the startup
  poc-contact scrub or netixlan cascade, also changes it. Each ETag value
  changes once at upgrade, so caches fetch each URL once more.

## [1.28.2] - 2026-09-24

### Changed

- A sync forces a garbage collection after a type only when it upserted
  1000 or more rows of that type. Before this release, it forced one
  after each of the 13 types, at about 20 ms each on the primary,
  although an hourly incremental sync upserts only tens to hundreds of
  rows per type. A full sync still forces one after each large type,
  where it keeps the peak heap low.

### Fixed

- Sync no longer serves the connections (`netixlan`) of a deleted network
  as live. When PeeringDB deletes a network because the RIR reclaimed its
  ASN, it removes the live connections of the network without a
  tombstone, so a `?since=` sync does not see the delete. Sync now marks
  such a connection `deleted` and sets `operational` to `false`. Before it
  marks a connection of a network that was deleted before the current
  sync, it asks PeeringDB for the connection, and it keeps the connection
  live when PeeringDB still serves it. A connection that PeeringDB changed
  after it deleted the network stays live. When the primary starts, it
  also marks the connections that it already holds. `updated` does not
  change.
- A full sync no longer turns a deleted connection back into a live one
  when the PeeringDB cache lists an old copy of it with the same
  `updated` value.

### Upgrade notes

- PeeringDB removes these connections about once a day, and sync marks
  them without a new `updated` value on every API. A client that syncs
  netixlan by `updated` (REST or GraphQL `updated` filters, ConnectRPC
  `updated_since`, `/api/` `?since=`) does not see these changes. The
  same is true against PeeringDB, which sends no tombstone for them.
  Re-fetch the full netixlan list once after this upgrade, and then from
  time to time. `/api/netixlan?since=N` returns these connections with
  status `deleted` when N is not later than their `updated` value.
  PeeringDB returns nothing (see `docs/API.md` § Known Divergences).
- Sync sends up to 10 more requests to PeeringDB in a sync, usually none,
  to check the connections of deleted networks. The first start after
  the upgrade sends about 3. `PDBPLUS_FK_BACKFILL_MAX_REQUESTS_PER_CYCLE`
  now also caps these requests, and `0` turns the check off. The
  connections of a network that the RIR reclaim deletes during a sync
  are still marked in that sync, with no request.
- The Deletes per Type panel of the overview dashboard is now Cascaded
  Deletes per Type. It shows the connections that sync marks deleted.
  Re-import `deploy/grafana/dashboards/pdbplus-overview.json` in Grafana.

## [1.28.1] - 2026-09-23

### Fixed

- A full sync no longer rolls rows back to an older version. PeeringDB
  serves the list that a full sync fetches from a cache that can be hours
  or days old. From v1.21.0, a full sync wrote the cached version of every
  row, `updated` included, over the version that incremental syncs had
  stored, and later incremental syncs did not fetch those rows again. On
  2026-09-23 this reverted 623 netixlans that PeeringDB had marked
  `not-operational`. A full sync now keeps a stored row that is newer than
  the cached version, and fetches the rows that changed after PeeringDB
  built the cache. It still takes the cached version of a row that
  PeeringDB restored to an older version.
- A full sync repairs a reverted row that PeeringDB lists as live. When
  many rows share one `updated` value, the repair can take more than one
  full sync.
- A sync no longer fails with `file exists` when it creates its scratch
  database. The file name had the process ID in it, so a file left by a
  killed process, or by a process in another PID namespace that shares
  the temp directory, could have the same name. The name is now random.

### Upgrade notes

- A client that mirrors any `/api/<type>` list with `?since=` should
  re-fetch each full list once after the first full sync on v1.28.1. The
  repair moves `updated` forward to a value that can be below the
  client's cursor, so a `?since=` poll can miss it.
- A full sync before v1.28.1 could also turn a PeeringDB delete back into
  a live row. Sync does not repair these rows: a bare list contains live
  rows only, and the delete is older than every later window. Such a row
  is live in the mirror, missing from a fresh PeeringDB bare list, and no
  newer than the newest row in that list. PeeringDB returns its tombstone
  for `?since=1&id__in=<ids>`.

## [1.28.0] - 2026-09-23

This release brings the PeeringDB-compatible API (`/api/`) to parity with
PeeringDB 2.83.0 and fixes conformance and privacy bugs. Several `/api/`
responses change. Read "Changed" before you upgrade a client.

### Added

- Mirror the `meta` object that PeeringDB 2.83.0 adds to `net` and
  `netixlan` on every API: `/api/`, REST, GraphQL, ConnectRPC (as
  `google.protobuf.Struct`) and MCP. `/api/netixlan` accepts `meta__*`
  filters.
- Serve the netixlan status `not-operational` as a live status, as
  PeeringDB 2.83.0 does. The web UI and MCP include these connections and
  mark them.
- Accept the upstream names of FK filter keys on `/api/`: `?org=`,
  `?network=`, `?facility_id__in=`, `network__<field>` and
  `facility__<field>`, with the upstream operators.
- Filter `/api/` on the upstream `prepare_query` relation keys, for example
  `fac?net=`, `net?ix=`, `netixlan?name=` and `campus?facility=`. A join row
  that is not live does not match.
- Filter `net` `info_types` and `fac` `available_voltage_services` with the
  upstream rules for multi-value fields. `net` also accepts the legacy
  `info_type` keys.

### Changed

- `/api/` lists without `?since` return rows in `id` order, ascending, as
  upstream does. Up to v1.27.0, the newest `updated` came first. Lists
  with `?since` are in `updated` order, ascending.
- `?since=N` includes the rows updated in second `N`. Up to v1.27.0, a
  poller that sent the last `updated` value it had seen could miss rows.
- A list request with `id` (any type) or `asn` (`net`) that matches no row
  returns `404` with the detail `Entity not found`, as upstream does.
  `id__in` and `asn__in` still return `200` with an empty list.
- `?status=` narrows the status set of the request and no longer adds
  other statuses to it, as upstream does.
- Depth `_set` lists contain only live rows. `net.netfac_set`,
  `ix.fac_set` and `carrier.carrierfac_set` are in facility `id` order.
- `ix.media` is always `Ethernet` and `ixlan.dot1q_support` is always
  `false`, as upstream renders them. `net.info_types` is `[]`, not `null`,
  when it holds no value.
- A caller who may see `ixlan.ixf_ixp_member_list_url` gets the key with
  `""` when no URL is stored. Before, the key was left out.
- `/api/` ignores the filter keys that upstream ignores: `netixlan`
  `net_side*`, `carrier` `fac_count*`, relation keys on fields that are not
  model fields, and reverse or 2-hop `__status` keys.
- ConnectRPC internal errors no longer include database error text. A
  canceled request returns `Canceled`, and a request past its deadline
  returns `DeadlineExceeded`.
- REST query binding errors are in lower case (form decoder 4.5.0).
- The GraphQL playground uses GraphiQL 4.1.2.
- Update connect-go to 1.21.0, the MCP Go SDK to 1.8.0, SQLite to 1.59.0,
  entgo contrib to a master snapshot with entgql fixes, and the
  `golang.org/x` modules. Lock buf 1.73.0 and govulncheck 1.8.0.
- Do not record the ConnectRPC `rpc.server.*` metrics. They were dropped
  before export, so the exported metrics do not change.

### Fixed

- Do not serve the name, phone, email or url of a deleted contact (`poc`)
  on any API. Sync stores deleted contacts without these fields, and the
  primary blanks them on the tombstones that it already holds.
- `PDBPLUS_PUBLIC_TIER=users` no longer shows `Private` contacts, on any
  API or through a filter.
- The GraphQL `where` input no longer has contact edge predicates, which
  could test values of hidden contacts.
- A 2-hop `/api/` key through contacts, for example
  `/api/net?poc__net__asn=`, no longer matches through contacts that the
  caller cannot read.
- REST `sort=pocs.count` returns `400`, and the OpenAPI sort list no
  longer offers it. The count included hidden contacts.
- `/api/` time filters and `?since=` compare in UTC. A value with an
  offset, for example `?updated=2026-04-01T11:00:00+01:00`, matches the
  same instant stored in UTC.
- Sync sets netixlan `operational` when upstream leaves it out.
- The `/api/fac` detail budget no longer counts sets that the response
  does not include.

### Upgrade notes

- A client that mirrors `/api/netixlan` with `?since=` must re-fetch the
  full list once. Rows that became `not-operational` were hidden before
  v1.28.0.

## [1.27.0] — 2026-09-07

### Changed

- Update Go to 1.27.1 and gqlgen to 0.17.95.
- Update entrest to 1.2.0, gqlparser to 2.5.37, otelsql to 0.44.0,
  compress to 1.20.0, and SQLite to 1.58.0 with libc 1.75.6.
- Use the new REST JSON encoder. Empty list fields emit arrays instead of
  `null`, and pretty responses use tabs. The PeeringDB-compatible API keeps
  its existing serializers.
- Migrate the web UI to htmx 4, including history navigation and fragment
  error handling.

### Fixed

- Replace CARTO maps, which now require an API key, with OpenStreetMap tiles.
  Operators can configure a different tile URL and attribution.
- Pin the CI mise installer to 2026.9.1 to avoid a missing release archive.
  Skip cache cleanup when tool setup has not created the cache directory.

## [1.26.0] — 2026-08-29

### Added

- Add an MCP server card, Agent Skills discovery index, standard skill alias,
  `llms.txt`, and root `Link` headers for agent discovery.

### Changed

- Support MCP 2026-07-28 sessionless discovery while retaining legacy
  handshake compatibility.
- Add tool output schemas, stronger input constraints, explicit read-only
  capabilities, request cancellation, and current MCP CORS headers.
- Move the toolchain to Go 1.26.7 and golangci-lint 2.13.2.
- Bump direct Go modules: `github.com/KimMachineGun/automemlimit`
  0.7.5→1.0.0 (new `memlimit.Set` entry point, same cgroup-then-system
  provider chain), the OpenTelemetry API/SDK 1.45.0→1.46.0 with contrib
  0.70.0→0.71.0 and `sdk/log` 0.21.0→0.22.0, `modernc.org/sqlite`
  1.56.0→1.57.0, and `github.com/stretchr/testify` 1.12.0→1.12.1.
  `github.com/lrstanley/entrest` stays at 1.0.4 because 1.1.0 requires
  Go 1.27. `govulncheck` reports no vulnerabilities.

## [1.25.0] — 2026-07-24

### Added

- Show the application version and optional serving region on the About page
  without requiring a Fly.io runtime.
- Add a read-only Streamable HTTP MCP server with catalog search,
  entity detail, network comparison, IP lookup, sync freshness,
  resources, and guided prompts.
- Host an origin-neutral agent skill as both Markdown and an installable ZIP.
  The ZIP is generated per request so its MCP dependency follows the requested
  hostname or `PDBPLUS_PUBLIC_URL`.

### Changed

- Extract web catalog queries and result types into a protocol-neutral package
  shared by the UI and MCP server.
- Add the MCP Go SDK at v1.4.1, including its default DNS-rebinding protection.

### Fixed

- Correct the REST pagination and About-page documentation examples.

## [1.24.1] — 2026-07-24

### Fixed

- Correct production version stamping so the intentionally filtered Docker
  build context does not make every release report a false `-dirty` suffix.

## [1.24.0] — 2026-07-24

Toolchain consolidation and dependency refresh.
There are no API or runtime behavior changes from PeeringDB 2.81.0:
its intervening changes affect upstream writes, IX-F cleanup,
and RIR deletion verification rather than the mirror's read contract.

### Added

- Add a cross-platform mise manifest and lockfile covering Go, code generators,
  Tailwind, gotestsum, lint, and vulnerability tooling.
- Add mise tasks for generation, build, race tests, coverage, lint,
  vulnerability scanning, and the canonical local validation sweep.

### Changed

- Move CI and local development to the same locked tool versions.
  CI now installs tools through mise, runs race coverage through gotestsum,
  validates its workflow with actionlint, and retains the existing cached
  sequential Go job plus parallel Docker builds.
- Replace the dynamically assembled coverage package list with explicit
  hand-written package trees; `.octocov.yml` remains the file-level exclusion
  authority for generated GraphQL and templ output.
- Update Go to 1.26.5, `otelsql` to 0.43.0, `compress` to 1.19.1,
  `ogen` to 1.23.0, modernc SQLite to 1.54.0, and Tailwind to 4.3.3.
  SQLite 1.54.0 includes the upstream WAL corruption fix in SQLite 3.53.3.

### Removed

- Remove the `go.mod` tool block and Buf's codegen-only transitive module graph.
- Remove the custom Tailwind downloader; mise now downloads and verifies the
  same standalone GitHub release assets without requiring Node.js.

## [1.23.0] — 2026-07-10

Full-repository review remediation across sync reliability, API correctness,
web usability, operations, tooling, and internal structure.

### Fixed

- Harden sync watchdogs, FK pre-processing, heap telemetry, upstream
  authentication diagnostics, and tombstone filtering.
- Correct HTTP recovery, conditional requests, REST redaction scope,
  gRPC health evaluation, stream timeout validation, and reserved metadata.
- Improve web accessibility, static asset delivery, CSP enforcement,
  relation pagination, error handling, and light/dark presentation.
- Extend pdbcompat response budgeting and lock registries and filter tables
  against generated schema or SQL drift.

### Changed

- Make Docker builds more cacheable and runtime images smaller.
- Harden schema, compatibility, load-test, LiteFS, and observability tooling.
- Consolidate repeated API, sync, GraphQL, and command wiring behind typed
  registries and shared helpers.

## [1.22.0] — 2026-06-30

Upstream parity refresh to PeeringDB 2.80.1, plus a maintenance sweep. The new
`net.ixp_update_exclude` field is the only read-path addition across all 13
types since the prior ~2.77 anchor.

### Added

- **`ixp_update_exclude` on `/api/net`.** PeeringDB 2.80.1 added this field —
  a JSON list of the IX-F fields a network excludes from automatic import
  updates (`speed`, `is_rs_peer`, `operational`) — to `NetworkSerializer`. The
  pdbcompat `/api`, entrest REST, and GraphQL surfaces now emit it (an empty
  list `[]` when unset, matching upstream), and it is synced from upstream and
  round-trips through storage. ConnectRPC is unchanged: the proto surface has
  been frozen since v1.6.

### Changed

- Refresh the pdbcompat upstream parity anchor from `99e92c72` (~2.77) to
  `545c58a4` (PeeringDB 2.80.1) and re-verify the cited upstream line numbers
  throughout `docs/API.md`. A full schema re-extract confirmed the only
  read-path field drift across all 13 types in that range is the
  `ixp_update_exclude` field above; every other upstream change in the window
  is write-path and irrelevant to a read-only mirror.
- Rebuild `cmd/pdb-schema-extract` around DRF serializer introspection so it
  parses the current django-peeringdb 3.7.0 / peeringdb-server source layout,
  and document it explicitly as an upstream drift detector — not the schema
  source of truth, which stays hand-curated and is consumed as such by
  `cmd/pdb-schema-generate`.
- Bump Go modules to their latest releases via `go get -u`; `golangci-lint`
  and `govulncheck` are clean.

### Removed

- Drop the obsolete `GEMINI.md` agent-scaffolding file.

## [1.21.1] — 2026-06-25

Maintenance release: dependency and CI-tooling updates only. No functional or
API changes, and `govulncheck` reported no vulnerabilities — this release keeps
the module graph and CI actions current.

### Changed

- Bump direct Go modules to their latest patch/minor releases:
  `golang.org/x/net` 0.55.0→0.56.0, `golang.org/x/text` 0.37.0→0.38.0,
  `modernc.org/sqlite` 1.52.0→1.53.0, `charm.land/lipgloss/v2` 2.0.3→2.0.4,
  `github.com/99designs/gqlgen` 0.17.90→0.17.92,
  `github.com/vektah/gqlparser/v2` 2.5.33→2.5.35, and
  `github.com/ogen-go/ogen` 1.20.3→1.22.0. The `buf` build tool moves
  1.68.4→1.71.0; `go mod tidy` settled the transitive graph, and the gqlgen
  GraphQL output was regenerated for the bump.
- Raise the `go` directive to 1.26.4 to match the toolchain.
- Update CI action pins to their latest majors: `actions/checkout` v6→v7 and
  `actions/cache` v5→v6.
- Add `-v` to the production image's `go build` so compile progress is visible
  in build logs, making a stalled build distinguishable from a slow one.

## [1.21.0] — 2026-06-10

Fixes for all 42 confirmed findings of the 2026-06-10 full-codebase audit
(4 high, 16 medium, 22 low; 3 further findings were refuted during
adversarial verification).

### Security

- **REST no longer leaks the tier-gated `ixf_ixp_member_list_url` through
  eager-loaded edges.** Redaction was scoped to `/rest/v1/ix-lans*` and
  top-level JSON keys, but entrest eager-loads the ixlan edge on
  internet-exchange, ix-prefix, and network-ix-lan responses — the gated URL
  reached anonymous callers under `edges.*`. The middleware now buffers all
  `/rest/v1/` responses and redacts recursively wherever the `_visible`
  companion appears.
- **ConnectRPC request bodies are capped at 1 MB** via
  `connect.WithReadMaxBytes` (raw and decompressed). The HTTP-level body cap
  deliberately exempts ConnectRPC paths for streaming, which had left unary
  endpoints unbounded — a single gzip-bombed POST could OOM a 256 MB replica.
- **GraphQL complexity costing is fan-out aware.** Default gqlgen costing
  charged 1 per field, so nested unpaginated edge lists could materialize
  millions of rows under the old limit. Connections now cost by requested
  page size and edge lists by average per-parent cardinality.
- **The pdbcompat response budget accounts for concurrency.** Admission
  charges a shared in-flight pool; requests that would jointly exceed the
  budget get 503 + Retry-After instead of stacking past replica memory.

### Fixed

- **Full-mode syncs no longer lose the tombstone window.** A bare list is
  `status='ok'`-only and committing the snapshot advances the derived cursor,
  so the daily forced-full cycle silently discarded upstream deletes in the
  window — permanently. Full staging now issues a follow-up `?since=<cursor>`
  fetch (tombstones win via scratch `INSERT OR REPLACE`); a window-fetch
  failure fails the type so the cursor never advances past unseen deletes.
- **Full-mode syncs now reconcile completely.** The upsert skip gate
  (`excluded.updated > updated`) applied in every mode despite docs claiming
  otherwise, so rows mutated locally without an `updated` bump (orphan-filter
  FK nulls) never re-converged. Full cycles now bypass the gate.
- **Tombstoned rows are hidden everywhere.** Depth≥2 `_set` collections, all
  web UI queries (52 sites), and GraphQL's `networkByAsn` now filter
  `StatusIn("ok", "pending")`, matching the list-path status matrix.
- **pdbcompat parity restored on five fronts:** `?since=0` is inert
  (upstream's `if since > 0` gate), the since boundary is strictly greater,
  and since lists order `updated` ascending; non-numeric/negative
  `limit`/`skip` return 400 and the hidden 1000-row clamp is gone; bare
  `city`/`address1`/`state` filters are substring matches and `country`
  follows the 2-char-iexact rule; time filters accept ISO 8601 with
  upstream's date day-window semantics; string `__in` is case-insensitive
  and routes folded fields through the `_fold` shadow columns.
- **Caching headers corrected:** error responses and `/healthz`/`/readyz`
  are `no-store` (the readiness 304 short-circuit no longer masks health
  flips); render paths `Add` to `Vary` instead of clobbering gzhttp's
  `Accept-Encoding`; web 404/500 pages set headers before the status.
- **Replica readiness latch recovers.** A replica booted before the
  primary's first successful sync no longer serves 503 forever — the
  heartbeat re-reads sync_status until ready.
- **`unifold` folds `œ`/`Œ`, `ð`/`Ð`, and dotless `ı`** like upstream's
  unidecode. Run one full sync after deploying to converge stale `_fold`
  values.
- **The served shell-completion scripts work** (`pdb asn 13335` no longer
  issues two URLs; zsh subcommand completion expands).
- **`fly.toml` health checks probe `/readyz`** so Fly Proxy actually
  excludes hydrating replicas; `Dockerfile.prod` builds with
  `CGO_ENABLED=0` as documented.

### Changed

- GraphQL flat-list resolvers batch edge loads via `CollectFields`
  (previously 1+N queries per request).
- SQLite page cache is 8 MB per connection (was 32 MB; the pool multiplied
  it to 320 MB worst-case on a 256 MB replica).
- `POST /sync` returns 409 when a cycle is already running instead of a
  202 whose trigger was silently dropped; on-demand syncs honour demotion;
  the retry ladder short-circuits on upstream WAF blocks.
- `/rest/v1/` 4xx errors carry entrest's validation detail; the served
  OpenAPI spec documents the problem+json error shape actually emitted.
- Incremental since-pagination stops on a short page, saving one upstream
  request per type per cycle.
- Dead exported code removed (`peeringdb.FetchType`, the grpcserver
  stream-cursor wire codec); the `/api/` problem+json error envelope and
  the poc_set/fold-window divergences are now registered and test-locked
  in `docs/API.md § Known Divergences`.

## [1.20.6] — 2026-06-08

### Fixed

- **The `/api/` index now matches upstream PeeringDB's shape.** It returned a
  peeringdb-plus-specific `{"<type>":{"list_endpoint":"/api/<type>"},...}` with
  relative paths; it now emits upstream's
  `{"data":[{"<type>":"<absolute-url>",...}],"meta":{}}` envelope with absolute
  list-endpoint URLs built from the request host, so a drop-in client can follow
  them directly. Locked by `TestIndex`. Upstream's 14th endpoint `as_set` (a
  bulk `asn → irr_as_set` dump) remains unmirrored and is omitted from the index
  so it never advertises a dead link — documented in
  `docs/API.md § Known Divergences`.
- **The root `/` discovery JSON reports the real build version.** It served a
  hardcoded `"version":"0.1.0"` and never consulted `internal/buildinfo`, even
  though `Dockerfile.prod` already injects `git describe` into it via -ldflags.
  The banner now reflects the deployed build. Go's `debug.ReadBuildInfo` records
  only the commit, never the tag, so the tag must be injected — `buildinfo`
  resolves injected → `Main.Version` → `vcs.revision` → `unknown`.

## [1.20.5] — 2026-06-08

### Fixed

- **pdbcompat `/api/` single-object depth responses now match upstream
  PeeringDB.** A live shape comparison of all 13 types at `?depth=0/1/2`
  against `www.peeringdb.com/api` (2026-06-08) surfaced several divergences,
  all corrected and locked by `internal/pdbcompat/depth_test.go`:
  - **`?depth=1` is now a real, distinct level.** The detail handler honoured
    only `?depth=0/2` and silently coerced every other value (including `1`) to
    `2`. Depth is now clamped to `[0, 4]` as upstream does; `depth=1` expands
    forward FK objects flat with reverse `_set` fields as bare ID lists. Depths
    3–4 render the depth-2 shape.
  - **`ixlan` exposes `net_set`, not `netixlan_set`.** Upstream's
    IXLanSerializer resolves the netixlan join to its networks
    (`nested(NetworkSerializer, through="netixlan_set", getter="network")`); the
    mirror was exposing the raw join rows under the wrong key and omitting
    `net_set` entirely.
  - **Second-level nested FK objects at `?depth=2` now carry their own
    reverse-relation ID lists** (e.g. a netixlan's `net` carries
    `poc_set`/`netfac_set`/`netixlan_set`, its `ixlan` carries
    `net_set`/`ixpfx_set`), matching upstream's recursive depth budget. Removes
    the bounded divergence deferred in v1.19.3.
  - **Nested back-reference FK stripping corrected** to match upstream's
    per-serializer `exclude=` lists: a facility nested under a campus keeps
    `campus_id` and drops `org_id`; a carrierfac nested under a carrier keeps
    `carrier_id`.
  - **Campus-less facilities emit `campus:null` at detail depth** rather than
    omitting the key (upstream's `FacilitySerializer.campus` is a related
    field present at detail depth).

### Changed

- Response-budget row-size floor (`internal/pdbcompat/rowsize.go`) recalibrated
  for the larger depth=2 rows (leaf join entities grew most now that each
  embeds its FK objects' own ID-list sets); a `?depth=1` request bills the
  depth=2 estimate.
- One intentional non-parity is retained and documented in
  `docs/API.md § Known Divergences`: anonymous `poc_set` ID lists omit
  non-`Public` POC ids that upstream lists at `?depth=1` (upstream hides them
  only on expansion). Matching upstream there would leak the existence of
  non-`Public` contacts, contradicting the row-level `poc.visible` privacy
  policy.

## [1.20.4] — 2026-06-08

### Changed

- **Trimmed low-signal SQL trace spans.** otelsql now omits `sql.rows` and
  `sql.conn.reset_session` spans (via `SpanOptions`). These roughly halve the
  DB-span count on every request trace and eliminate the orphan single-span
  traces that connection-pool lifecycle and boot-time schema-migration DB
  operations previously emitted with no request root. The `sql.conn.query`
  spans (carrying `db.statement`) are retained.

## [1.20.3] — 2026-06-08

### Fixed

- **Overlong location text no longer overflows search rows or detail headers.**
  Some exchanges store a comma-separated list of cities in the upstream `city`
  field (e.g. IX 3958 "1-IX EU" lists eleven cities, ~95 chars). In search
  results this expanded and squashed the entity name out of the row; in the
  detail-page header it overflowed the subtitle. The search-row city is now
  width-capped with ellipsis truncation, and the detail-header subtitle
  truncates to the available width. Both keep the full text available via a
  `title` tooltip; normal short locations are unaffected.

## [1.20.2] — 2026-06-08

### Changed

- **Search result location renders the city before the country flag.** The flag
  was rendered first, so it floated at a position determined by the city text
  width and flags did not line up down the result list. Putting the city first
  makes the fixed-width flag the rightmost (flush-right) element, so flags align
  on the right edge across all rows.

## [1.20.1] — 2026-06-08

### Changed

- **Search count badges now show the exact total match count.** A grouped
  search type that overflowed the 10-result quick-search cap previously showed
  "(10+)", which disagreed with the "View all N" link's exact figure. The badge
  now shows the same exact total (e.g. "Networks (1,234)"), comma-formatted. The
  terminal search header (`curl`/plain user agents) shows the exact total too,
  for parity. The total is already computed for overflowing types, so this adds
  no extra query.

### Dependencies

- Bump `golang.org/x/sync` v0.20.0 → v0.21.0 and `modernc.org/sqlite`
  v1.51.0 → v1.52.0.

## [1.20.0] — 2026-06-08

### Added

- **"View all" search results.** Each grouped search type that exceeds the
  10-result quick-search cap now links to a per-type results page
  (`/ui/search?q=<term>&type=<slug>`) that shows the exact total match count and
  the full result set, paginated with an htmx "Load more" button (50 per page).
  Each request loads at most one page, so memory stays bounded. The exact total
  is computed only for types that overflow the quick-search cap, and the link
  reads "View all N". Result ordering is now deterministic (name, then id)
  across both the quick-search top-10 and the view-all pages.

### Fixed

- **Facility detail pages showed the facility's own name for every related
  carrier, network, and exchange.** PeeringDB's association objects
  (`carrierfac`, `netfac`, `ixfac`) carry a `name` equal to the facility name,
  not the related entity; the web UI rendered that value directly, so e.g.
  every carrier at "Global Switch Paris" displayed as "Global Switch Paris".
  Names are now resolved from the related-entity edge, and the carrier and IXP
  lists are sorted by the related entity's name. The `/api`, REST, GraphQL, and
  gRPC surfaces are unchanged: there the association `name` equals the facility
  name by upstream contract (drop-in parity).

## [1.19.5] — 2026-06-04

### Added

- **Per-query SQL tracing (`PDBPLUS_OTEL_SQL`, on by default).** The shared
  `*sql.DB` is opened through XSAM/otelsql so every statement — ent's and the
  raw `sync_status` queries — emits a DB span nested under the active
  request/sync span, surfacing the SQL behind a request that flat traces
  couldn't show. On by default (the data is useful and bounded); set
  `PDBPLUS_OTEL_SQL=false` to disable. DB-span volume is bounded by the
  existing sampler: API reads inherit `PDBPLUS_OTEL_SAMPLE_RATE`, and
  **scheduled sync cycles are no longer traced at all** (so they emit no DB
  spans — the historical high-volume concern), while a **manually triggered
  `POST /sync` is traced by default** — pass `?trace=0` to opt out.
  Implemented via two new sampler gates (`pdbplus.origin=sync` drops scheduled
  cycles; `pdbplus.force_sample` force-samples a manual run) and a
  `WithForceTrace` context flag threaded from the `/sync` handler.

### Changed

- **Scheduled sync cycles are no longer sampled into traces** (previously the
  per-route default ~1%). They are dropped by the new `pdbplus.origin=sync`
  sampler gate so the sync path stays trace-free unless a sync is triggered
  manually via `POST /sync`. Independent of `PDBPLUS_OTEL_SQL`.

## [1.19.4] — 2026-06-04

### Changed

- **Request access log and server spans now record the query string.**
  The `Logging` middleware logged only `method`/`path`/`status`/`duration`,
  and the otelhttp server span carried `url.path` but not the query — so
  `/api` requests were uninspectable from either surface (a trace or log
  entry showed `/api/net` with no hint of the filter the caller sent).
  The middleware now adds `query` to the access log and stamps
  `url.query` onto the active span, for every API surface, so a request
  can be reconstructed from Loki or a trace alike. Health/readiness
  probes remain skipped.

### Security

- **The HTTP server is now treated as a public OTel endpoint**
  (`otelhttp.WithPublicEndpointFn`). An inbound `traceparent` previously
  made our server span a child of the caller's trace, letting a client
  choose our trace-ids and — via the ParentBased sampler — force our
  sampling decision (a sampled traceparent overriding the per-route rate
  is a trace-volume/cost vector). Each request now starts a fresh root
  span with its own trace-id; the inbound context is kept as a span link.
  The global W3C TraceContext + Baggage propagator is unchanged, so
  outbound propagation still works once a downstream service exists.

## [1.19.3] — 2026-06-04

### Fixed

- **`/api/<type>/<id>?depth=2` nested serialization now matches upstream
  PeeringDB.** A conformance comparison against live `www.peeringdb.com`
  surfaced three divergences in single-object depth-2 expansion, now
  corrected: nested reverse-set elements drop the parent back-reference
  FK (a netixlan embedded under a net no longer carries `net_id`); an
  embedded `org` carries its own reverse relations as bare ID lists
  (`net_set`/`fac_set`/…) instead of being flattened away; and the
  Facility serializer no longer embeds `netfac_set`/`ixfac_set`/
  `carrierfac_set` at depth=2 (a ~28x payload reduction for dense
  facilities), expanding only its `org` and `campus` FK objects. Flat
  (depth-0) list/detail responses were already byte-faithful across all
  13 types. The one remaining bounded gap — second-level nested FK
  objects stay flat — is recorded under § Known Divergences in the API
  reference.

## [1.19.2] — 2026-06-04

### Removed

- **Ported parity-fixture pipeline (`internal/testutil/parity` +
  `cmd/pdb-fixture-port`).** The ~55k lines of generated fixture data
  carried unseedable Python-source artefacts and were consumed by no
  behavioural test, and the `--check` drift command was wired into
  neither CI nor the `go generate` drift gate. The
  `internal/pdbcompat/parity` regression suite is unaffected: each test
  already seeds clean rows inline via the ent client and cites the
  upstream `pdb_api_test.py` source line in a comment.

### Added

- **Per-entity status×since matrix regression test** covering all 13
  PeeringDB types (previously only `net` + `campus` had behavioural
  coverage), guarding against a per-entity wiring omission that would
  leak `deleted`/`pending` rows onto the anonymous `/api` list surface.
- **`StreamIxLans` field-level redaction coverage** at both privacy
  tiers, closing the one load-bearing redaction surface that had no
  test for the gated `ixf_ixp_member_list_url` field.

### Changed

- **CI collapsed from four parallel Go jobs to one.** The separate
  lint / test / build / govulncheck jobs were folded into a single
  cached `ci` job that runs them in order (generated-code drift check →
  `go build` → race tests → `golangci-lint` → `govulncheck`) against one
  warmed module/build cache; `docker-build` stays a separate parallel
  job. `govulncheck` is now advisory (`continue-on-error`) — a flagged
  vulnerability warns but does not block the merge.
- **Five overview-dashboard panels reworked for honest rendering.**
  Sync Duration (p95) draws as points (sparse per-sync data, not a
  holey line); Sync Operations shows `round(increase())` bars (cycle
  counts, not a meaningless sub-0.01 ops/s rate); Request Rate by Route
  labels the empty-route series `(unrouted)` instead of "Value"; Latency
  adds p50 and excludes health/readiness probes (~92% of requests,
  which masked the real API tail); Objects Synced per Type rounds the
  edge-extrapolated counts.

### Fixed

- **`<field>_fold` shadow columns no longer leak on the REST surface.**
  The diacritic-folding shadow columns are server-side plumbing and are
  skipped on the GraphQL and proto wire surfaces, but the entrest path
  still emitted them; they are now stripped from `/rest/v1/` responses.
- **Sync-freshness gauge reported replica uptime, not data age.** The
  `pdbplus.sync.freshness` value was cached and refreshed only by the
  sync worker's `OnSyncComplete`, which never fires on replicas (they
  do not run the worker), so the gauge climbed with replica uptime — a
  node up 22h reported 22h of staleness while serving fresh data,
  tripping the >2h alert fleet-wide. It now reads `sync_status` live on
  each metric collection (a cheap single-row local lookup), so replicas
  report true data age and real LiteFS replication lag.

### Internal

- **`go generate ./...` now converges in a single pass.** The
  `pdb-schema-generate` step is sequenced ahead of `entc` inside
  `ent/generate.go`, so the schema producer always runs before its
  consumer and no second pass is needed.
- **Treewide removal of internal planning and process vocabulary** from
  comments, test identifiers, documentation, log/panic strings, the
  Grafana dashboards, and the code-generation sources. Behaviour-
  preserving; genuine external references (upstream source citations,
  versions, dates) are kept.

## [1.19.1] — 2026-05-31

### Changed

- **Vendored htmx bumped to 2.0.10.**

## [1.19.0] — 2026-05-31

_v1.17 and v1.18.x shipped as incremental patch work that was not
catalogued here individually; see the Git tags for those releases. The
entries below are the 2026-05-30 audit-hardening batch (merged via
PR #13)._

### Breaking

- **`PDBPLUS_INCLUDE_DELETED` is now a fatal startup error.** The v1.16
  deprecation logged a WARN and ignored the variable, promising a hard
  error in v1.17; that is now enforced. Remove it from your environment —
  sync always persists deleted rows as tombstones.

### Added

- **`__in` filtering on bool, float and time fields.**
  `?info_unicast__in=true,false`, `?latitude__in=…`, and
  `?created__in=<epoch>,<epoch>` now filter instead of returning `400`,
  matching upstream Django coercion. Values bind through ent's type
  converter (not the string `json_each` path), so time comparisons match
  the stored representation exactly.

### Changed

- **Repeated query parameters take the last value** (`?asn=1&asn=2` → `2`),
  matching Django's `QueryDict` and upstream PeeringDB (was first-value).
- **Filter type errors name the type** (`field type bool`) instead of the
  internal enum integer (`field type 2`).
- **`PDBPLUS_SYNC_STALE_THRESHOLD` is validated at startup** — a
  non-positive value is rejected at boot rather than pinning `/readyz` at
  503 for the process lifetime.
- **Detail endpoints (`/api/<type>/<id>`) are gated by the response memory
  budget**, like list endpoints — an over-budget `depth=2` expansion now
  returns `413` instead of being served unbounded.

### Security

- **`/api` 500 responses no longer echo raw ent/SQL error strings.** The
  driver error is logged server-side; the client receives a generic
  detail.
- **`Cache-Control: private` on Users-tier deployments.** When
  `PDBPLUS_PUBLIC_TIER=users`, responses carry private-audience data and
  are no longer marked `public`, so shared/CDN caches will not store them.
  Public deployments are unchanged (`public`).

### Performance

- List pages that serve no rows (e.g. `?skip=` past the end of the result
  set) short-circuit before the `ORDER BY … OFFSET` sort.
- The sync-freshness gauge reads a cached value instead of a live
  `sync_status` query on every Prometheus scrape.
- FK validation memoises confirmed-present parents per sync cycle,
  collapsing the per-child `Exist()` queries when many children share one
  untouched parent.
- pdbcompat list serialization builds its output in a single pass,
  dropping a redundant intermediate slice.
- The `/rest/v1/ix-lans` redaction path skips re-marshalling the body when
  no field was gated out.

### Fixed

- Corrected three documentation claims against the pinned upstream source:
  the detail `?depth=` default is `2` (not `0`); there is no top-level
  `meta.count` (the empty `__in` example is `{"data":[],"meta":{}}`); and
  `org_flags` is not an upstream filter parameter (recorded as a
  Validation Note, not a divergence).

### Internal

- Removed dead sync code (the write-only FK skipped-ID tracker and the
  unread `getStatus` filter) and the unused `litefs.IsPrimary` wrapper.
- Clarified stale doc comments: the per-request heap-delta metric is
  process-global, the FK-backfill cap default is 20, stream cursors are
  session-local, and the REST writers' `http.Flusher` contract (the
  pass-through writer delegates `Flush`; the buffering redact writer must
  not).

## [1.16.0] — 2026-04-19

v1.16 is a coordinated release. The cross-surface default ordering,
status×since matrix, `?limit=0` semantics, Unicode folding, cross-entity
traversal, and memory-safe response paths ship together in a single deploy
window; the upstream parity regression suite ships independently as a
code-only test lock-in. Do not deploy the `?limit=0` change in isolation —
pdbcompat `?limit=0` now returns all matching rows, and the memory-safe
response paths that bound that behaviour ship alongside it.

> **Coordinated release window:** the v1.16 behavioural changes are now
> complete and ready to deploy as a bundle. The `limit=0` unbounded
> semantics are safe in prod only with the memory budget in place — do
> NOT ship the unbounded-limit change without the memory budget. The
> parity regression test lock-in ships independently as a follow-up —
> no production deploy required; it is a CI regression gate only.
>
> **v1.16 complete (2026-04-19):** all behavioural changes shipped.
> Default ordering, status, limit, `__in`, Unicode, traversal, memory,
> and parity coverage are traced and complete.

### Breaking

- **Removed `PDBPLUS_INCLUDE_DELETED` environment variable.** Sync now
  always persists deleted rows as tombstones (soft-delete via
  `UPDATE ... SET status='deleted'`). During the v1.16 → v1.17 grace
  period, setting this variable triggers a startup WARN and is ignored;
  v1.17 upgrades this to a fatal startup error. Remove it from your
  environment. See
  [`docs/CONFIGURATION.md` § Removed in v1.16](./docs/CONFIGURATION.md#removed-in-v116).

  **One-time gap:** Rows hard-deleted by sync cycles BEFORE the v1.16
  upgrade are gone forever. `?status=deleted` and `?since=N` queries
  populate going forward from the first post-upgrade sync cycle. See
  [`docs/API.md` § Known Divergences](./docs/API.md#known-divergences).

### Added

- **pdbcompat status × since matrix** matching upstream
  `peeringdb_server/rest.py:694-727`. List requests without `?since`
  return only `status=ok`. List requests with `?since=N` admit
  `(ok, deleted)`, plus `pending` for campus. Single-object GETs
  (`/api/<type>/<id>`) admit `(ok, pending)` for all 13 entity types.
  Explicit `?status=deleted` on a list request without `?since`
  silently returns an empty set, matching the upstream unconditional
  `filter(status='ok')` on `rest.py:725`.

- **pdbcompat `?limit=0` semantics** match upstream `rest.py:734-737`:
  an explicit `limit=0` returns all matching rows. The default-when-unset
  remains `250`. `?depth=` on list endpoints is silently ignored at this
  stage; list+depth support with the `API_DEPTH_ROW_LIMIT=250` cap
  follows later.

- **pdbcompat cross-surface default ordering** flipped to
  `(-updated, -created, -id)` (shipped earlier in v1.16).
  Applies to pdbcompat `/api/`, entrest `/rest/v1/`, ConnectRPC list
  RPCs, and GraphQL list queries. Single-object lookups and nested
  `_set` fields are unchanged.

- **pdbcompat Unicode folding** for diacritic-insensitive matching on
  searchable text fields. `?name__contains=Zurich` now matches a DB
  row where `name="Zürich"`. Implementation uses shadow columns
  (`<field>_fold`) populated at sync time by a new `internal/unifold`
  package (NFKD decomposition + a small hand-rolled ligature map for
  `ß`/`æ`/`ø`/`ł`/`þ`/`đ`). 16 shadow columns across 6 entity types
  (network, facility, internetexchange, organization, campus,
  carrier). Matches upstream `peeringdb_server/rest.py:576`
  (`unidecode.unidecode(v)`).

- **pdbcompat operator coercion**: `__contains` is now equivalent to
  `__icontains` (case-insensitive) and `__startswith` is equivalent
  to `__istartswith`, per upstream `rest.py:638-641`. All other
  operators (`__exact`, `__iexact`, `__gt`, `__lt`, `__gte`, `__lte`,
  `__in`) are unchanged.

- **pdbcompat `__in` large-list support**: `?<field>__in=` now accepts
  arbitrarily-large comma-separated lists via a SQLite `json_each`
  single-bind rewrite, bypassing the 999-variable parameter limit.
  Empty `__in` (e.g. `?asn__in=`) returns `{"data":[],"meta":{}}`
  with no SQL executed, matching Django ORM `Model.objects.filter(id__in=[])`
  semantics.

- **pdbcompat fuzz corpus** extended with 21 non-ASCII and `__in`
  edge-case seeds (diacritics, CJK, RTL, RLO/LRO overrides, ZWJ,
  combining marks, null bytes, 70 KB literals, 1201-element `__in`,
  empty `__in`, all-empty `__in` parts). Local 60s run on a Ryzen 5
  3600 logged 469k executions / 65 new interesting / zero panics.

- **Cross-entity `__` traversal in pdbcompat.** The
  `/api/<type>?<fk>__<field>=` and
  `/api/<type>?<fk>__<fk>__<field>=` query shapes now resolve across
  foreign-key edges, mirroring upstream PeeringDB's `prepare_query`
  allowlists (Path A) and `queryable_relations()` auto-introspection
  (Path B). Hard-capped at 2 hops. Every 13-entity
  allowlist was translated 1:1 from `peeringdb_server/serializers.py`
  (SHA `99e92c72`); each annotation carries a `serializers.py:<line>`
  source comment for audit. A new codegen tool `cmd/pdb-compat-allowlist`
  reads ent schema annotations and emits
  `internal/pdbcompat/allowlist_gen.go` (Path A allowlists + Path B
  `Edges` map) wired into `go generate ./...` after ent codegen and
  before buf codegen. Example 2-hop case working:
  `GET /api/fac?ixlan__ix__fac_count__gt=0`.

- **Unknown filter fields silently ignored.**
  `GET /api/net?totally_unknown_field=x` returns HTTP 200 with a
  silently-unfiltered result rather than 400, matching upstream
  `rest.py:544-662`. Operators gain DEBUG-level visibility via
  `slog.DebugContext("pdbcompat: unknown filter fields silently ignored", ...)`
  and OTel span attribute
  `pdbplus.filter.unknown_fields` (CSV of all unknowns per request).
  The same diagnostic fires for typos, deprecated field names, and
  filter keys with >2 `__`-separated relation segments (the 2-hop cap).

- **2-hop cost ceiling (<50ms/op @ 10k rows).** New
  `BenchmarkTraversal_*` in `internal/pdbcompat/bench_traversal_test.go`
  plus a go-test-time `TestBenchTraversal_TwoHopCeiling` gate guard the
  2-hop query cost ceiling. A nightly CI workflow
  (`.github/workflows/bench.yml`) regression-gates via benchstat —
  prevents a future Cartesian-join regression from landing silently.

- **Memory-safe response paths on 256 MB replicas.**
  - **Streaming JSON emission** for pdbcompat list responses —
    `internal/pdbcompat/stream.go` `StreamListResponse` writes
    `{"meta":…,"data":[…]}` token-by-token via per-row `json.Marshal`
    and `http.Flusher.Flush()` every 100 rows. Replaces the legacy
    full-slice `json.NewEncoder` materialisation on the `serveList`
    path.
  - **`PDBPLUS_RESPONSE_MEMORY_LIMIT` env var** (default 128 MiB =
    256 MB replica − 80 MB Go runtime baseline − 48 MB slack). Gates
    response size via a pre-flight `SELECT COUNT(*) × typical_row_bytes`
    heuristic in `internal/pdbcompat/budget.go` `CheckBudget`.
    Over-budget requests receive RFC 9457
    `application/problem+json` 413 with `max_rows`, `budget_bytes`,
    and a human-readable `detail` string BEFORE any row data is
    fetched. Unit suffix required (`KB`/`MB`/`GB`/`TB`); the literal
    `0` disables the check for local development. No `Retry-After` —
    413 is request-shape, not transient. Per-entity
    `typical_row_bytes` calibrated via `BenchmarkRowSize_*` and
    doubled for headroom (13 types × 2 depths in
    `internal/pdbcompat/rowsize.go`).
  - **Per-request heap-delta telemetry.** New OTel span attribute
    `pdbplus.response.heap_delta_kib` (sampled once at handler entry
    and once at exit via `defer`; STW ~µs, NEVER per row) plus
    Prometheus histogram
    `pdbplus_response_heap_delta_kib{endpoint,entity}`. Registered
    via `pdbotel.InitResponseHeapHistogram()`. Grafana gains a
    "Response Heap Delta (KiB) — p50/p95/p99 by endpoint" panel (id
    36) at the bottom of the sync-memory watch row in
    `deploy/grafana/dashboards/pdbplus-overview.json`.
  - **`docs/ARCHITECTURE.md` § Response Memory Envelope** documents
    the envelope derivation, the three moving parts (stream / rowsize
    / budget), a per-entity worst-case sizing table with computed
    `max_rows` at the 128 MiB default, the request lifecycle, and the
    telemetry wire-up. `CLAUDE.md` gains a sibling § Response memory
    envelope convention with the maintainer checklist for
    adding new entity types.

- **Upstream parity regression lock-in.**
  - **`internal/pdbcompat/parity/` category-split regression suite** —
    6 test files (`ordering_test.go`, `status_test.go`,
    `limit_test.go`, `unicode_test.go`, `in_test.go`,
    `traversal_test.go`) + a shared `harness_helpers_test.go`. 31
    hard-pass tests total: 27 v1.16-semantic sub-tests covering the
    default-ordering, status, limit, Unicode, `__in`, and traversal
    behaviours plus 4 harness probes. 2 explicit
    `DIVERGENCE_` sub-tests lock the v1.16 silent-ignore semantic for
    the 3-hop `fac?ixlan__ix__fac_count__gt=0` case and the
    HTTP 500 outcome for the `fac?campus__name=X` case. 15
    `pdb_api_test.py`-or-synthesised citation hits, 36 `t.Parallel()`
    call sites, 4 `DIVERGENCE` markers.
    Suite wall time 15.4s under `-race`.
  - **`cmd/pdb-fixture-port/` fixture-porting tool** reads upstream
    `src/peeringdb_server/management/commands/pdb_api_test.py` and
    emits Go fixture literals into
    `internal/testutil/parity/fixtures.go`. 5560 ported rows across 6
    category vars pinned to `peeringdb/peeringdb@99e92c72` (full SHA
    `99e92c726172ead7d224ce34c344eff0bccb3e63`) with
    `sha256:75c7a6fab734db7…` source-file hash recorded in the
    generated header. `--upstream-commit <sha>` override preserves the
    pinned SHA during snapshot-replay regeneration;
    `--check` flag compares current upstream against the pinned SHA
    and reports drift (advisory only, not blocking).
  - **`internal/pdbcompat/parity/bench_test.go` performance lock-in** —
    3 `b.Loop()`-style benchmarks:
    `BenchmarkParity_TwoHopTraversal` (`ixpfx?ixlan__ix__id=20`,
    ~580μs/op on Ryzen 5 3600),
    `BenchmarkParity_LimitZeroStreaming` (5000-row seeded end-to-end
    `stream.go` path, ~82.7ms/op),
    `BenchmarkParity_InFiveThousandElements` (5001-id IN via the
    `json_each` rewrite, ~98.6ms/op). Benchstat-on-main is out of
    scope — benchmarks are local-run only, gated
    by the standard `go test -race ./...` tier. `testing.TB` widening
    plumbed through `testutil.SetupClient` + 9 parity harness helpers
    (type-only change, every `*testing.T` call site still satisfies
    the interface).
  - **`docs/API.md § Known Divergences` extended** with 3 new rows
    (pre-soft-delete hard-delete gap parity cross-ref; pdbfe `limit=0`
    count-only invalid claim; depth-on-list silent-drop
    guardrail) plus `TestParity_*` cross-refs appended to the Since
    columns of the existing `fac?campus__name=X` +
    `fac?ixlan__ix__fac_count__gt=0` divergence rows.
  - **`docs/API.md § Validation Notes` NEW sub-section** documenting
    5 invalid third-party claims about upstream behaviour with pinned
    `peeringdb/peeringdb@99e92c72…` SHA refs: (1) `net?country=NL` is
    not a valid filter (country lives on `org`), (2) `?limit=0` is
    unlimited not count-only, (3) default ordering is
    `(-updated, -created)` not `id ASC`, (4) Unicode folding is Python
    `unidecode` not MySQL collation, (5) filter surface is
    `prepare_query` + `queryable_relations` not a DRF `filterset_class`.
    Each row cites the specific upstream file:line and cross-
    references the parity sub-test locking the corrected behaviour.
    Future conformance audits against pdbfe's gotchas doc don't re-
    research the same invalid claims.

### Changed

- **Sync now soft-deletes** instead of hard-deleting. The 13
  `deleteStale*` functions in `internal/sync/delete.go` were renamed
  to `markStaleDeleted*`; they run
  `UPDATE ... SET status='deleted', updated=<cycle_start>` per sync
  cycle. One `cycleStart` timestamp is stamped on every tombstone
  within a cycle so `?since=N` windows stay atomic. Tombstone
  garbage-collection policy is deferred as dormant work (planted
  2026-04-19).

- **`parseFieldOp` signature extended** in
  `internal/pdbcompat/filter.go`. Return tuple expanded from
  `(field, op string)` to `(relationSegments []string, finalField,
  op string)` so the parser can detect `<fk>__<field>` patterns before
  consulting Path A / Path B and enforce the 2-hop cap.
  Internal-only — no callers exist outside `internal/pdbcompat`.

- **`ParseFilters` gains a context-aware sibling.** New
  `ParseFiltersCtx(ctx, params, tc)` threads an unknown-field
  accumulator via `context.Value` so the handler emits one aggregated
  `slog.DebugContext` call and a single OTel span attribute per
  request, rather than one per unknown field. Legacy `ParseFilters`
  kept as a shim for existing call sites.

### Deprecated

- `PDBPLUS_INCLUDE_DELETED` (see Breaking above; removal completes
  with fatal startup error in v1.17).

### Fixed

- `?limit=0` on pdbcompat list endpoints previously fell back to
  `DefaultLimit=250`. Now returns all rows up to any other filter,
  matching upstream behaviour (`rest.py:734-737`).

### Known issues

- **One-time ASCII-only window for diacritic-insensitive matching.**
  Between v1.16 deploy and the first post-deploy sync cycle (≤1h with
  the default `PDBPLUS_SYNC_INTERVAL=1h`), rows synced before the
  upgrade have `<field>_fold = ''` and return no match for non-ASCII
  queries against `__contains` / `__startswith` on searchable text
  fields. ASCII queries continue to work via the existing non-folded
  columns throughout the window. No manual backfill is required — the
  next standard sync cycle rewrites every affected row via the
  `OnConflict().UpdateNewValues()` path. See
  [`docs/API.md` § Known Divergences](./docs/API.md#known-divergences).

- **Unknown filter fields silently ignored is a feature, not a bug.**
  Typos (`?nmae=x`), deprecated field names,
  and filter keys with >2 `__`-separated relation segments do not
  return HTTP 400 — the filter is silently dropped and the response
  contains the full unfiltered result set. This matches upstream
  PeeringDB (`rest.py:544-662`) and preserves existing client
  integrations that probe field names. Clients that want strict
  validation should inspect the OTel span attribute
  `pdbplus.filter.unknown_fields` or enable DEBUG-level logging to
  surface the dropped keys.

- **`campus` edge table-name codegen bug.**
  `cmd/pdb-compat-allowlist` emits `TargetTable: "campus"` instead
  of the correct `"campuses"` for all edges targeting the Campus
  entity, because `entc.LoadGraph` does not apply the
  `fixCampusInflection` patch used by the ent runtime codegen. Affected
  queries (e.g. `GET /api/fac?campus__name=X`) return
  `500 SQL logic error: no such table: campus (1)`. The outgoing
  edges FROM Campus are correct. Documented one-time gap; fix
  scheduled as a follow-up (preferred approach: add
  `entsql.Annotation{Table: "campuses"}` to `ent/schema/campus.go`).

- **`fac?ixlan__ix__fac_count__gt=0` (`pdb_api_test.py:2340`) is
  silent-ignored** — requires 3-hop traversal via `ixfac` which
  exceeds the documented 2-hop cap; the parity suite locks this as a
  documented divergence. The
  generic 2-hop mechanism works for entity pairs with direct edges
  (e.g. `ixpfx?ixlan__ix__id=20`).

[Unreleased]: https://github.com/dotwaffle/peeringdb-plus/compare/v1.32.2...HEAD
[1.32.2]: https://github.com/dotwaffle/peeringdb-plus/compare/v1.32.1...v1.32.2
[1.32.1]: https://github.com/dotwaffle/peeringdb-plus/compare/v1.32.0...v1.32.1
[1.32.0]: https://github.com/dotwaffle/peeringdb-plus/compare/v1.31.1...v1.32.0
[1.31.1]: https://github.com/dotwaffle/peeringdb-plus/compare/v1.31.0...v1.31.1
[1.31.0]: https://github.com/dotwaffle/peeringdb-plus/compare/v1.30.0...v1.31.0
[1.30.0]: https://github.com/dotwaffle/peeringdb-plus/compare/v1.29.0...v1.30.0
[1.29.0]: https://github.com/dotwaffle/peeringdb-plus/compare/v1.28.5...v1.29.0
[1.28.5]: https://github.com/dotwaffle/peeringdb-plus/compare/v1.28.4...v1.28.5
[1.28.4]: https://github.com/dotwaffle/peeringdb-plus/compare/v1.28.3...v1.28.4
[1.28.3]: https://github.com/dotwaffle/peeringdb-plus/compare/v1.28.2...v1.28.3
[1.28.2]: https://github.com/dotwaffle/peeringdb-plus/compare/v1.28.1...v1.28.2
[1.28.1]: https://github.com/dotwaffle/peeringdb-plus/compare/v1.28.0...v1.28.1
[1.28.0]: https://github.com/dotwaffle/peeringdb-plus/compare/v1.27.0...v1.28.0
[1.27.0]: https://github.com/dotwaffle/peeringdb-plus/compare/v1.26.0...v1.27.0
[1.26.0]: https://github.com/dotwaffle/peeringdb-plus/compare/v1.25.0...v1.26.0
[1.25.0]: https://github.com/dotwaffle/peeringdb-plus/compare/v1.24.1...v1.25.0
[1.24.1]: https://github.com/dotwaffle/peeringdb-plus/compare/v1.24.0...v1.24.1
[1.24.0]: https://github.com/dotwaffle/peeringdb-plus/compare/v1.23.0...v1.24.0
[1.23.0]: https://github.com/dotwaffle/peeringdb-plus/compare/v1.22.0...v1.23.0
[1.22.0]: https://github.com/dotwaffle/peeringdb-plus/compare/v1.21.1...v1.22.0
[1.21.1]: https://github.com/dotwaffle/peeringdb-plus/compare/v1.21.0...v1.21.1
[1.21.0]: https://github.com/dotwaffle/peeringdb-plus/compare/v1.20.6...v1.21.0
[1.20.6]: https://github.com/dotwaffle/peeringdb-plus/compare/v1.20.5...v1.20.6
[1.20.5]: https://github.com/dotwaffle/peeringdb-plus/compare/v1.20.4...v1.20.5
[1.20.4]: https://github.com/dotwaffle/peeringdb-plus/compare/v1.20.3...v1.20.4
[1.20.3]: https://github.com/dotwaffle/peeringdb-plus/compare/v1.20.2...v1.20.3
[1.20.2]: https://github.com/dotwaffle/peeringdb-plus/compare/v1.20.1...v1.20.2
[1.20.1]: https://github.com/dotwaffle/peeringdb-plus/compare/v1.20.0...v1.20.1
[1.20.0]: https://github.com/dotwaffle/peeringdb-plus/compare/v1.19.5...v1.20.0
[1.19.5]: https://github.com/dotwaffle/peeringdb-plus/compare/v1.19.4...v1.19.5
[1.19.4]: https://github.com/dotwaffle/peeringdb-plus/compare/v1.19.3...v1.19.4
[1.19.3]: https://github.com/dotwaffle/peeringdb-plus/compare/v1.19.2...v1.19.3
[1.19.2]: https://github.com/dotwaffle/peeringdb-plus/compare/v1.19.1...v1.19.2
[1.19.1]: https://github.com/dotwaffle/peeringdb-plus/compare/v1.19.0...v1.19.1
[1.19.0]: https://github.com/dotwaffle/peeringdb-plus/compare/v1.18.14...v1.19.0
[1.16.0]: https://github.com/dotwaffle/peeringdb-plus/releases/tag/v1.16.0
