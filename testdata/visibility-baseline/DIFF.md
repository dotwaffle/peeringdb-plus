# PeeringDB Visibility Baseline Diff

_Generated: 2026-04-16T23:45:08Z_

_Schema version: 1_

_Targets: beta_

## Table of Contents

- [beta/campus](#beta/campus)
- [beta/carrier](#beta/carrier)
- [beta/carrierfac](#beta/carrierfac)
- [beta/fac](#beta/fac)
- [beta/ix](#beta/ix)
- [beta/ixfac](#beta/ixfac)
- [beta/ixlan](#beta/ixlan)
- [beta/ixpfx](#beta/ixpfx)
- [beta/net](#beta/net)
- [beta/netfac](#beta/netfac)
- [beta/netixlan](#beta/netixlan)
- [beta/org](#beta/org)
- [beta/poc](#beta/poc)

### beta/campus

- Anon rows: 74
- Auth rows: 74
- Auth-only rows: 0

No field-level deltas.

### beta/carrier

- Anon rows: 276
- Auth rows: 276
- Auth-only rows: 0

No field-level deltas.

### beta/carrierfac

- Anon rows: 500
- Auth rows: 500
- Auth-only rows: 0

No field-level deltas.

### beta/fac

- Anon rows: 500
- Auth rows: 500
- Auth-only rows: 0

No field-level deltas.

### beta/ix

- Anon rows: 500
- Auth rows: 500
- Auth-only rows: 0

No field-level deltas.

### beta/ixfac

- Anon rows: 500
- Auth rows: 500
- Auth-only rows: 0

No field-level deltas.

### beta/ixlan

- Anon rows: 500
- Auth rows: 500
- Auth-only rows: 0

| Field | Auth-only | Placeholder | Rows added | PII? | Notes |
|-------|-----------|-------------|------------|------|-------|
| `ixf_ixp_member_list_url` | yes | `<auth-only:string>` | 18 | no |  |

### beta/ixpfx

- Anon rows: 500
- Auth rows: 500
- Auth-only rows: 0

No field-level deltas.

### beta/net

- Anon rows: 500
- Auth rows: 500
- Auth-only rows: 0

No field-level deltas.

### beta/netfac

- Anon rows: 500
- Auth rows: 500
- Auth-only rows: 0

No field-level deltas.

### beta/netixlan

- Anon rows: 500
- Auth rows: 500
- Auth-only rows: 0

No field-level deltas.

### beta/org

- Anon rows: 500
- Auth rows: 500
- Auth-only rows: 0

No field-level deltas.

### beta/poc

- Anon rows: 40
- Auth rows: 500
- Auth-only rows: 460
- `visible` values (anon): `Public`
- `visible` values (auth): `Public`, `Users`

| Field | Auth-only | Placeholder | Rows added | PII? | Notes |
|-------|-----------|-------------|------------|------|-------|
| `created` | yes | `<auth-only:string>` | 460 | no |  |
| `email` | yes | `<auth-only:string>` | 460 | yes |  |
| `id` | yes | `<auth-only:string>` | 460 | no |  |
| `name` | yes | `<auth-only:string>` | 460 | yes |  |
| `net_id` | yes | `<auth-only:string>` | 460 | no |  |
| `phone` | yes | `<auth-only:string>` | 460 | yes |  |
| `role` | yes | `<auth-only:string>` | 460 | no |  |
| `status` | yes | `<auth-only:string>` | 460 | no |  |
| `updated` | yes | `<auth-only:string>` | 460 | no |  |
| `url` | yes | `<auth-only:string>` | 460 | no |  |
| `visible` | no | `` | 0 | no | value set differs across modes |

