package web

import (
	"github.com/dotwaffle/peeringdb-plus/ent"
	"github.com/dotwaffle/peeringdb-plus/internal/catalog"
)

// CompareService compares two networks through the catalog service.
type CompareService = catalog.CompareService

// CompareInput parameterizes a network comparison.
type CompareInput = catalog.CompareInput

// NewCompareService creates a catalog-backed comparison service.
func NewCompareService(client *ent.Client) *CompareService {
	return catalog.NewCompareService(client)
}
