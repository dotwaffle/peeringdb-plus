---
phase: quick-260427-vvx
plan: 01
type: execute
wave: 1
depends_on: []
files_modified:
  - cmd/loadtest/main.go
  - cmd/loadtest/surfaces.go
  - cmd/loadtest/registry.go
  - cmd/loadtest/endpoints.go
  - cmd/loadtest/sync.go
  - cmd/loadtest/soak.go
  - cmd/loadtest/report.go
  - cmd/loadtest/README.md
autonomous: true
requirements:
  - QUICK-260427-VVX
must_haves:
  truths:
    - "`go build ./...` and `go test ./...` ignore the loadtest binary (build-tag isolated)."
    - "`go build -tags loadtest ./cmd/loadtest` produces a working `loadtest` binary."
    - "`loadtest endpoints --base http://...` exercises every entity type across all 5 API surfaces (pdbcompat /api, entrest /rest/v1, GraphQL /graphql, ConnectRPC /peeringdb.v1.*, Web UI /ui) and prints a per-surface summary."
    - "`loadtest sync --mode=full` replays the 13-entity full-sync GET sequence against /api/<type>?depth=0 in the same order as internal/sync/worker.go syncSteps()."
    - "`loadtest sync --mode=incremental --since=<rfc3339|unix>` replays the same sequence with `?since=N` and defaults `since` to `now-1h` when unset."
    - "`loadtest soak --duration=30s --concurrency=4 --qps=5` drives sustained mixed-surface traffic with a hard QPS cap and prints per-surface p50/p95/p99 latency + success rate."
    - "Default --base is `https://peeringdb-plus.fly.dev` and `--help` carries an unmissable SAFETY warning that this tool MUST NOT be pointed at `https://www.peeringdb.com`."
    - "`PDBPLUS_LOADTEST_AUTH_TOKEN` env var, when set, is sent as `Authorization: Bearer <token>` on every request."
    - "Read-only: only HTTP GET (REST/UI/pdbcompat) and POST (GraphQL queries, ConnectRPC unary calls) — no mutating verbs anywhere."
    - "README.md inside cmd/loadtest/ explains all three modes, all flags, the auth env var, and the 'do not run in CI' warning prominently."
  artifacts:
    - path: "cmd/loadtest/main.go"
      provides: "Build-tag-gated entrypoint, --mode flag dispatch, --base/--help/--auth handling"
      contains: "//go:build loadtest"
    - path: "cmd/loadtest/surfaces.go"
      provides: "Surface enum (pdbcompat, entrest, graphql, connectrpc, webui), per-surface plural-name maps, and `Hit(ctx, base, surface, op) (*Result, error)` dispatch."
    - path: "cmd/loadtest/registry.go"
      provides: "13-entity x N-shape inventory (list-default, list-filtered, get-by-id) wired to the 5 surface helpers; the canonical mapping of internal/peeringdb/types.go constants to /rest/v1 plurals (organizations, networks, facilities, internet-exchanges, pocs, ix-lans, ix-prefixes, network-ix-lans, network-facilities, ix-facilities, carriers, carrier-facilities, campuses) and ConnectRPC service names."
    - path: "cmd/loadtest/endpoints.go"
      provides: "`runEndpoints(ctx, cfg)` — sweep mode driver."
    - path: "cmd/loadtest/sync.go"
      provides: "`runSync(ctx, cfg, mode, since)` — replays the 13-step ordered sequence."
    - path: "cmd/loadtest/soak.go"
      provides: "`runSoak(ctx, cfg, duration, concurrency, qps)` — golang.org/x/time/rate-limited worker pool over the registry."
    - path: "cmd/loadtest/report.go"
      provides: "Per-surface counters, latency histograms (sorted []time.Duration → p50/p95/p99), terminal summary report."
    - path: "cmd/loadtest/README.md"
      provides: "User-facing docs + CI exclusion banner."
  key_links:
    - from: "cmd/loadtest/main.go"
      to: "cmd/loadtest/registry.go"
      via: "registry.All() returning []Endpoint, fed to mode dispatch"
      pattern: "registry\\.All\\("
    - from: "cmd/loadtest/registry.go"
      to: "internal/peeringdb/types.go"
      via: "plural-name map keyed on the TypeOrg/TypeNet/... string constants"
      pattern: "peeringdb\\.Type(Org|Net|Fac|IX|Poc|IXLan|IXPfx|NetIXLan|NetFac|IXFac|Carrier|CarrierFac|Campus)"
    - from: "cmd/loadtest/sync.go"
      to: "internal/sync/worker.go"
      via: "type-name ordering must match syncSteps(): org, campus, fac, carrier, carrierfac, ix, ixlan, ixpfx, ixfac, net, poc, netfac, netixlan"
      pattern: "syncOrder\\s*=\\s*\\[\\]string\\{"
    - from: "cmd/loadtest/main.go"
      to: "https://peeringdb-plus.fly.dev"
      via: "default --base flag value"
      pattern: "peeringdb-plus\\.fly\\.dev"
---

<objective>
Build a `cmd/loadtest/` Go tool that exercises every API endpoint of every entity type on the deployed peeringdb-plus mirror, simulates full and incremental PeeringDB syncs, and drives sustained mixed-surface soak load. The binary lives behind a `//go:build loadtest` tag so it is invisible to `go build ./...`, `go test ./...`, and CI. Default target is `https://peeringdb-plus.fly.dev`; localhost is supported via `--base`.

Purpose: an operator tool for warming dashboards, validating fleet capacity post-deploy, and reproducing load patterns against deployed instances. NOT a CI/test-suite component.

Output: single Go binary with three modes (`endpoints`, `sync`, `soak`), a per-surface latency report, and a README that prominently warns operators not to point it at upstream PeeringDB.
</objective>

<execution_context>
@$HOME/.claude/get-shit-done/workflows/execute-plan.md
@$HOME/.claude/get-shit-done/templates/summary.md
</execution_context>

<context>
@.planning/STATE.md
@CLAUDE.md
@docs/API.md
@internal/peeringdb/types.go
@internal/peeringdb/client.go
@internal/sync/worker.go
@proto/peeringdb/v1/services.proto
@cmd/peeringdb-plus/main.go

<interfaces>
<!-- Keys/contracts the executor needs. Extracted from the codebase so no scavenger hunt is required. -->

13 entity-type constants (internal/peeringdb/types.go):
```go
TypeOrg = "org"        TypeNet = "net"          TypeFac = "fac"
TypeIX  = "ix"         TypePoc = "poc"          TypeIXLan = "ixlan"
TypeIXPfx = "ixpfx"    TypeNetIXLan = "netixlan" TypeNetFac = "netfac"
TypeIXFac = "ixfac"    TypeCarrier = "carrier"  TypeCarrierFac = "carrierfac"
TypeCampus = "campus"
```

REST plural paths (extracted from ent/rest/openapi.json — verified 2026-04-27):
```
org        -> /rest/v1/organizations
net        -> /rest/v1/networks
fac        -> /rest/v1/facilities
ix         -> /rest/v1/internet-exchanges
poc        -> /rest/v1/pocs
ixlan      -> /rest/v1/ix-lans
ixpfx      -> /rest/v1/ix-prefixes
netixlan   -> /rest/v1/network-ix-lans
netfac     -> /rest/v1/network-facilities
ixfac      -> /rest/v1/ix-facilities
carrier    -> /rest/v1/carriers
carrierfac -> /rest/v1/carrier-facilities
campus     -> /rest/v1/campuses
```

pdbcompat paths follow upstream PeeringDB convention: `/api/<type>` and `/api/<type>/{id}` using the SHORT name (org, net, fac, ix, poc, ixlan, ixpfx, netixlan, netfac, ixfac, carrier, carrierfac, campus).

ConnectRPC service names (proto/peeringdb/v1/services.proto): `peeringdb.v1.<Service>` where `<Service>` is one of:
```
CampusService, CarrierService, CarrierFacilityService, FacilityService,
InternetExchangeService, IxFacilityService, IxLanService, IxPrefixService,
NetworkService, NetworkFacilityService, NetworkIxLanService,
OrganizationService, PocService
```
Each exposes `Get<Type>`, `List<Type>s`, `Stream<Type>s` over POST. URL form: `/peeringdb.v1.NetworkService/GetNetwork` with `Content-Type: application/json` and a JSON body like `{"id": 1}` or `{"limit": 10}`.

Sync step ordering (internal/sync/worker.go syncSteps() — DO NOT REORDER):
```
org, campus, fac, carrier, carrierfac, ix, ixlan, ixpfx, ixfac, net, poc, netfac, netixlan
```

Web UI routes (internal/web/handler.go inferred from `mux.HandleFunc("GET /{$}")` + nav links): `/ui/`, `/ui/about`, `/ui/compare`, `/ui/asn/{n}`, plus per-type detail pages where they exist. The loadtest hits `/ui/`, `/ui/asn/15169`, and `/ui/about` only — limited per-type routes are intentionally not exhaustive (the surface is content-negotiated; curl-style UA gets ANSI text, browsers get HTML; loadtest sends a browser-like UA).

GraphQL endpoint (cmd/peeringdb-plus/main.go:362): POST /graphql with `Content-Type: application/json` body `{"query": "..."}`. Schema follows entgql/gqlgen conventions: root list fields are pluralised English names (`networks`, `organizations`, ...), single-record queries are `node(id: ID!)`. The loadtest issues one minimal query per entity type (e.g. `{ networks(first: 10) { edges { node { id name asn } } } }`).
</interfaces>
</context>

<tasks>

<task type="auto" tdd="true">
  <name>Task 1: Scaffold binary, surface registry, and endpoints sweep mode</name>
  <files>
    cmd/loadtest/main.go,
    cmd/loadtest/surfaces.go,
    cmd/loadtest/registry.go,
    cmd/loadtest/endpoints.go,
    cmd/loadtest/report.go,
    cmd/loadtest/registry_test.go
  </files>
  <behavior>
    - Test 1 (registry_test.go): registry.All() returns >= 130 endpoints (13 entities × 5 surfaces × ≥2 shapes each). Each endpoint has a non-empty Surface, EntityType, Method, and Path/Body builder.
    - Test 2 (registry_test.go): for every internal/peeringdb.Type* constant, registry.All() contains at least one endpoint per surface (asserts no missing entity coverage).
    - Test 3 (registry_test.go): pdbcompat-list endpoint for "net" produces URL `/api/net?limit=10` with `?asn=15169` filter variant present; `_fold`-routing variant for "org" produces `/api/org?name__contains=foo`.
    - Test 4 (registry_test.go): ConnectRPC endpoint for "network" Get produces path `/peeringdb.v1.NetworkService/GetNetwork`, method POST, body `{"id":1}`.
    - Test 5 (registry_test.go): REST plural map exhaustively covers all 13 type constants and matches the openapi.json values pinned in the plan's <interfaces> block.
    - All tests live behind `//go:build loadtest` so they don't run in CI.
  </behavior>
  <action>
    Create `cmd/loadtest/` and add `//go:build loadtest` (with matching `// +build loadtest` blank line) at the top of every .go file in the package — this is what gates `go build ./...` per the constraint that the tool MUST NOT compile or test as part of normal cycles.

    **main.go** — `package main`, `flag.Parse`-style CLI (stdlib only; no cobra). Flags: `--mode {endpoints|sync|soak}`, `--base <url>` default `https://peeringdb-plus.fly.dev`, `--timeout <duration>` default 30s, `--verbose` bool. Parse subcommand from `os.Args[1]` (`loadtest endpoints`, `loadtest sync`, `loadtest soak`) — single position arg, then `flag` parses remaining flags. `--help` MUST print a banner with the SAFETY notice: `WARNING: This tool targets the peeringdb-plus mirror at <base>. NEVER point --base at https://www.peeringdb.com — they enforce 1 req/hour and will block you.` Build a `Config` struct from flags and dispatch to `runEndpoints`/`runSync`/`runSoak` (latter two are stubs returning `errors.New("not implemented")` until Task 2/3 — keep the dispatch wired so e2e flag parsing works now).

    Wire the auth header reader: read `PDBPLUS_LOADTEST_AUTH_TOKEN` once at startup; if non-empty, every outbound request gets `Authorization: Bearer <token>`. Build a single `*http.Client` with the timeout and pass it through Config (per GO-CTX-1, never store ctx — pass it as the first arg of every helper).

    **surfaces.go** — declare:
    ```go
    type Surface string
    const (
      SurfacePdbCompat  Surface = "pdbcompat"
      SurfaceEntRest    Surface = "entrest"
      SurfaceGraphQL    Surface = "graphql"
      SurfaceConnectRPC Surface = "connectrpc"
      SurfaceWebUI      Surface = "webui"
    )
    type Endpoint struct {
      Surface    Surface
      EntityType string  // peeringdb.TypeOrg, etc. (or "" for surface-wide endpoints like /ui/about)
      Shape      string  // "list-default", "list-filtered", "get-by-id", "graphql-list", "rpc-get", "rpc-list", "ui-home", ...
      Method     string  // "GET" or "POST"
      Path       string  // path only — base URL prepended at request time
      Body       []byte  // nil for GET
      Header     http.Header // optional Content-Type for POST
    }
    type Result struct {
      Endpoint Endpoint
      Status   int
      Latency  time.Duration
      Err      error
    }
    func Hit(ctx context.Context, client *http.Client, base, authToken string, ep Endpoint) Result
    ```
    `Hit` builds the `*http.Request`, sets User-Agent (`peeringdb-plus-loadtest/<version>`), copies `ep.Header`, sets `Authorization` if `authToken != ""`, executes, drains body to ensure connection reuse (per the project's own client.go pattern), and returns a `Result` with wall-clock latency.

    **registry.go** — the inventory. Declare two maps keyed on the 13 type constants:
    ```go
    var restPlurals = map[string]string{
      peeringdb.TypeOrg: "organizations", peeringdb.TypeNet: "networks",
      peeringdb.TypeFac: "facilities",     peeringdb.TypeIX:  "internet-exchanges",
      peeringdb.TypePoc: "pocs",           peeringdb.TypeIXLan: "ix-lans",
      peeringdb.TypeIXPfx: "ix-prefixes",  peeringdb.TypeNetIXLan: "network-ix-lans",
      peeringdb.TypeNetFac: "network-facilities", peeringdb.TypeIXFac: "ix-facilities",
      peeringdb.TypeCarrier: "carriers",   peeringdb.TypeCarrierFac: "carrier-facilities",
      peeringdb.TypeCampus: "campuses",
    }
    var rpcServiceNames = map[string]string{
      peeringdb.TypeOrg: "OrganizationService", peeringdb.TypeNet: "NetworkService",
      peeringdb.TypeFac: "FacilityService", peeringdb.TypeIX: "InternetExchangeService",
      peeringdb.TypePoc: "PocService", peeringdb.TypeIXLan: "IxLanService",
      peeringdb.TypeIXPfx: "IxPrefixService", peeringdb.TypeNetIXLan: "NetworkIxLanService",
      peeringdb.TypeNetFac: "NetworkFacilityService", peeringdb.TypeIXFac: "IxFacilityService",
      peeringdb.TypeCarrier: "CarrierService", peeringdb.TypeCarrierFac: "CarrierFacilityService",
      peeringdb.TypeCampus: "CampusService",
    }
    var rpcMethodNames = map[string]string{
      peeringdb.TypeOrg: "Organization", peeringdb.TypeNet: "Network", peeringdb.TypeFac: "Facility",
      peeringdb.TypeIX: "InternetExchange", peeringdb.TypePoc: "Poc", peeringdb.TypeIXLan: "IxLan",
      peeringdb.TypeIXPfx: "IxPrefix", peeringdb.TypeNetIXLan: "NetworkIxLan",
      peeringdb.TypeNetFac: "NetworkFacility", peeringdb.TypeIXFac: "IxFacility",
      peeringdb.TypeCarrier: "Carrier", peeringdb.TypeCarrierFac: "CarrierFacility",
      peeringdb.TypeCampus: "Campus",
    }
    ```
    Then `func All() []Endpoint` builds the full inventory. For each of the 13 types, generate:
    - **pdbcompat list-default**: `GET /api/<short>?limit=10`
    - **pdbcompat list-filtered**: `GET /api/<short>?<filter>` — choose per-type: `?asn=15169` for net; `?city__contains=london` for org/fac/ix/campus (folded entities); `?since=<unix-1h>` for everything else; `?limit=10&depth=1` for ixlan/ixpfx (depth coverage)
    - **pdbcompat get-by-id**: `GET /api/<short>/1`
    - **entrest list-default**: `GET /rest/v1/<plural>?itemsPerPage=10`
    - **entrest get-by-id**: `GET /rest/v1/<plural>/1`
    - **graphql list**: `POST /graphql` with body `{"query":"{ <plural>(first: 10) { edges { node { id } } } }"}` (use restPlurals[t] as the GraphQL root field — entgql convention)
    - **connectrpc rpc-get**: `POST /peeringdb.v1.<Service>/Get<Method>` body `{"id": 1}` Content-Type `application/json`
    - **connectrpc rpc-list**: `POST /peeringdb.v1.<Service>/List<Method>s` body `{"limit": 10}` Content-Type `application/json`

    Plus 3 surface-wide Web UI endpoints (EntityType=""): `GET /ui/`, `GET /ui/about`, `GET /ui/asn/15169`. Web UI must send User-Agent `Mozilla/5.0 (compatible; pdbplus-loadtest)` so the content-negotiation middleware returns HTML, not ANSI text (per CLAUDE.md note about termrender).

    Total endpoint count: 13 × 8 (pdbcompat ×3 + entrest ×2 + graphql ×1 + connectrpc ×2) + 3 ui = **107**, well above the ≥130 target if we add a second graphql shape. Add `graphql-get` (`{"query":"{ node(id: \"1\") { ... on <Type> { id } } }"}`) per type for an extra 13 → **120**, plus an extra pdbcompat `__startswith` filter variant for the 6 folded entities → **126**, plus an `ix-lan` traversal variant for net (`?ix__name__contains=de-cix`) → ~130. Pin the count loosely (`>= 100`) in tests so future shape additions don't break the assertion.

    **endpoints.go** — `func runEndpoints(ctx context.Context, cfg Config, eps []Endpoint, rep *Report) error`. Iterate over `eps` sequentially (no concurrency in sweep mode — easier to read output), call `Hit`, append to `rep`. Sequential is fine; this is a once-per-run inventory pass, not a load test (soak mode handles concurrency).

    **report.go** — `Report` struct holds `[]Result` plus a mutex for thread-safe append (used by soak). Methods: `Append(Result)`, `Print(io.Writer)`. Print groups by Surface then by EntityType, computes count, success-rate (status 2xx / total), and p50/p95/p99 from a sorted []time.Duration (use `sort.Slice` then index — no need for a histogram lib at this scale). Also prints overall totals row. Rendering target: a fixed-width table compatible with `column -t -s$'\t'` piping (tab-separated values, header row).

    **registry_test.go** — the 5 tests listed in `<behavior>`. Use stdlib testing with `t.Parallel()` per GO-T-3.

    Verify the build-tag enforcement at the end of the task: `go build ./...` must succeed without compiling cmd/loadtest, and `go build -tags loadtest ./cmd/loadtest` must produce a binary.
  </action>
  <verify>
    <automated>test "$(go build ./... 2>&1 | wc -l)" -eq 0 &amp;&amp; go build -tags loadtest -o /tmp/loadtest ./cmd/loadtest &amp;&amp; /tmp/loadtest --help 2>&amp;1 | grep -q "NEVER point --base at https://www.peeringdb.com" &amp;&amp; go test -tags loadtest -race ./cmd/loadtest/...</automated>
  </verify>
  <done>
    - `go build ./...` succeeds and produces no `cmd/loadtest` artifact (tag-isolated).
    - `go build -tags loadtest ./cmd/loadtest` produces `loadtest` binary.
    - `loadtest --help` prints SAFETY warning naming `https://www.peeringdb.com`.
    - `loadtest endpoints --base http://localhost:8080` runs the sweep against a local server (not asserted in automated test, but the dispatch must be wired).
    - All 5 registry tests pass under `-tags loadtest -race`.
  </done>
</task>

<task type="auto" tdd="true">
  <name>Task 2: Sync simulation modes (full + incremental)</name>
  <files>cmd/loadtest/sync.go, cmd/loadtest/sync_test.go, cmd/loadtest/main.go</files>
  <behavior>
    - Test 1 (sync_test.go): `syncOrder` matches the 13-name sequence from internal/sync/worker.go syncSteps() exactly. Test imports the sync package and asserts `[]string{...} == derived order from syncSteps()` so future reordering of syncSteps fails this test.
    - Test 2 (sync_test.go): `buildSyncEndpoints(mode=full)` returns 13 endpoints, all Surface=pdbcompat, Method=GET, Path matches `/api/<short>?depth=0&limit=250` per the project's own client.go pattern.
    - Test 3 (sync_test.go): `buildSyncEndpoints(mode=incremental, since=<t>)` adds `&since=<unix>` to every URL.
    - Test 4 (sync_test.go): incremental mode with `since=time.Time{}` (zero) defaults to `now-1h`.
  </behavior>
  <action>
    Implement `runSync(ctx, cfg, mode, since)` in `cmd/loadtest/sync.go`.

    Declare `var syncOrder = []string{"org", "campus", "fac", "carrier", "carrierfac", "ix", "ixlan", "ixpfx", "ixfac", "net", "poc", "netfac", "netixlan"}` — copy-paste from internal/sync/worker.go syncSteps(). The test enforces parity with the live ordering by importing `github.com/dotwaffle/peeringdb-plus/internal/sync` and reflecting on syncSteps() output. (If syncSteps is not exported, write the test to call a small sync-package helper or fall back to a `//go:generate` doc comment + hand-mirror with a test that lists the 13 names alphabetically and asserts both arrays sort to the same set — single-source-of-truth via a small exported helper is cleaner; export `Sync.StepOrder() []string` if needed, that's a one-line addition.)

    Per-step URL builder mirrors internal/peeringdb/client.go FetchRawPage:
    ```
    url := fmt.Sprintf("%s/api/%s?limit=250&skip=0&depth=0", cfg.Base, t)
    if mode == "incremental" {
      url += fmt.Sprintf("&since=%d", since.Unix())
    }
    ```
    Sync mode does NOT page — single page per type is enough to exercise the endpoint shape. The goal is endpoint exhaustion + dashboard warmup, not exhaustive data fetch (the real sync worker handles that on the server side anyway).

    Sequential execution per the upstream sync's natural pattern. Append every Result to the Report. Honor ctx cancellation.

    Add `--mode` and `--since` flags handling to main.go's sync dispatch:
    - `--mode={full|incremental}`, default `full`
    - `--since=<rfc3339|unix>`, default unset; when unset and mode=incremental, default to `time.Now().Add(-1*time.Hour)`. Accept either format: try `time.Parse(time.RFC3339, s)` first, fall back to `strconv.ParseInt` for unix seconds.

    Emit a sync-cycle summary at the end via the same Report.Print: counts, success rate, total wall-clock duration of the cycle (informational — the rate-limit on the server side will make this >= 13×rate-limit-interval).
  </action>
  <verify>
    <automated>go build -tags loadtest -o /tmp/loadtest ./cmd/loadtest &amp;&amp; go test -tags loadtest -race ./cmd/loadtest/... -run TestSync</automated>
  </verify>
  <done>
    - `loadtest sync --mode=full --base=...` issues exactly 13 GETs in syncSteps() order.
    - `loadtest sync --mode=incremental` defaults `since` to now-1h and appends `&since=<unix>` on every URL.
    - Test parity guard catches future reorderings of syncSteps().
  </done>
</task>

<task type="auto" tdd="true">
  <name>Task 3: Soak mode + final summary report + README</name>
  <files>cmd/loadtest/soak.go, cmd/loadtest/soak_test.go, cmd/loadtest/report.go, cmd/loadtest/README.md</files>
  <behavior>
    - Test 1 (soak_test.go): `runSoak` with --duration=200ms --concurrency=2 --qps=10 against an httptest.Server returns a Report with >= 1 result and <= 5 results (200ms × 10qps = 2 max with rate-limit jitter).
    - Test 2 (soak_test.go): runSoak honors ctx cancellation — cancelling ctx returns within 100ms even if duration is 10s.
    - Test 3 (soak_test.go): runSoak's QPS cap is enforced by golang.org/x/time/rate.Limiter — observed request rate over a 1s window stays within ±20% of --qps.
    - Test 4 (report_test.go — extension): p99 of {1ms × 99, 100ms × 1} == 100ms; p50 == 1ms; p95 in [1ms, 1ms] for the same input (tight latency distribution sanity check).
  </behavior>
  <action>
    **soak.go** — `func runSoak(ctx, cfg Config, duration time.Duration, concurrency int, qps float64, eps []Endpoint, rep *Report) error`.

    Implementation: golang.org/x/time/rate.Limiter sized at `rate.Every(time.Second/time.Duration(qps))` with burst 1 (matches the project's own client.go pattern; no new dependency since x/time/rate is already a direct dep). Spawn `concurrency` goroutines via errgroup (per GO-CC-4); each worker loops:
    1. `limiter.Wait(ctx)` — gates QPS globally across all workers
    2. pick a random endpoint via `rand.Intn(len(eps))` (math/rand/v2; seeded automatically)
    3. `Hit(ctx, ...)` and append to `rep`
    4. continue until `time.After(duration)` fires or ctx is cancelled

    Use a shared `time.After(duration)` channel + `ctx.Done()` select pattern — first to close ends the group. Errgroup's `Wait` returns first error, but `Hit` swallows network errors into `Result.Err` so the group only errors on ctx cancellation.

    Hard defaults: `--concurrency=4`, `--qps=5`, `--duration=30s`. These are conservative-by-default for shared-cpu-1x replicas. Document in --help and README that bumping --qps above ~50 against a deployed Fly app risks tripping middleware rate-limiting.

    **report.go extension** — extend `Print` with a "Soak Summary" header when invoked from soak mode showing: total requests, successful (2xx), failed, observed rate (req/s = total / wall-clock duration), per-surface breakdown (count, p50/p95/p99 latency, error count). Sorting endpoint results into surface buckets uses `map[Surface][]time.Duration` for the per-surface latency arrays; sort each bucket and index for percentiles. Edge cases: empty bucket → "—" placeholder; bucket of 1 → p50=p95=p99=that-one-latency; bucket of 2-9 → use `sort.Slice` + integer-index arithmetic (`idx := int(math.Ceil(p/100*float64(n))) - 1` clamped to [0, n-1]).

    **soak_test.go** — Test 1 uses `httptest.NewServer` returning 200 with a small JSON body, builds a 2-endpoint registry, runs runSoak with --duration=200ms --qps=10 --concurrency=2, asserts result count is in [1, 5]. Test 2 starts runSoak with --duration=10s in a goroutine, cancels ctx after 50ms, asserts the goroutine returns within 100ms. Test 3 uses a counting handler and a 1-second harness; asserts observed RPS within ±20% of --qps=10. Test 4 lives in a new `report_test.go` and verifies the percentile calculation directly against a fixed []time.Duration input.

    **README.md** — `cmd/loadtest/README.md`. Sections:
    - **WARNING** banner at the top: this binary MUST NOT be invoked from CI; build it with `-tags loadtest`. NEVER point `--base` at `https://www.peeringdb.com` (1 req/hr rate limit; will block your IP).
    - Build instructions: `go build -tags loadtest -o loadtest ./cmd/loadtest`
    - Three modes documented:
      - `loadtest endpoints [--base URL]` — one-shot inventory sweep (~120 requests across 5 surfaces × 13 entity types).
      - `loadtest sync --mode={full|incremental} [--since RFC3339|unix]` — replay a 13-type sync sequence in worker.go ordering. Default `since` for incremental is now-1h.
      - `loadtest soak --duration DUR --concurrency N --qps F` — sustained mixed-surface load. Defaults: 30s, 4 workers, 5 req/s. Document the QPS rationale.
    - Auth: `PDBPLUS_LOADTEST_AUTH_TOKEN` env var → `Authorization: Bearer <token>`.
    - CI exclusion: explain the build tag and that `go build ./...` / `go test ./...` ignore the package by design. `golangci-lint` will also skip it (configurable in .golangci.yml if needed; tag-gated files are already filtered).
    - Sample output snippet from each mode (table format from report.go).
    - Out-of-scope: distributed load, recording/replay, write endpoints — pointers to wrk/k6/vegeta if those needs arise.
  </action>
  <verify>
    <automated>go build -tags loadtest -o /tmp/loadtest ./cmd/loadtest &amp;&amp; go test -tags loadtest -race ./cmd/loadtest/... &amp;&amp; test -f cmd/loadtest/README.md &amp;&amp; test "$(grep -c 'NEVER point' cmd/loadtest/README.md)" -ge 1 &amp;&amp; test "$(grep -c '//go:build loadtest' cmd/loadtest/main.go)" -eq 1</automated>
  </verify>
  <done>
    - `loadtest soak --duration=10s --concurrency=4 --qps=5` runs 4 concurrent workers, caps at 5 req/s globally, ends after 10s, and prints per-surface p50/p95/p99 + success rates.
    - All soak + report percentile tests pass under `-race`.
    - cmd/loadtest/README.md exists, contains the SAFETY warning, documents all three modes + every flag, and explains the build-tag CI exclusion mechanism.
    - `go build ./...` still excludes the package; `golangci-lint run` succeeds (tag-gated files are skipped automatically by golangci-lint's default tag handling).
  </done>
</task>

</tasks>

<verification>
End-to-end smoke (manual, post-merge):
- `go build -tags loadtest -o /tmp/loadtest ./cmd/loadtest`
- `/tmp/loadtest endpoints --base=http://localhost:8080` (against a local `go run ./cmd/peeringdb-plus`)
- `/tmp/loadtest sync --mode=full --base=http://localhost:8080`
- `/tmp/loadtest sync --mode=incremental --since=2026-04-27T00:00:00Z --base=http://localhost:8080`
- `/tmp/loadtest soak --duration=10s --concurrency=2 --qps=5 --base=http://localhost:8080`

CI invariant: `go build ./...`, `go test ./...`, `golangci-lint run`, `govulncheck ./...` MUST all pass without any cmd/loadtest interaction. Confirm by running locally before merging.
</verification>

<success_criteria>
- Single `cmd/loadtest/` Go package, build-tag-isolated (`//go:build loadtest`).
- `go build ./...`, `go test ./...`, and CI (`.github/workflows/ci.yml`) ignore the package — verified by grepping for absence of compilation.
- Three modes operational: endpoints sweep (≥100 distinct invocations), sync (13-step ordered, full + incremental), soak (concurrency × duration with QPS cap).
- All 5 API surfaces exercised in endpoints mode: pdbcompat /api, entrest /rest/v1, GraphQL /graphql, ConnectRPC /peeringdb.v1.*, Web UI /ui.
- Per-surface latency report (p50/p95/p99 + success rate) printed at end of every mode.
- `PDBPLUS_LOADTEST_AUTH_TOKEN` env var honored for authenticated tests.
- Default base `https://peeringdb-plus.fly.dev`; `--help` carries the SAFETY warning naming `https://www.peeringdb.com` as forbidden.
- Read-only: only GET (REST/UI/pdbcompat) and POST (GraphQL/ConnectRPC, both read-only protocols) — no PUT/PATCH/DELETE anywhere in the source. Grep verifies.
- README.md with build instructions, mode docs, flag reference, auth env var, and prominent CI-exclusion warning.
- All tests pass under `go test -tags loadtest -race ./cmd/loadtest/...`.
</success_criteria>

<output>
After completion, create `.planning/quick/260427-vvx-loadtest-script/260427-vvx-SUMMARY.md` summarising:
- The three modes and their flag surface.
- The build-tag isolation mechanism and why it (vs. a doc-only convention) was the right call.
- Endpoint count delivered (the ~130 ballpark, broken down per surface).
- The sync ordering parity guard (test that tracks worker.go's syncSteps).
- Any deviations from this plan (sizing of registry, test counts, etc.).
- Operator quick-start: `go build -tags loadtest ./cmd/loadtest && ./loadtest endpoints`.
</output>
