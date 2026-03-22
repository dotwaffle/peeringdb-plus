package schema_test

import (
	"context"
	"testing"
	"time"

	"github.com/dotwaffle/peeringdb-plus/ent/organization"
	"github.com/dotwaffle/peeringdb-plus/ent/schema"
	"github.com/dotwaffle/peeringdb-plus/internal/testutil"
)

func TestOrganizationCRUD(t *testing.T) {
	t.Parallel()

	client := testutil.SetupClient(t)
	ctx := context.Background()
	now := time.Now().Truncate(time.Second)

	tests := []struct {
		name       string
		id         int
		orgName    string
		aka        string
		website    string
		city       string
		country    string
		latitude   *float64
		longitude  *float64
		social     []schema.SocialMedia
		wantStatus string
	}{
		{
			name:       "basic organization",
			id:         1,
			orgName:    "Test Organization",
			aka:        "TestOrg",
			website:    "https://example.com",
			city:       "San Francisco",
			country:    "US",
			wantStatus: "ok",
		},
		{
			name:       "organization with coordinates",
			id:         2,
			orgName:    "Geo Org",
			latitude:   ptrFloat64(37.7749),
			longitude:  ptrFloat64(-122.4194),
			wantStatus: "ok",
		},
		{
			name:    "organization with social media",
			id:      3,
			orgName: "Social Org",
			social: []schema.SocialMedia{
				{Service: "twitter", Identifier: "@socialorg"},
				{Service: "github", Identifier: "socialorg"},
			},
			wantStatus: "ok",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create
			create := client.Organization.Create().
				SetID(tt.id).
				SetName(tt.orgName).
				SetCreated(now).
				SetUpdated(now)

			if tt.aka != "" {
				create.SetAka(tt.aka)
			}
			if tt.website != "" {
				create.SetWebsite(tt.website)
			}
			if tt.city != "" {
				create.SetCity(tt.city)
			}
			if tt.country != "" {
				create.SetCountry(tt.country)
			}
			if tt.latitude != nil {
				create.SetLatitude(*tt.latitude)
			}
			if tt.longitude != nil {
				create.SetLongitude(*tt.longitude)
			}
			if tt.social != nil {
				create.SetSocialMedia(tt.social)
			}

			org, err := create.Save(ctx)
			if err != nil {
				t.Fatalf("creating organization: %v", err)
			}

			// Query back
			got, err := client.Organization.Get(ctx, org.ID)
			if err != nil {
				t.Fatalf("querying organization: %v", err)
			}

			// Verify fields
			if got.Name != tt.orgName {
				t.Errorf("name = %q, want %q", got.Name, tt.orgName)
			}
			if got.Status != tt.wantStatus {
				t.Errorf("status = %q, want %q", got.Status, tt.wantStatus)
			}
			if got.ID != tt.id {
				t.Errorf("id = %d, want %d", got.ID, tt.id)
			}
			if tt.latitude != nil && (got.Latitude == nil || *got.Latitude != *tt.latitude) {
				t.Errorf("latitude = %v, want %v", got.Latitude, *tt.latitude)
			}
			if tt.longitude != nil && (got.Longitude == nil || *got.Longitude != *tt.longitude) {
				t.Errorf("longitude = %v, want %v", got.Longitude, *tt.longitude)
			}
			if tt.social != nil {
				if len(got.SocialMedia) != len(tt.social) {
					t.Errorf("social_media length = %d, want %d", len(got.SocialMedia), len(tt.social))
				}
			}
		})
	}

	// Test update
	t.Run("update organization", func(t *testing.T) {
		updated, err := client.Organization.
			UpdateOneID(1).
			SetName("Updated Organization").
			SetUpdated(time.Now()).
			Save(ctx)
		if err != nil {
			t.Fatalf("updating organization: %v", err)
		}
		if updated.Name != "Updated Organization" {
			t.Errorf("updated name = %q, want %q", updated.Name, "Updated Organization")
		}
	})

	// Test query by status
	t.Run("query by status", func(t *testing.T) {
		orgs, err := client.Organization.Query().
			Where(organization.StatusEQ("ok")).
			All(ctx)
		if err != nil {
			t.Fatalf("querying by status: %v", err)
		}
		if len(orgs) != 3 {
			t.Errorf("got %d organizations with status ok, want 3", len(orgs))
		}
	})

	// Test delete
	t.Run("delete organization", func(t *testing.T) {
		err := client.Organization.DeleteOneID(3).Exec(ctx)
		if err != nil {
			t.Fatalf("deleting organization: %v", err)
		}
		count, err := client.Organization.Query().Count(ctx)
		if err != nil {
			t.Fatalf("counting organizations: %v", err)
		}
		if count != 2 {
			t.Errorf("got %d organizations after delete, want 2", count)
		}
	})
}

func ptrFloat64(v float64) *float64 {
	return &v
}
