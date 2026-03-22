// Package dataloader provides DataLoader middleware for batching relationship queries.
// It prevents N+1 query problems when traversing GraphQL relationships by collecting
// individual entity lookups within a batching window and executing them as a single query.
package dataloader

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/dotwaffle/peeringdb-plus/ent"
	"github.com/dotwaffle/peeringdb-plus/ent/campus"
	"github.com/dotwaffle/peeringdb-plus/ent/carrier"
	"github.com/dotwaffle/peeringdb-plus/ent/facility"
	"github.com/dotwaffle/peeringdb-plus/ent/internetexchange"
	"github.com/dotwaffle/peeringdb-plus/ent/ixlan"
	"github.com/dotwaffle/peeringdb-plus/ent/network"
	"github.com/dotwaffle/peeringdb-plus/ent/organization"
	"github.com/vikstrous/dataloadgen"
)

// batchWait is the duration to wait for batching before executing a query.
const batchWait = 2 * time.Millisecond

type contextKey struct{}

// Loaders holds all DataLoader instances for a single request.
type Loaders struct {
	OrganizationByID     *dataloadgen.Loader[int, *ent.Organization]
	NetworkByID          *dataloadgen.Loader[int, *ent.Network]
	FacilityByID         *dataloadgen.Loader[int, *ent.Facility]
	InternetExchangeByID *dataloadgen.Loader[int, *ent.InternetExchange]
	IxLanByID            *dataloadgen.Loader[int, *ent.IxLan]
	CarrierByID          *dataloadgen.Loader[int, *ent.Carrier]
	CampusByID           *dataloadgen.Loader[int, *ent.Campus]
}

// NewLoaders creates a new set of DataLoaders backed by the given ent client.
func NewLoaders(client *ent.Client) *Loaders {
	return &Loaders{
		OrganizationByID: dataloadgen.NewMappedLoader(
			newOrganizationBatcher(client),
			dataloadgen.WithWait(batchWait),
		),
		NetworkByID: dataloadgen.NewMappedLoader(
			newNetworkBatcher(client),
			dataloadgen.WithWait(batchWait),
		),
		FacilityByID: dataloadgen.NewMappedLoader(
			newFacilityBatcher(client),
			dataloadgen.WithWait(batchWait),
		),
		InternetExchangeByID: dataloadgen.NewMappedLoader(
			newInternetExchangeBatcher(client),
			dataloadgen.WithWait(batchWait),
		),
		IxLanByID: dataloadgen.NewMappedLoader(
			newIxLanBatcher(client),
			dataloadgen.WithWait(batchWait),
		),
		CarrierByID: dataloadgen.NewMappedLoader(
			newCarrierBatcher(client),
			dataloadgen.WithWait(batchWait),
		),
		CampusByID: dataloadgen.NewMappedLoader(
			newCampusBatcher(client),
			dataloadgen.WithWait(batchWait),
		),
	}
}

// Middleware creates fresh DataLoaders per request and injects them into the context.
func Middleware(client *ent.Client, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		loaders := NewLoaders(client)
		ctx := context.WithValue(r.Context(), contextKey{}, loaders)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// For retrieves the DataLoaders from the context. Panics if middleware is not configured.
func For(ctx context.Context) *Loaders {
	loaders, ok := ctx.Value(contextKey{}).(*Loaders)
	if !ok {
		panic("dataloader: middleware not configured; use dataloader.Middleware")
	}
	return loaders
}

// Batch functions for each entity type.
// Each fetches entities by ID using IDIn and returns a map keyed by ID.

func newOrganizationBatcher(client *ent.Client) func(ctx context.Context, ids []int) (map[int]*ent.Organization, error) {
	return func(ctx context.Context, ids []int) (map[int]*ent.Organization, error) {
		entities, err := client.Organization.Query().
			Where(organization.IDIn(ids...)).
			All(ctx)
		if err != nil {
			return nil, fmt.Errorf("batch load organizations: %w", err)
		}
		result := make(map[int]*ent.Organization, len(entities))
		for _, e := range entities {
			result[e.ID] = e
		}
		return result, nil
	}
}

func newNetworkBatcher(client *ent.Client) func(ctx context.Context, ids []int) (map[int]*ent.Network, error) {
	return func(ctx context.Context, ids []int) (map[int]*ent.Network, error) {
		entities, err := client.Network.Query().
			Where(network.IDIn(ids...)).
			All(ctx)
		if err != nil {
			return nil, fmt.Errorf("batch load networks: %w", err)
		}
		result := make(map[int]*ent.Network, len(entities))
		for _, e := range entities {
			result[e.ID] = e
		}
		return result, nil
	}
}

func newFacilityBatcher(client *ent.Client) func(ctx context.Context, ids []int) (map[int]*ent.Facility, error) {
	return func(ctx context.Context, ids []int) (map[int]*ent.Facility, error) {
		entities, err := client.Facility.Query().
			Where(facility.IDIn(ids...)).
			All(ctx)
		if err != nil {
			return nil, fmt.Errorf("batch load facilities: %w", err)
		}
		result := make(map[int]*ent.Facility, len(entities))
		for _, e := range entities {
			result[e.ID] = e
		}
		return result, nil
	}
}

func newInternetExchangeBatcher(client *ent.Client) func(ctx context.Context, ids []int) (map[int]*ent.InternetExchange, error) {
	return func(ctx context.Context, ids []int) (map[int]*ent.InternetExchange, error) {
		entities, err := client.InternetExchange.Query().
			Where(internetexchange.IDIn(ids...)).
			All(ctx)
		if err != nil {
			return nil, fmt.Errorf("batch load internet exchanges: %w", err)
		}
		result := make(map[int]*ent.InternetExchange, len(entities))
		for _, e := range entities {
			result[e.ID] = e
		}
		return result, nil
	}
}

func newIxLanBatcher(client *ent.Client) func(ctx context.Context, ids []int) (map[int]*ent.IxLan, error) {
	return func(ctx context.Context, ids []int) (map[int]*ent.IxLan, error) {
		entities, err := client.IxLan.Query().
			Where(ixlan.IDIn(ids...)).
			All(ctx)
		if err != nil {
			return nil, fmt.Errorf("batch load ix lans: %w", err)
		}
		result := make(map[int]*ent.IxLan, len(entities))
		for _, e := range entities {
			result[e.ID] = e
		}
		return result, nil
	}
}

func newCarrierBatcher(client *ent.Client) func(ctx context.Context, ids []int) (map[int]*ent.Carrier, error) {
	return func(ctx context.Context, ids []int) (map[int]*ent.Carrier, error) {
		entities, err := client.Carrier.Query().
			Where(carrier.IDIn(ids...)).
			All(ctx)
		if err != nil {
			return nil, fmt.Errorf("batch load carriers: %w", err)
		}
		result := make(map[int]*ent.Carrier, len(entities))
		for _, e := range entities {
			result[e.ID] = e
		}
		return result, nil
	}
}

func newCampusBatcher(client *ent.Client) func(ctx context.Context, ids []int) (map[int]*ent.Campus, error) {
	return func(ctx context.Context, ids []int) (map[int]*ent.Campus, error) {
		entities, err := client.Campus.Query().
			Where(campus.IDIn(ids...)).
			All(ctx)
		if err != nil {
			return nil, fmt.Errorf("batch load campuses: %w", err)
		}
		result := make(map[int]*ent.Campus, len(entities))
		for _, e := range entities {
			result[e.ID] = e
		}
		return result, nil
	}
}
