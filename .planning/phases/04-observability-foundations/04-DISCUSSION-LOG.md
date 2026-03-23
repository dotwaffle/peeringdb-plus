# Phase 4: Observability Foundations - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-03-22
**Phase:** 04-observability-foundations
**Areas discussed:** HTTP span design, Metric expansion, MeterProvider fix

---

## HTTP Span Design

### HTTP tracing approach

| Option | Description | Selected |
|--------|-------------|----------|
| otelhttp transport | One-line wrap, automatic per-request spans with HTTP semantic conventions | |
| Manual spans | More control but re-implements otelhttp | |
| Both layers | otelhttp for raw HTTP + manual parent spans in FetchAll with type/page attributes | ✓ |

**User's choice:** Both layers
**Notes:** Gives both automatic HTTP semantics AND business-level span hierarchy

### Span naming convention

| Option | Description | Selected |
|--------|-------------|----------|
| peeringdb.fetch/{type} | Type in span name, follows OTel RPC naming | ✓ |
| peeringdb.fetch + attr | Generic name with type attribute | |
| You decide | Claude picks | |

**User's choice:** peeringdb.fetch/{type}

### Retry visibility

| Option | Description | Selected |
|--------|-------------|----------|
| Explicit retry spans | Each attempt gets own span with attempt number attribute | ✓ |
| Just otelhttp | Let otelhttp handle, retries show as sibling HTTP spans | |
| You decide | Claude picks | |

**User's choice:** Explicit retry spans

### Page-level attributes

| Option | Description | Selected |
|--------|-------------|----------|
| As span events | Add event per page with number, count, running total | ✓ |
| As final attrs | Set totals once FetchAll completes | |
| You decide | Claude picks | |

**User's choice:** As span events

### Rate limiter visibility

| Option | Description | Selected |
|--------|-------------|----------|
| As span event | Record rate_limiter.wait event with duration | ✓ |
| Skip it | Internal plumbing, not worth trace noise | |
| You decide | Claude picks | |

**User's choice:** As span event

---

## Metric Expansion

### Per-type metrics

| Option | Description | Selected |
|--------|-------------|----------|
| Duration + counts | 3 new instruments with type attribute | |
| Duration + counts + errors | 4 new instruments, diagnose which types fail | ✓ |
| You decide | Claude picks | |

**User's choice:** Duration + counts + errors

### Sync freshness gauge

| Option | Description | Selected |
|--------|-------------|----------|
| Callback gauge | Observable gauge with callback, compute on scrape | ✓ |
| Push on sync | Record timestamp after sync completes | |
| You decide | Claude picks | |

**User's choice:** Callback gauge

### Metric naming convention

| Option | Description | Selected |
|--------|-------------|----------|
| Flat + attribute | pdbplus.sync.type.* with type=net\|ix\|fac attribute | ✓ |
| Hierarchical | pdbplus.sync.net.duration etc., separate per type | |
| You decide | Claude picks | |

**User's choice:** Flat + attribute

### Error counter split

| Option | Description | Selected |
|--------|-------------|----------|
| Separate counters | pdbplus.sync.type.fetch_errors and upsert_errors | ✓ |
| Single with attr | pdbplus.sync.type.errors with error_source attribute | |
| You decide | Claude picks | |

**User's choice:** Separate counters

---

## MeterProvider Fix

### Tech debt scope

| Option | Description | Selected |
|--------|-------------|----------|
| Clean up all | Remove unused DataLoader, config.IsPrimary, globalid exports | |
| OTel-related only | Only fix metrics and tracing gaps | ✓ |
| You decide | Claude assesses | |

**User's choice:** OTel-related only
**Notes:** MeterProvider already initialized in provider.go:62-70 — confirmed working. Only need to wire .Record()/.Add() calls.

---

## Claude's Discretion

None — all decisions made by user.

## Deferred Ideas

- DataLoader middleware cleanup
- config.IsPrimary field removal
- globalid.go export cleanup
