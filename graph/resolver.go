// Package graph provides the GraphQL resolver layer for the PeeringDB Plus API.
package graph

import (
	"database/sql"

	"github.com/dotwaffle/peeringdb-plus/ent"
)

// Resolver is the root resolver providing dependencies to all query resolvers.
type Resolver struct {
	client *ent.Client
	db     *sql.DB
}

// NewResolver creates a resolver with the ent client and raw sql.DB for sync_status queries.
func NewResolver(client *ent.Client, db *sql.DB) *Resolver {
	return &Resolver{client: client, db: db}
}
