---
gsd_plan_version: 1.0
plan_id: 260418-1cn
mode: quick
created: "2026-04-18"
description: "Add sqlite3 binary to Dockerfile.prod + fly deploy + verify (pre-Phase-65 prep)"
---

# Quick Task 260418-1cn: Add sqlite3 to prod image

## Context

Phase 65 (asymmetric Fly fleet migration) requires `sqlite3` on the running machines to perform volume cleanup steps during the rollout. STATE.md marks this as a pre-phase quick task with owner "next session — /gsd-autonomous will pick it up as the first item".

The prod image is Chainguard `glibc-dynamic:latest-dev`, which is Wolfi-based and already uses `apk add --no-cache fuse3` for LiteFS deps. Adding `sqlite` to that same apk line is a one-line change.

## Task 1 — Add sqlite to Dockerfile.prod apk install

**Files:**
- `Dockerfile.prod`

**Action:**
Change line 17 from `RUN apk add --no-cache fuse3` to `RUN apk add --no-cache fuse3 sqlite`. No other changes.

**Verify (local build):**
```bash
docker build -f Dockerfile.prod -t pdbplus-prod-sqlite-test . --target=""
docker run --rm --entrypoint sh pdbplus-prod-sqlite-test -c 'sqlite3 --version && which sqlite3'
```
Skip local docker verify if the sandbox lacks docker — deploy verify covers it.

**Done when:** `Dockerfile.prod` diff shows `sqlite` added to line 17 apk install.

## Task 2 — Commit Dockerfile change

**Files:**
- `Dockerfile.prod`

**Action:**
```
git add Dockerfile.prod
git commit -m "build(prod): add sqlite3 to prod image for Phase 65 fleet ops"
```

**Done when:** commit lands; `git show HEAD --stat` lists `Dockerfile.prod`.

## Task 3 — User deploy + verify (human-in-loop)

**Files:** None — this is a deploy step.

**Action:**
- Pause and ask the user to run `fly deploy` themselves (or approve me running it).
- After deploy, verify sqlite3 is present on at least one machine:
  ```
  fly ssh console -C 'sqlite3 /litefs/peeringdb-plus.db ".tables"'
  ```
- Optional: spot-check another region's machine if convenient.

**Done when:** `sqlite3 .tables` returns the pdbplus ent table list on the LHR primary without error.

## must_haves

- `truths`:
  - `Dockerfile.prod` line 17 `apk add` includes `sqlite`
  - prod image build succeeds (verified by Fly build log on deploy)
  - `sqlite3 /litefs/peeringdb-plus.db ".tables"` runs successfully on LHR primary
- `artifacts`:
  - `Dockerfile.prod` (modified)
- `key_links`:
  - STATE.md "Pre-phase quick task" section captures this as the first item
  - 65-CONTEXT.md assumes sqlite3 is available during fleet migration

## Failure modes

- Wolfi `sqlite` package name: confirmed in Chainguard/Wolfi `apk search sqlite` — ships as `sqlite` (CLI binary) with `sqlite-libs` pulled in as a dep.
- Image size bump: sqlite CLI is ~2 MB — negligible for a Chainguard runtime image.
- If `fly deploy` fails, revert the commit; nothing else to roll back (no LiteFS schema or config change).
