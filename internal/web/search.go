package web

import (
	"context"
	"fmt"
	"strings"

	"entgo.io/ent/dialect/sql"
	"golang.org/x/sync/errgroup"

	"github.com/dotwaffle/peeringdb-plus/ent"
)

// displayLimit is the maximum number of search results shown per entity type.
const displayLimit = 10

// SearchHit represents a single search result with display-ready fields.
type SearchHit struct {
	// ID is the entity's database identifier.
	ID int
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

// TypeResult groups search hits for a single entity type with metadata for display.
type TypeResult struct {
	// TypeName is the human-readable plural name (e.g. "Networks", "IXPs").
	TypeName string
	// TypeSlug is the short identifier used in URLs (e.g. "net", "ix").
	TypeSlug string
	// AccentColor is the Tailwind color name for visual grouping (e.g. "emerald", "sky").
	AccentColor string
	// Results holds up to displayLimit matching entities.
	Results []SearchHit
	// HasMore indicates whether additional matches exist beyond the displayed results.
	HasMore bool
}

// searchTypeConfig defines the metadata and query fields for a searchable entity type.
type searchTypeConfig struct {
	typeName    string
	typeSlug    string
	accentColor string
	fields      []string
}

// searchTypes defines the 6 searchable PeeringDB entity types in display order.
// Order: Networks, IXPs, Facilities, Organizations, Campuses, Carriers.
var searchTypes = []searchTypeConfig{
	{typeName: "Networks", typeSlug: "net", accentColor: "emerald", fields: []string{"name", "aka", "name_long", "irr_as_set"}},
	{typeName: "IXPs", typeSlug: "ix", accentColor: "sky", fields: []string{"name", "aka", "name_long", "city", "country"}},
	{typeName: "Facilities", typeSlug: "fac", accentColor: "violet", fields: []string{"name", "aka", "name_long", "city", "country"}},
	{typeName: "Organizations", typeSlug: "org", accentColor: "amber", fields: []string{"name", "aka", "name_long"}},
	{typeName: "Campuses", typeSlug: "campus", accentColor: "rose", fields: []string{"name"}},
	{typeName: "Carriers", typeSlug: "carrier", accentColor: "cyan", fields: []string{"name", "aka", "name_long"}},
}

// SearchService provides search across all 6 PeeringDB entity types.
type SearchService struct {
	client *ent.Client
}

// NewSearchService creates a SearchService backed by the given ent client.
func NewSearchService(client *ent.Client) *SearchService {
	return &SearchService{client: client}
}

// Search queries all 6 entity types in parallel for the given search term.
// Returns only types with matches, sorted in canonical type order.
// Queries under 2 characters (after trimming) return an empty slice.
func (s *SearchService) Search(ctx context.Context, query string) ([]TypeResult, error) {
	query = strings.TrimSpace(query)
	if len(query) < 2 {
		return nil, nil
	}

	// Pre-allocate results with metadata. Each goroutine writes to its own
	// index so no mutex is needed (distinct indices are race-free).
	results := make([]TypeResult, len(searchTypes))
	for i, cfg := range searchTypes {
		results[i] = TypeResult{
			TypeName:    cfg.typeName,
			TypeSlug:    cfg.typeSlug,
			AccentColor: cfg.accentColor,
		}
	}

	g, gctx := errgroup.WithContext(ctx)

	for i, cfg := range searchTypes {
		g.Go(s.typeQueryFunc(gctx, i, cfg, query, results))
	}

	if err := g.Wait(); err != nil {
		return nil, fmt.Errorf("search %q: %w", query, err)
	}

	// Filter out types with zero matches.
	var filtered []TypeResult
	for _, r := range results {
		if len(r.Results) > 0 {
			filtered = append(filtered, r)
		}
	}

	return filtered, nil
}

// typeQueryFunc returns a function that queries a single entity type and
// populates the corresponding results slot.
func (s *SearchService) typeQueryFunc(ctx context.Context, idx int, cfg searchTypeConfig, query string, results []TypeResult) func() error {
	return func() error {
		pred := buildSearchPredicate(query, cfg.fields)
		if pred == nil {
			return nil
		}

		var hits []SearchHit
		var hasMore bool
		var err error

		switch cfg.typeSlug {
		case "net":
			hits, hasMore, err = s.queryNetworks(ctx, pred)
		case "ix":
			hits, hasMore, err = s.queryIXPs(ctx, pred)
		case "fac":
			hits, hasMore, err = s.queryFacilities(ctx, pred)
		case "org":
			hits, hasMore, err = s.queryOrganizations(ctx, pred)
		case "campus":
			hits, hasMore, err = s.queryCampuses(ctx, pred)
		case "carrier":
			hits, hasMore, err = s.queryCarriers(ctx, pred)
		}

		if err != nil {
			return fmt.Errorf("query %s: %w", cfg.typeSlug, err)
		}

		results[idx].Results = hits
		results[idx].HasMore = hasMore
		return nil
	}
}

func (s *SearchService) queryNetworks(ctx context.Context, pred func(*sql.Selector)) ([]SearchHit, bool, error) {
	items, err := s.client.Network.Query().Where(pred).Limit(displayLimit + 1).All(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("fetch networks: %w", err)
	}
	hasMore := len(items) > displayLimit
	if hasMore {
		items = items[:displayLimit]
	}
	hits := make([]SearchHit, len(items))
	for i, n := range items {
		hits[i] = SearchHit{
			ID:        n.ID,
			Name:      n.Name,
			ASN:       n.Asn,
			DetailURL: fmt.Sprintf("/ui/asn/%d", n.Asn),
		}
	}
	return hits, hasMore, nil
}

func (s *SearchService) queryIXPs(ctx context.Context, pred func(*sql.Selector)) ([]SearchHit, bool, error) {
	items, err := s.client.InternetExchange.Query().Where(pred).Limit(displayLimit + 1).All(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("fetch ixps: %w", err)
	}
	hasMore := len(items) > displayLimit
	if hasMore {
		items = items[:displayLimit]
	}
	hits := make([]SearchHit, len(items))
	for i, ix := range items {
		hits[i] = SearchHit{
			ID:        ix.ID,
			Name:      ix.Name,
			Country:   ix.Country,
			City:      ix.City,
			DetailURL: fmt.Sprintf("/ui/ix/%d", ix.ID),
		}
	}
	return hits, hasMore, nil
}

func (s *SearchService) queryFacilities(ctx context.Context, pred func(*sql.Selector)) ([]SearchHit, bool, error) {
	items, err := s.client.Facility.Query().Where(pred).Limit(displayLimit + 1).All(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("fetch facilities: %w", err)
	}
	hasMore := len(items) > displayLimit
	if hasMore {
		items = items[:displayLimit]
	}
	hits := make([]SearchHit, len(items))
	for i, fac := range items {
		hits[i] = SearchHit{
			ID:        fac.ID,
			Name:      fac.Name,
			Country:   fac.Country,
			City:      fac.City,
			DetailURL: fmt.Sprintf("/ui/fac/%d", fac.ID),
		}
	}
	return hits, hasMore, nil
}

func (s *SearchService) queryOrganizations(ctx context.Context, pred func(*sql.Selector)) ([]SearchHit, bool, error) {
	items, err := s.client.Organization.Query().Where(pred).Limit(displayLimit + 1).All(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("fetch organizations: %w", err)
	}
	hasMore := len(items) > displayLimit
	if hasMore {
		items = items[:displayLimit]
	}
	hits := make([]SearchHit, len(items))
	for i, org := range items {
		hits[i] = SearchHit{
			ID:        org.ID,
			Name:      org.Name,
			Country:   org.Country,
			City:      org.City,
			DetailURL: fmt.Sprintf("/ui/org/%d", org.ID),
		}
	}
	return hits, hasMore, nil
}

func (s *SearchService) queryCampuses(ctx context.Context, pred func(*sql.Selector)) ([]SearchHit, bool, error) {
	items, err := s.client.Campus.Query().Where(pred).Limit(displayLimit + 1).All(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("fetch campuses: %w", err)
	}
	hasMore := len(items) > displayLimit
	if hasMore {
		items = items[:displayLimit]
	}
	hits := make([]SearchHit, len(items))
	for i, c := range items {
		hits[i] = SearchHit{
			ID:        c.ID,
			Name:      c.Name,
			Country:   c.Country,
			City:      c.City,
			DetailURL: fmt.Sprintf("/ui/campus/%d", c.ID),
		}
	}
	return hits, hasMore, nil
}

func (s *SearchService) queryCarriers(ctx context.Context, pred func(*sql.Selector)) ([]SearchHit, bool, error) {
	items, err := s.client.Carrier.Query().Where(pred).Limit(displayLimit + 1).All(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("fetch carriers: %w", err)
	}
	hasMore := len(items) > displayLimit
	if hasMore {
		items = items[:displayLimit]
	}
	hits := make([]SearchHit, len(items))
	for i, cr := range items {
		hits[i] = SearchHit{
			ID:        cr.ID,
			Name:      cr.Name,
			DetailURL: fmt.Sprintf("/ui/carrier/%d", cr.ID),
		}
	}
	return hits, hasMore, nil
}

// buildSearchPredicate creates a sql.Selector predicate that ORs together
// case-insensitive contains matches across the given fields.
// Returns nil if search is empty.
//
// This is a local duplicate of the pdbcompat version to avoid cross-package
// coupling. The search service owns its own predicate construction.
func buildSearchPredicate(search string, searchFields []string) func(*sql.Selector) {
	search = strings.TrimSpace(search)
	if search == "" {
		return nil
	}
	return func(s *sql.Selector) {
		var ors []*sql.Predicate
		for _, f := range searchFields {
			ors = append(ors, sql.ContainsFold(f, search))
		}
		if len(ors) > 0 {
			s.Where(sql.Or(ors...))
		}
	}
}

