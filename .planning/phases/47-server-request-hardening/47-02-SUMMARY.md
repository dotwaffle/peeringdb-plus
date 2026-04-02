---
phase: 47-server-request-hardening
plan: 02
subsystem: web
tags: [input-validation, security, asn, terminal]

requires:
  - phase: 28-terminal-rendering
    provides: terminal rendering with width parameter
  - phase: 15-detail-pages
    provides: handleNetworkDetail with ASN routing
provides:
  - ASN range validation (1-4294967295) returning 400 Bad Request
  - Width parameter capping at 500 columns
  - parseASN helper function for web handlers
affects: [web-ui, terminal-rendering]

tech-stack:
  added: []
  patterns: [parseASN helper for ASN validation, silent width capping]

key-files:
  created: []
  modified:
    - internal/web/handler.go
    - internal/web/detail.go
    - internal/web/render.go
    - internal/web/handler_test.go
    - internal/web/detail_test.go

key-decisions:
  - "parseASN returns false for non-numeric (400 not 404) -- invalid input is client error"
  - "Width capping is silent (no error) -- graceful degradation for terminal users"

patterns-established:
  - "parseASN(s string) (int, bool) validates ASN range and returns false for invalid"
  - "maxTerminalWidth constant caps all width parsing in renderPage"

requirements-completed: [SEC-01, SEC-02]

duration: 4min
completed: 2026-04-02
---

# Phase 47 Plan 02: ASN Validation & Width Capping Summary

**ASN range validation returning 400 Bad Request for out-of-range values, and silent width capping at 500 columns**

## Performance

- **Duration:** 4 min
- **Started:** 2026-04-02T04:19:35Z
- **Completed:** 2026-04-02T04:24:20Z
- **Tasks:** 2
- **Files modified:** 5

## Accomplishments
- ASN values outside 1-4294967295 return 400 Bad Request with RFC 9457 problem detail in handleNetworkDetail and handleCompare
- Width parameter (?w=) silently capped to 500 in all 3 renderPage branches
- 15 new test cases covering ASN overflow, zero, negative, non-numeric, and width capping

## Task Commits

Each task was committed atomically:

1. **Task 1: ASN range validation in web handlers** - `0ade7ff` (test + feat, TDD)
2. **Task 2: Width parameter capping at 500 columns** - `96a6f1a` (test + feat, TDD)

## Files Created/Modified
- `internal/web/handler.go` - Added parseASN helper, maxASN constant, updated handleCompare with WriteProblem
- `internal/web/detail.go` - Updated handleNetworkDetail with parseASN and WriteProblem
- `internal/web/render.go` - Added maxTerminalWidth constant, capped width in 3 locations
- `internal/web/handler_test.go` - Added TestASNValidation (8 cases), TestWidthParameterCapping (5 cases), TestMaxTerminalWidthConstant
- `internal/web/detail_test.go` - Updated TestDetailPages_NotFound to expect 400 for invalid ASN

## Decisions Made
- parseASN returns false for non-numeric strings, making non-numeric ASNs return 400 (Bad Request) instead of 404 (Not Found). Invalid input is a client error, not a missing resource.
- Width capping is silent per CONTEXT.md -- no error for ?w=99999, just caps to 500.
- 3 width-parsing locations (not 4 as plan estimated) -- ModeJSON has no width parsing.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Updated pre-existing tests for new ASN validation behavior**
- **Found during:** Task 1 (ASN validation)
- **Issue:** TestDetailPages_NotFound and TestCompareResultsPage_NonNumericASN expected 404 for non-numeric ASNs
- **Fix:** Updated TestDetailPages_NotFound to use per-case wantStatus (400 for invalid ASN, 404 for others). Updated TestCompareResultsPage_NonNumericASN to expect 400.
- **Files modified:** internal/web/detail_test.go, internal/web/handler_test.go
- **Verification:** All 216 tests pass
- **Committed in:** 0ade7ff (Task 1 commit)

---

**Total deviations:** 1 auto-fixed (1 bug)
**Impact on plan:** Necessary update to pre-existing tests that assumed non-numeric ASNs return 404.

## Issues Encountered
None

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- Input validation complete for ASN and width parameters
- All web handler tests pass (216 tests)
- Ready for remaining hardening work (server timeouts, body limits, etc.)

---
## Self-Check: PASSED

All files exist. All commits verified.

---
*Phase: 47-server-request-hardening*
*Completed: 2026-04-02*
