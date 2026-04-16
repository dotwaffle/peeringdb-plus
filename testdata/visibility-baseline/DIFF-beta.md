# PeeringDB Visibility Baseline Diff

_Generated: 2026-04-16T23:45:08Z_

_Schema version: 1_

_Targets: beta_

## Table of Contents

- [campus](#campus)
- [carrier](#carrier)
- [carrierfac](#carrierfac)
- [fac](#fac)
- [ix](#ix)
- [ixfac](#ixfac)
- [ixlan](#ixlan)
- [ixpfx](#ixpfx)
- [net](#net)
- [netfac](#netfac)
- [netixlan](#netixlan)
- [org](#org)
- [poc](#poc)

### campus

- Anon rows: 74
- Auth rows: 74
- Auth-only rows: 0

No field-level deltas.

### carrier

- Anon rows: 276
- Auth rows: 276
- Auth-only rows: 0

No field-level deltas.

### carrierfac

- Anon rows: 500
- Auth rows: 500
- Auth-only rows: 0

No field-level deltas.

### fac

- Anon rows: 500
- Auth rows: 500
- Auth-only rows: 0

No field-level deltas.

### ix

- Anon rows: 500
- Auth rows: 500
- Auth-only rows: 0

No field-level deltas.

### ixfac

- Anon rows: 500
- Auth rows: 500
- Auth-only rows: 0

No field-level deltas.

### ixlan

- Anon rows: 500
- Auth rows: 500
- Auth-only rows: 0

| Field | Auth-only | Placeholder | Rows added | PII? | Notes |
|-------|-----------|-------------|------------|------|-------|
| `ixf_ixp_member_list_url` | yes | `<auth-only:string>` | 18 | no |  |

### ixpfx

- Anon rows: 500
- Auth rows: 500
- Auth-only rows: 0

No field-level deltas.

### net

- Anon rows: 500
- Auth rows: 500
- Auth-only rows: 0

No field-level deltas.

### netfac

- Anon rows: 500
- Auth rows: 500
- Auth-only rows: 0

No field-level deltas.

### netixlan

- Anon rows: 500
- Auth rows: 500
- Auth-only rows: 0

No field-level deltas.

### org

- Anon rows: 500
- Auth rows: 500
- Auth-only rows: 0

No field-level deltas.

### poc

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

