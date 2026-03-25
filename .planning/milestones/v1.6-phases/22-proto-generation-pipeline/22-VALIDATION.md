---
phase: 22
slug: proto-generation-pipeline
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-03-25
---

# Phase 22 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | Go testing (stdlib) + buf CLI |
| **Config file** | buf.yaml, buf.gen.yaml |
| **Quick run command** | `go generate ./ent/... && buf lint && buf generate && go build ./gen/...` |
| **Full suite command** | `go test ./... -race -count=1` |
| **Estimated runtime** | ~45 seconds |

---

## Sampling Rate

- **After every task commit:** Run `go generate ./ent/... && buf lint && buf generate && go build ./gen/...`
- **After every plan wave:** Run `go test ./... -race -count=1`
- **Before `/gsd:verify-work`:** Full suite must be green
- **Max feedback latency:** 45 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|-----------|-------------------|-------------|--------|
| 22-01-01 | 01 | 1 | PROTO-01 | smoke | `go generate ./ent/...` | N/A (generation) | ⬜ pending |
| 22-01-02 | 01 | 1 | PROTO-02 | smoke | `buf lint` | No — Wave 0 | ⬜ pending |
| 22-01-03 | 01 | 1 | PROTO-03 | smoke | `test -f proto/peeringdb/v1/entpb.proto` | No — Wave 0 | ⬜ pending |
| 22-01-04 | 01 | 1 | PROTO-04 | smoke | `go build ./gen/...` | No — Wave 0 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `buf.yaml` — buf workspace configuration
- [ ] `buf.gen.yaml` — buf code generation configuration
- [ ] `proto/peeringdb/v1/common.proto` — manual SocialMedia message
- [ ] Tool deps: `go get -tool google.golang.org/protobuf/cmd/protoc-gen-go@latest`
- [ ] Tool deps: `go get -tool connectrpc.com/connect/cmd/protoc-gen-connect-go@latest`
- [ ] Runtime dep: `go get connectrpc.com/connect@latest`

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Proto field mappings match ent schema types | PROTO-01 | Semantic correctness, not just compilation | Spot-check 3-4 types: compare ent schema field types against generated proto field types |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 45s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
