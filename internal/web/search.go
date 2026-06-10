package web

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"entgo.io/ent/dialect/sql"
	"golang.org/x/sync/errgroup"

	"github.com/dotwaffle/peeringdb-plus/ent"
	"github.com/dotwaffle/peeringdb-plus/ent/campus"
	"github.com/dotwaffle/peeringdb-plus/ent/carrier"
	"github.com/dotwaffle/peeringdb-plus/ent/facility"
	"github.com/dotwaffle/peeringdb-plus/ent/internetexchange"
	"github.com/dotwaffle/peeringdb-plus/ent/network"
	"github.com/dotwaffle/peeringdb-plus/ent/organization"
)

// displayLimit is the maximum number of search results shown per entity type
// in the grouped quick-search.
const displayLimit = 10

// pageSize is the number of results per page on the per-type "view all" page,
// for both the initial page and each "Load more" append.
const pageSize = 50

// ErrUnknownSearchType is returned by SearchType when the type slug is not a
// recognized searchable entity type.
var ErrUnknownSearchType = errors.New("unknown search type")

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
	// Total is the exact number of matches across all pages. It equals
	// len(Results) when HasMore is false; when HasMore is true it is the full
	// count (so the "view all" link can show the real total).
	Total int
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
//
// Networks additionally match on the asn column when the query parses as an
// ASN literal (see networkSearchPredicate + parseASNQuery). The fields slice
// here lists only the text columns ORed via ContainsFold.
var searchTypes = []searchTypeConfig{
	{typeName: "Networks", typeSlug: "net", accentColor: "emerald", fields: []string{"name", "aka", "name_long", "irr_as_set"}},
	{typeName: "IXPs", typeSlug: "ix", accentColor: "sky", fields: []string{"name", "aka", "name_long", "city", "country"}},
	{typeName: "Facilities", typeSlug: "fac", accentColor: "violet", fields: []string{"name", "aka", "name_long", "city", "country"}},
	{typeName: "Organizations", typeSlug: "org", accentColor: "amber", fields: []string{"name", "aka", "name_long"}},
	{typeName: "Campuses", typeSlug: "campus", accentColor: "rose", fields: []string{"name"}},
	{typeName: "Carriers", typeSlug: "carrier", accentColor: "cyan", fields: []string{"name", "aka", "name_long"}},
}

// searchTypeBySlug returns the config for a type slug, or false if unknown.
func searchTypeBySlug(slug string) (searchTypeConfig, bool) {
	for _, cfg := range searchTypes {
		if cfg.typeSlug == slug {
			return cfg, true
		}
	}
	return searchTypeConfig{}, false
}

// SearchService provides search across all 6 PeeringDB entity types.
type SearchService struct {
	client *ent.Client
}

// NewSearchService creates a SearchService backed by the given ent client.
func NewSearchService(client *ent.Client) *SearchService {
	return &SearchService{client: client}
}

// searchPage bounds a single page of results.
type searchPage struct {
	offset int
	limit  int
}

// searchOps bundles the per-type query operations so the grouped search and the
// per-type "view all" page share one code path.
type searchOps struct {
	// predicate builds the WHERE clause for the given query, or returns nil
	// when the query is empty.
	predicate func(query string) func(*sql.Selector)
	// list returns one page of hits plus whether more rows exist.
	list func(ctx context.Context, pred func(*sql.Selector), pg searchPage) ([]SearchHit, bool, error)
	// count returns the exact number of matches for the predicate.
	count func(ctx context.Context, pred func(*sql.Selector)) (int, error)
}

// opsFor returns the query operations for a type config.
func (s *SearchService) opsFor(cfg searchTypeConfig) searchOps {
	switch cfg.typeSlug {
	case "net":
		return searchOps{
			predicate: func(q string) func(*sql.Selector) { return networkSearchPredicate(q, cfg.fields) },
			list:      s.listNetworks,
			count:     s.countNetworks,
		}
	case "ix":
		return searchOps{
			predicate: func(q string) func(*sql.Selector) { return buildSearchPredicate(q, cfg.fields) },
			list:      s.listIXPs,
			count:     s.countIXPs,
		}
	case "fac":
		return searchOps{
			predicate: func(q string) func(*sql.Selector) { return buildSearchPredicate(q, cfg.fields) },
			list:      s.listFacilities,
			count:     s.countFacilities,
		}
	case "org":
		return searchOps{
			predicate: func(q string) func(*sql.Selector) { return buildSearchPredicate(q, cfg.fields) },
			list:      s.listOrganizations,
			count:     s.countOrganizations,
		}
	case "campus":
		return searchOps{
			predicate: func(q string) func(*sql.Selector) { return buildSearchPredicate(q, cfg.fields) },
			list:      s.listCampuses,
			count:     s.countCampuses,
		}
	default: // "carrier"
		return searchOps{
			predicate: func(q string) func(*sql.Selector) { return buildSearchPredicate(q, cfg.fields) },
			list:      s.listCarriers,
			count:     s.countCarriers,
		}
	}
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
// populates the corresponding results slot. The exact Total is fetched only
// when the first page overflows (HasMore), so the common small-result case
// pays no extra count query.
func (s *SearchService) typeQueryFunc(ctx context.Context, idx int, cfg searchTypeConfig, query string, results []TypeResult) func() error {
	return func() error {
		ops := s.opsFor(cfg)
		pred := ops.predicate(query)
		if pred == nil {
			return nil
		}

		hits, hasMore, err := ops.list(ctx, pred, searchPage{offset: 0, limit: displayLimit})
		if err != nil {
			return fmt.Errorf("query %s: %w", cfg.typeSlug, err)
		}

		total := len(hits)
		if hasMore {
			total, err = ops.count(ctx, pred)
			if err != nil {
				return fmt.Errorf("count %s: %w", cfg.typeSlug, err)
			}
		}

		results[idx].Results = hits
		results[idx].HasMore = hasMore
		results[idx].Total = total
		return nil
	}
}

// SearchTypeInput parameterizes a single-type paginated search for the
// "view all" results page. The context is passed separately to SearchType.
type SearchTypeInput struct {
	// Query is the raw search term.
	Query string
	// TypeSlug selects the entity type (e.g. "net", "fac").
	TypeSlug string
	// Offset is the number of leading matches to skip (>= 0).
	Offset int
	// Limit is the page size; values < 1 default to pageSize.
	Limit int
}

// SearchTypeResult holds one page of results for a single entity type.
type SearchTypeResult struct {
	// TypeName is the human-readable plural name (e.g. "Networks").
	TypeName string
	// TypeSlug is the short identifier used in URLs (e.g. "net").
	TypeSlug string
	// AccentColor is the Tailwind color name for visual grouping.
	AccentColor string
	// Hits holds this page's matching entities.
	Hits []SearchHit
	// HasMore indicates whether matches exist beyond this page.
	HasMore bool
	// Total is the exact number of matches across all pages.
	Total int
}

// SearchType returns one page of matches for a single entity type, backing the
// "view all" results page. It returns ErrUnknownSearchType when the slug is not
// a recognized searchable type; queries under 2 characters yield an empty page.
func (s *SearchService) SearchType(ctx context.Context, in SearchTypeInput) (SearchTypeResult, error) {
	cfg, ok := searchTypeBySlug(in.TypeSlug)
	if !ok {
		return SearchTypeResult{}, ErrUnknownSearchType
	}
	res := SearchTypeResult{
		TypeName:    cfg.typeName,
		TypeSlug:    cfg.typeSlug,
		AccentColor: cfg.accentColor,
	}

	query := strings.TrimSpace(in.Query)
	if len(query) < 2 {
		return res, nil
	}

	offset := max(in.Offset, 0)
	limit := in.Limit
	if limit < 1 {
		limit = pageSize
	}

	ops := s.opsFor(cfg)
	pred := ops.predicate(query)
	if pred == nil {
		return res, nil
	}

	hits, hasMore, err := ops.list(ctx, pred, searchPage{offset: offset, limit: limit})
	if err != nil {
		return SearchTypeResult{}, fmt.Errorf("search %s: %w", cfg.typeSlug, err)
	}
	total, err := ops.count(ctx, pred)
	if err != nil {
		return SearchTypeResult{}, fmt.Errorf("count %s: %w", cfg.typeSlug, err)
	}

	res.Hits = hits
	res.HasMore = hasMore
	res.Total = total
	return res, nil
}

// trimPage drops the sentinel extra row used to detect more results and reports
// whether the page overflowed.
func trimPage[T any](items []T, limit int) ([]T, bool) {
	if len(items) > limit {
		return items[:limit], true
	}
	return items, false
}

func (s *SearchService) listNetworks(ctx context.Context, pred func(*sql.Selector), pg searchPage) ([]SearchHit, bool, error) {
	items, err := s.client.Network.Query().
		Where(pred).
		Where(network.StatusIn("ok", "pending")).
		Order(network.ByName(), network.ByID()).
		Offset(pg.offset).Limit(pg.limit + 1).
		All(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("fetch networks: %w", err)
	}
	items, hasMore := trimPage(items, pg.limit)
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

func (s *SearchService) countNetworks(ctx context.Context, pred func(*sql.Selector)) (int, error) {
	return s.client.Network.Query().Where(pred).Where(network.StatusIn("ok", "pending")).Count(ctx)
}

func (s *SearchService) listIXPs(ctx context.Context, pred func(*sql.Selector), pg searchPage) ([]SearchHit, bool, error) {
	items, err := s.client.InternetExchange.Query().
		Where(pred).
		Where(internetexchange.StatusIn("ok", "pending")).
		Order(internetexchange.ByName(), internetexchange.ByID()).
		Offset(pg.offset).Limit(pg.limit + 1).
		All(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("fetch ixps: %w", err)
	}
	items, hasMore := trimPage(items, pg.limit)
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

func (s *SearchService) countIXPs(ctx context.Context, pred func(*sql.Selector)) (int, error) {
	return s.client.InternetExchange.Query().Where(pred).Where(internetexchange.StatusIn("ok", "pending")).Count(ctx)
}

func (s *SearchService) listFacilities(ctx context.Context, pred func(*sql.Selector), pg searchPage) ([]SearchHit, bool, error) {
	items, err := s.client.Facility.Query().
		Where(pred).
		Where(facility.StatusIn("ok", "pending")).
		Order(facility.ByName(), facility.ByID()).
		Offset(pg.offset).Limit(pg.limit + 1).
		All(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("fetch facilities: %w", err)
	}
	items, hasMore := trimPage(items, pg.limit)
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

func (s *SearchService) countFacilities(ctx context.Context, pred func(*sql.Selector)) (int, error) {
	return s.client.Facility.Query().Where(pred).Where(facility.StatusIn("ok", "pending")).Count(ctx)
}

func (s *SearchService) listOrganizations(ctx context.Context, pred func(*sql.Selector), pg searchPage) ([]SearchHit, bool, error) {
	items, err := s.client.Organization.Query().
		Where(pred).
		Where(organization.StatusIn("ok", "pending")).
		Order(organization.ByName(), organization.ByID()).
		Offset(pg.offset).Limit(pg.limit + 1).
		All(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("fetch organizations: %w", err)
	}
	items, hasMore := trimPage(items, pg.limit)
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

func (s *SearchService) countOrganizations(ctx context.Context, pred func(*sql.Selector)) (int, error) {
	return s.client.Organization.Query().Where(pred).Where(organization.StatusIn("ok", "pending")).Count(ctx)
}

func (s *SearchService) listCampuses(ctx context.Context, pred func(*sql.Selector), pg searchPage) ([]SearchHit, bool, error) {
	items, err := s.client.Campus.Query().
		Where(pred).
		Where(campus.StatusIn("ok", "pending")).
		Order(campus.ByName(), campus.ByID()).
		Offset(pg.offset).Limit(pg.limit + 1).
		All(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("fetch campuses: %w", err)
	}
	items, hasMore := trimPage(items, pg.limit)
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

func (s *SearchService) countCampuses(ctx context.Context, pred func(*sql.Selector)) (int, error) {
	return s.client.Campus.Query().Where(pred).Where(campus.StatusIn("ok", "pending")).Count(ctx)
}

func (s *SearchService) listCarriers(ctx context.Context, pred func(*sql.Selector), pg searchPage) ([]SearchHit, bool, error) {
	items, err := s.client.Carrier.Query().
		Where(pred).
		Where(carrier.StatusIn("ok", "pending")).
		Order(carrier.ByName(), carrier.ByID()).
		Offset(pg.offset).Limit(pg.limit + 1).
		All(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("fetch carriers: %w", err)
	}
	items, hasMore := trimPage(items, pg.limit)
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

func (s *SearchService) countCarriers(ctx context.Context, pred func(*sql.Selector)) (int, error) {
	return s.client.Carrier.Query().Where(pred).Where(carrier.StatusIn("ok", "pending")).Count(ctx)
}

// networkSearchPredicate builds the network search WHERE clause: a case-
// insensitive contains match across the text fields, ORed with an exact asn
// equality when the query parses as an ASN literal. Returns nil for an empty
// query.
func networkSearchPredicate(query string, textFields []string) func(*sql.Selector) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil
	}
	asnVal, hasASN := parseASNQuery(query)
	return func(sel *sql.Selector) {
		var ors []*sql.Predicate
		for _, f := range textFields {
			ors = append(ors, sql.ContainsFold(f, query))
		}
		if hasASN {
			ors = append(ors, sql.EQ("asn", asnVal))
		}
		if len(ors) > 0 {
			sel.Where(sql.Or(ors...))
		}
	}
}

// parseASNQuery returns (asn, true) if q looks like an ASN literal: optional
// leading "AS"/"as" prefix followed by digits, parsed as a positive 32-bit
// value. Otherwise returns (0, false). Callers use this to OR an exact asn
// equality into an otherwise text-only search predicate.
func parseASNQuery(q string) (int64, bool) {
	s := strings.TrimSpace(q)
	if len(s) >= 2 && (s[0] == 'A' || s[0] == 'a') && (s[1] == 'S' || s[1] == 's') {
		s = s[2:]
	}
	if s == "" {
		return 0, false
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n <= 0 || n > 0xFFFFFFFF {
		return 0, false
	}
	return n, true
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
