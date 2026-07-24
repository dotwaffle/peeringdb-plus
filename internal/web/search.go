package web

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/dotwaffle/peeringdb-plus/ent"
	"github.com/dotwaffle/peeringdb-plus/internal/catalog"
)

const (
	displayLimit = 10
	pageSize     = 50
)

var ErrUnknownSearchType = catalog.ErrUnknownSearchType

// SearchHit is a catalog search hit with web presentation metadata.
type SearchHit struct {
	ID        int
	Name      string
	Country   string
	City      string
	ASN       int
	DetailURL string
}

// TypeResult groups web-presented search hits by entity type.
type TypeResult struct {
	TypeName    string
	TypeSlug    string
	AccentColor string
	Results     []SearchHit
	HasMore     bool
	Total       int
}

// SearchTypeInput parameterizes a single-type paginated search.
type SearchTypeInput = catalog.SearchTypeInput

// SearchTypeResult holds one web-presented page of single-type results.
type SearchTypeResult struct {
	TypeName    string
	TypeSlug    string
	AccentColor string
	Hits        []SearchHit
	HasMore     bool
	Total       int
}

// SearchService adapts protocol-neutral catalog searches for web presentation.
type SearchService struct {
	catalog *catalog.SearchService
}

// NewSearchService creates a web search adapter backed by client.
func NewSearchService(client *ent.Client) *SearchService {
	return &SearchService{catalog: catalog.NewSearchService(client)}
}

// Search queries all catalog entity types and adds web presentation metadata.
func (s *SearchService) Search(ctx context.Context, query string) ([]TypeResult, error) {
	results, err := s.catalog.Search(ctx, query)
	if err != nil {
		return nil, err
	}
	out := make([]TypeResult, len(results))
	for i, result := range results {
		out[i] = TypeResult{
			TypeName:    result.TypeName,
			TypeSlug:    result.TypeSlug,
			AccentColor: accentColor(result.TypeSlug),
			Results:     presentSearchHits(result.TypeSlug, result.Results),
			HasMore:     result.HasMore,
			Total:       result.Total,
		}
	}
	return out, nil
}

// SearchType queries one catalog entity type and adds web presentation metadata.
func (s *SearchService) SearchType(ctx context.Context, in SearchTypeInput) (SearchTypeResult, error) {
	result, err := s.catalog.SearchType(ctx, in)
	if err != nil {
		return SearchTypeResult{}, err
	}
	return SearchTypeResult{
		TypeName:    result.TypeName,
		TypeSlug:    result.TypeSlug,
		AccentColor: accentColor(result.TypeSlug),
		Hits:        presentSearchHits(result.TypeSlug, result.Hits),
		HasMore:     result.HasMore,
		Total:       result.Total,
	}, nil
}

func presentSearchHits(typeSlug string, hits []catalog.SearchHit) []SearchHit {
	out := make([]SearchHit, len(hits))
	for i, hit := range hits {
		out[i] = SearchHit{
			ID:        hit.ID,
			Name:      hit.Name,
			Country:   hit.Country,
			City:      hit.City,
			ASN:       hit.ASN,
			DetailURL: searchDetailURL(typeSlug, hit),
		}
	}
	return out
}

func searchDetailURL(typeSlug string, hit catalog.SearchHit) string {
	if typeSlug == "net" {
		return fmt.Sprintf("/ui/asn/%d", hit.ASN)
	}
	return fmt.Sprintf("/ui/%s/%d", typeSlug, hit.ID)
}

func accentColor(typeSlug string) string {
	return map[string]string{
		"net":     "emerald",
		"ix":      "sky",
		"fac":     "violet",
		"org":     "amber",
		"campus":  "rose",
		"carrier": "cyan",
	}[typeSlug]
}

func trimPage[T any](items []T, limit int) ([]T, bool) {
	if len(items) > limit {
		return items[:limit], true
	}
	return items, false
}

func parseASNQuery(q string) (int64, bool) {
	s := strings.TrimSpace(q)
	if len(s) >= 2 && strings.EqualFold(s[:2], "as") {
		s = strings.TrimSpace(s[2:])
	}
	if s == "" {
		return 0, false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, false
		}
	}
	v, err := strconv.ParseUint(s, 10, 32)
	if err != nil || v == 0 {
		return 0, false
	}
	return int64(v), true
}
