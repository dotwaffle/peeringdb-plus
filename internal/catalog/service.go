package catalog

import "github.com/dotwaffle/peeringdb-plus/ent"

// Service queries catalog entity details.
type Service struct {
	client *ent.Client
}

// NewService creates a catalog detail service backed by client.
func NewService(client *ent.Client) *Service {
	return &Service{client: client}
}
