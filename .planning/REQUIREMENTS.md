# Requirements: PeeringDB Plus

**Defined:** 2026-03-25
**Core Value:** Fast, reliable access to PeeringDB data from anywhere in the world, served from the nearest edge node with low latency.

## v1.8 Requirements

Requirements for Terminal CLI Interface milestone. Each maps to roadmap phases.

### Detection

- [ ] **DET-01**: Terminal clients (curl, wget, HTTPie, xh, PowerShell, fetch) auto-detected via User-Agent prefix matching
- [ ] **DET-02**: User can force plain text via ?T or ?format=plain query parameter
- [ ] **DET-03**: User can force JSON via ?format=json query parameter
- [ ] **DET-04**: Accept header (text/plain, application/json) serves as secondary format signal
- [ ] **DET-05**: Content negotiation applies to all /ui/ paths — browsers get HTML unchanged

### Rendering

- [ ] **RND-01**: Rich 256-color ANSI output with Unicode box-drawing for terminal clients
- [ ] **RND-02**: Network detail (/ui/asn/{asn}) renders with whois-style key-value header + IX/facility tables
- [ ] **RND-03**: IX detail (/ui/ix/{id}) renders with participant table, facility list, prefix list
- [ ] **RND-04**: Facility detail (/ui/fac/{id}) renders with address, network/IX/carrier lists
- [ ] **RND-05**: Org detail (/ui/org/{id}) renders with child entity lists
- [ ] **RND-06**: Campus detail (/ui/campus/{id}) renders with facility list
- [ ] **RND-07**: Carrier detail (/ui/carrier/{id}) renders with facility list
- [ ] **RND-08**: Search results (/ui/?q=...) render as grouped text list for terminal clients
- [ ] **RND-09**: ASN comparison (/ui/compare/{asn1}/{asn2}) renders shared IXPs/facilities/campuses
- [ ] **RND-10**: Plain text mode (?T) produces identical layout with ASCII box drawing, no ANSI codes
- [ ] **RND-11**: JSON mode (?format=json) outputs the same data structures as JSON
- [ ] **RND-12**: Port speed tiers color-coded (gray/neutral/blue/emerald/amber) matching web UI
- [ ] **RND-13**: Peering policy color-coded (Open=green, Selective=yellow, Restrictive=red)
- [ ] **RND-14**: Route server peers marked with colored [RS] badge in IX presence tables
- [ ] **RND-15**: Aggregate bandwidth displayed in network and IX detail headers
- [ ] **RND-16**: Entity IDs and cross-reference paths shown in output for easy follow-up curls
- [ ] **RND-17**: WHOIS-style output mode (?format=whois) using RPSL-like key-value format
- [ ] **RND-18**: NO_COLOR convention respected — suppress ANSI codes when ?nocolor param present

### Navigation

- [ ] **NAV-01**: Help text at /ui/ for terminal clients listing endpoints, params, and examples
- [ ] **NAV-02**: Text-formatted 404 error for terminal clients (not HTML)
- [ ] **NAV-03**: Text-formatted 500 error for terminal clients (not HTML)
- [ ] **NAV-04**: Root handler (/) returns help text for terminal clients (not redirect)

### Differentiators

- [ ] **DIF-01**: One-line summary mode (?format=short) outputs single-line entity summary
- [ ] **DIF-02**: Data freshness timestamp footer on all terminal responses
- [ ] **DIF-03**: Section filtering (?section=ix,fac) renders only requested sections
- [ ] **DIF-04**: Width parameter (?w=N) adapts table rendering to specified column width

### Shell Integration

- [ ] **SHL-01**: Bash completion script downloadable from server for entity type/ASN completion
- [ ] **SHL-02**: Zsh completion script downloadable from server for entity type/ASN completion
- [ ] **SHL-03**: Shell alias/function setup instructions in help text

## Future Requirements

Deferred to future release.

### Extended Formats

- **FMT-01**: CSV/TSV output mode for spreadsheet import
- **FMT-02**: Custom format strings like wttr.in's ?format=%l:+%c+%t

### Extended Shell

- **SHL-04**: Fish completion script
- **SHL-05**: Downloadable standalone CLI wrapper script (curl-based, not Go binary)

## Out of Scope

| Feature | Reason |
|---------|--------|
| Interactive TUI (Bubble Tea) | Server-side HTTP, not client-side binary. User runs curl, not a Go binary |
| Terminal width auto-detection | Impossible over HTTP. Provide ?w=N parameter instead |
| Client-side pager support | Cannot control pager from HTTP response. Document `\| less -R` |
| TrueColor (24-bit) as default | Not universally supported (screen, multiplexers). 256-color sufficient |
| Custom color themes | Complexity for minimal gain. Ship one well-designed palette |
| Markdown output format | Format proliferation. JSON for machines, ANSI/text for humans |
| WHOIS protocol (port 43) | Separate application. Fly.io doesn't easily expose TCP ports |
| Real-time streaming | curl is request-response. ConnectRPC streaming exists for bulk export |
| Mobile CLI app | Out of scope. curl is the interface |

## Traceability

| Requirement | Phase | Status |
|-------------|-------|--------|
| DET-01 | Phase 28 | Pending |
| DET-02 | Phase 28 | Pending |
| DET-03 | Phase 28 | Pending |
| DET-04 | Phase 28 | Pending |
| DET-05 | Phase 28 | Pending |
| RND-01 | Phase 28 | Pending |
| RND-02 | Phase 29 | Pending |
| RND-03 | Phase 30 | Pending |
| RND-04 | Phase 30 | Pending |
| RND-05 | Phase 30 | Pending |
| RND-06 | Phase 30 | Pending |
| RND-07 | Phase 30 | Pending |
| RND-08 | Phase 30 | Pending |
| RND-09 | Phase 30 | Pending |
| RND-10 | Phase 30 | Pending |
| RND-11 | Phase 30 | Pending |
| RND-12 | Phase 29 | Pending |
| RND-13 | Phase 29 | Pending |
| RND-14 | Phase 29 | Pending |
| RND-15 | Phase 29 | Pending |
| RND-16 | Phase 29 | Pending |
| RND-17 | Phase 30 | Pending |
| RND-18 | Phase 28 | Pending |
| NAV-01 | Phase 28 | Pending |
| NAV-02 | Phase 28 | Pending |
| NAV-03 | Phase 28 | Pending |
| NAV-04 | Phase 28 | Pending |
| DIF-01 | Phase 31 | Pending |
| DIF-02 | Phase 31 | Pending |
| DIF-03 | Phase 31 | Pending |
| DIF-04 | Phase 31 | Pending |
| SHL-01 | Phase 31 | Pending |
| SHL-02 | Phase 31 | Pending |
| SHL-03 | Phase 31 | Pending |

**Coverage:**
- v1.8 requirements: 34 total
- Mapped to phases: 34
- Unmapped: 0

---
*Requirements defined: 2026-03-25*
*Last updated: 2026-03-25 after roadmap creation*
