---
phase: 48-response-hardening-internal-quality
plan: 01
subsystem: middleware
tags: [csp, gzip, compression, security-headers, klauspost]

requires:
  - phase: 47-server-request-hardening
    provides: server-level request hardening and input validation
provides:
  - CSP Report-Only headers on web UI and GraphQL routes
  - Gzip compression middleware excluding gRPC content types
  - Updated middleware chain with CSP and Compression
affects: [48-02, deployment, web-ui]

tech-stack:
  added: [klauspost/compress/gzhttp (promoted from indirect)]
  patterns: [per-route header middleware, content-type based compression exclusion]

key-files:
  created:
    - internal/middleware/csp.go
    - internal/middleware/csp_test.go
    - internal/middleware/compression.go
    - internal/middleware/compression_test.go
  modified:
    - cmd/peeringdb-plus/main.go
    - go.mod
    - go.sum

key-decisions:
  - "ExceptContentTypes needs explicit application/grpc+proto entry -- gzhttp MIME matching treats it as distinct from application/grpc"

patterns-established:
  - "Per-route header middleware: path-based switch in middleware to apply different headers to different route prefixes"

requirements-completed: [SEC-03, PERF-01]

duration: 3min
completed: 2026-04-02
---

# Phase 48 Plan 01: CSP & Compression Middleware Summary

**Content-Security-Policy-Report-Only headers on web routes and gzip compression via klauspost/gzhttp with gRPC content-type exclusions**

## Performance

- **Duration:** 3 min
- **Started:** 2026-04-02T04:36:54Z
- **Completed:** 2026-04-02T04:40:29Z
- **Tasks:** 2
- **Files modified:** 7

## Accomplishments
- CSP Report-Only middleware with per-route policies: tighter on /ui/*, permissive (unsafe-eval) on /graphql, none on /api/ and /rest/
- Gzip compression middleware using klauspost/gzhttp excluding gRPC content types (application/grpc, application/grpc+proto, application/connect+proto)
- Middleware chain updated to: Recovery -> CORS -> OTel HTTP -> Logging -> Readiness -> CSP -> Caching -> Gzip -> mux

## Task Commits

Each task was committed atomically:

1. **Task 1: CSP middleware with per-route policies** - `ee07a3d` (feat)
2. **Task 2: Gzip compression middleware and wire both into main.go** - `72be127` (feat)

## Files Created/Modified
- `internal/middleware/csp.go` - CSP middleware with path-based policy selection
- `internal/middleware/csp_test.go` - 10 table-driven tests for CSP route matching and directive verification
- `internal/middleware/compression.go` - Gzip compression middleware via klauspost/gzhttp
- `internal/middleware/compression_test.go` - 6 table-driven tests for compression with content-type exclusions
- `cmd/peeringdb-plus/main.go` - Updated middleware chain with CSP and Compression
- `go.mod` - Promoted klauspost/compress to direct dependency
- `go.sum` - Updated checksums

## Decisions Made
- gzhttp ExceptContentTypes requires explicit `application/grpc+proto` entry -- MIME type matching in gzhttp treats `application/grpc+proto` as a distinct type from `application/grpc`, not as a subtype

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Added application/grpc+proto to compression exclusion list**
- **Found during:** Task 2 (Compression middleware)
- **Issue:** Plan listed only application/grpc and application/connect+proto, but gzhttp MIME matching treats application/grpc+proto as a distinct content type not covered by the application/grpc exclusion
- **Fix:** Added explicit application/grpc+proto entry to ExceptContentTypes list
- **Files modified:** internal/middleware/compression.go
- **Verification:** TestCompression/no_gzip_for_application/grpc+proto_content_type passes
- **Committed in:** 72be127 (Task 2 commit)

---

**Total deviations:** 1 auto-fixed (1 bug)
**Impact on plan:** Essential fix -- without it, gRPC+proto responses would be double-compressed, breaking ConnectRPC streaming.

## Issues Encountered
None

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- CSP and Compression middlewares are wired and tested, ready for deployment
- Phase 48-02 (GraphQL sentinel errors, metrics caching) can proceed independently

---
*Phase: 48-response-hardening-internal-quality*
*Completed: 2026-04-02*
