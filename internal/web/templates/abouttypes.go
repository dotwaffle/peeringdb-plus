package templates

import "time"

// DataFreshness holds sync status information for the About page display.
type DataFreshness struct {
	Available  bool
	LastSyncAt time.Time
	Age        time.Duration
}

// PrivacySync carries the Phase 61 OBS-02 Privacy & Sync section payload
// for both the HTML (about.templ) and terminal (termrender/about.go)
// About page renderings. Populated by web.Handler.handleAbout from the
// handler's captured startup values; the renderers do not read env vars
// or config at request time.
//
// OverrideActive is true iff PDBPLUS_PUBLIC_TIER=users was in effect at
// process start — the "never silent escalation" flag (D-06).
type PrivacySync struct {
	// AuthMode is the human-readable label: "Authenticated with PeeringDB
	// API key" or "Anonymous (no key)".
	AuthMode string

	// PublicTier is the lowercase env-var value: "public" or "users".
	// Matches PDBPLUS_PUBLIC_TIER so operators can map the UI back to the
	// deploy config without translating.
	PublicTier string

	// PublicTierExplanation is the one-line plain-language gloss of what
	// PublicTier means for an anonymous caller's data view. Rendered below
	// the PublicTier value on both surfaces.
	PublicTierExplanation string

	// OverrideActive is true when PublicTier == "users". When true, the
	// HTML renders an amber "Override active" badge and the terminal
	// prepends a "! " indicator to the Public Tier line.
	OverrideActive bool
}

// AboutPageData bundles the two payloads consumed by the terminal About
// renderer. The dispatch table (internal/web/termrender/dispatch.go)
// registers a single concrete type per page; this struct is that type
// for /ui/about post-Phase-61.
type AboutPageData struct {
	Freshness DataFreshness
	Privacy   PrivacySync
}
