package templates

import (
	"net/url"
	"strconv"
	"strings"
)

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
	// Total is the exact number of matches across all pages. Equals len(Results)
	// when HasMore is false; drives the "View all N" link when HasMore is true.
	Total int
}

// SearchTypeView holds the data for one page of the per-type "view all"
// results page (and its "Load more" fragment). It mirrors web.SearchTypeResult
// but lives in the templates package to avoid a web -> templates import cycle.
type SearchTypeView struct {
	// TypeName is the human-readable plural name (e.g. "Networks").
	TypeName string
	// TypeSlug is the short identifier used in URLs (e.g. "net").
	TypeSlug string
	// AccentColor is the Tailwind color name for visual grouping.
	AccentColor string
	// Query is the search term, echoed in the header and pagination links.
	Query string
	// Total is the exact number of matches across all pages.
	Total int
	// Hits holds this page's matching entities.
	Hits []SearchResult
	// HasMore indicates whether matches exist beyond this page.
	HasMore bool
	// NextOffset is the offset for the next "Load more" request.
	NextOffset int
}

// searchTypeURL builds the per-type results URL for a query and type slug.
// The offset is omitted when zero (the initial page).
func searchTypeURL(query, slug string, offset int) string {
	v := url.Values{}
	v.Set("q", query)
	v.Set("type", slug)
	if offset > 0 {
		v.Set("offset", strconv.Itoa(offset))
	}
	return "/ui/search?" + v.Encode()
}

// homeQueryURL builds the home-page URL that re-runs a search, used by the
// per-type page's "Back to search" link. Returns "/ui/" for an empty query.
func homeQueryURL(query string) string {
	if query == "" {
		return "/ui/"
	}
	return "/ui/?q=" + url.QueryEscape(query)
}

// FormatThousands renders n with comma thousands separators (e.g. 1234 -> "1,234").
func FormatThousands(n int) string {
	s := strconv.Itoa(n)
	neg := strings.HasPrefix(s, "-")
	if neg {
		s = s[1:]
	}
	var b strings.Builder
	for i := range len(s) {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteByte(s[i])
	}
	if neg {
		return "-" + b.String()
	}
	return b.String()
}

// SearchResult represents a single search hit for template rendering.
type SearchResult struct {
	// Name is the entity's display name.
	Name string
	// Country is the ISO 3166-1 alpha-2 code; empty if not available.
	Country string
	// City is the city name; empty if not available.
	City string
	// ASN is the AS number; 0 if not applicable (non-network entity).
	ASN int
	// DetailURL is the path to the entity's detail page (e.g. "/ui/asn/13335").
	DetailURL string
}
