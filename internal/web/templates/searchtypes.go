package templates

// SearchGroup represents a group of search results for one entity type.
// These types mirror web.TypeResult and web.SearchHit, defined in the
// templates package to avoid circular imports (web imports templates,
// so templates cannot import web).
type SearchGroup struct {
	// TypeName is the human-readable plural name (e.g. "Networks", "IXPs").
	TypeName string
	// TypeSlug is the short identifier used in URLs (e.g. "net", "ix").
	TypeSlug string
	// AccentColor is the Tailwind color name for visual grouping (e.g. "emerald", "sky").
	AccentColor string
	// Results holds up to 10 matching entities.
	Results []SearchResult
	// HasMore indicates whether additional matches exist beyond the displayed results.
	HasMore bool
}

// hasMoreSuffix returns "+" when hasMore is true, empty string otherwise.
// Used in count badge display to indicate additional results exist.
func hasMoreSuffix(hasMore bool) string {
	if hasMore {
		return "+"
	}
	return ""
}

// SearchResult represents a single search hit for template rendering.
type SearchResult struct {
	// Name is the entity's display name.
	Name string
	// Subtitle provides context: ASN for networks, city/country for facilities/IXPs/campuses.
	Subtitle string
	// DetailURL is the path to the entity's detail page (e.g. "/ui/asn/13335").
	DetailURL string
}
