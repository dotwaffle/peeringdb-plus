package web

import (
	"context"
	"fmt"
	"sort"

	"golang.org/x/sync/errgroup"

	"github.com/dotwaffle/peeringdb-plus/ent"
	"github.com/dotwaffle/peeringdb-plus/ent/facility"
	"github.com/dotwaffle/peeringdb-plus/ent/network"
	"github.com/dotwaffle/peeringdb-plus/ent/networkfacility"
	"github.com/dotwaffle/peeringdb-plus/ent/networkixlan"
	"github.com/dotwaffle/peeringdb-plus/internal/web/templates"
)

// CompareService compares two networks' presences across IXPs, facilities, and campuses.
type CompareService struct {
	client *ent.Client
}

// NewCompareService creates a CompareService backed by the given ent client.
func NewCompareService(client *ent.Client) *CompareService {
	return &CompareService{client: client}
}

// CompareInput holds the parameters for a network comparison.
type CompareInput struct {
	// ASN1 is the first network's Autonomous System Number.
	ASN1 int
	// ASN2 is the second network's Autonomous System Number.
	ASN2 int
	// ViewMode is "shared" (default) or "full".
	ViewMode string
}

// Compare loads both networks' presences and computes set intersections
// to find shared IXPs, facilities, and campuses.
func (s *CompareService) Compare(ctx context.Context, input CompareInput) (*templates.CompareData, error) {
	if input.ViewMode == "" {
		input.ViewMode = "shared"
	}

	// Look up both networks by ASN.
	netA, err := s.client.Network.Query().Where(network.Asn(input.ASN1)).First(ctx)
	if err != nil {
		return nil, fmt.Errorf("network ASN %d: %w", input.ASN1, err)
	}

	netB, err := s.client.Network.Query().Where(network.Asn(input.ASN2)).First(ctx)
	if err != nil {
		return nil, fmt.Errorf("network ASN %d: %w", input.ASN2, err)
	}

	// Load presences in parallel (CC-4: errgroup for fan-out).
	// Pre-allocate result slots so each goroutine writes to its own index.
	var (
		ixLansA  []*ent.NetworkIxLan
		ixLansB  []*ent.NetworkIxLan
		facNetsA []*ent.NetworkFacility
		facNetsB []*ent.NetworkFacility
	)

	g, gctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		var err error
		ixLansA, err = s.client.NetworkIxLan.Query().
			Where(networkixlan.HasNetworkWith(network.ID(netA.ID))).
			All(gctx)
		if err != nil {
			return fmt.Errorf("query IX presences for ASN %d: %w", input.ASN1, err)
		}
		return nil
	})

	g.Go(func() error {
		var err error
		ixLansB, err = s.client.NetworkIxLan.Query().
			Where(networkixlan.HasNetworkWith(network.ID(netB.ID))).
			All(gctx)
		if err != nil {
			return fmt.Errorf("query IX presences for ASN %d: %w", input.ASN2, err)
		}
		return nil
	})

	g.Go(func() error {
		var err error
		facNetsA, err = s.client.NetworkFacility.Query().
			Where(networkfacility.HasNetworkWith(network.ID(netA.ID))).
			WithFacility(). // Eager-load for coordinates
			All(gctx)
		if err != nil {
			return fmt.Errorf("query facility presences for ASN %d: %w", input.ASN1, err)
		}
		return nil
	})

	g.Go(func() error {
		var err error
		facNetsB, err = s.client.NetworkFacility.Query().
			Where(networkfacility.HasNetworkWith(network.ID(netB.ID))).
			WithFacility(). // Eager-load for coordinates
			All(gctx)
		if err != nil {
			return fmt.Errorf("query facility presences for ASN %d: %w", input.ASN2, err)
		}
		return nil
	})

	if err := g.Wait(); err != nil {
		return nil, fmt.Errorf("load presences: %w", err)
	}

	// Compute shared IXPs.
	sharedIXPs := computeSharedIXPs(ixLansA, ixLansB)

	// Compute shared facilities.
	sharedFacs := computeSharedFacilities(facNetsA, facNetsB)

	// Compute shared campuses from facility presences.
	sharedCampuses, err := s.computeSharedCampuses(ctx, facNetsA, facNetsB)
	if err != nil {
		return nil, fmt.Errorf("compute shared campuses: %w", err)
	}

	data := &templates.CompareData{
		NetA: templates.CompareNetwork{
			ASN:  netA.Asn,
			Name: netA.Name,
			ID:   netA.ID,
		},
		NetB: templates.CompareNetwork{
			ASN:  netB.Asn,
			Name: netB.Name,
			ID:   netB.ID,
		},
		SharedIXPs:       sharedIXPs,
		SharedFacilities: sharedFacs,
		SharedCampuses:   sharedCampuses,
		ViewMode:         input.ViewMode,
	}

	// Always compute all facilities for map rendering (D-08, D-12).
	data.AllFacilities = computeAllFacilities(facNetsA, facNetsB)

	// Full view: compute union of all IXP presences with shared flags.
	if input.ViewMode == "full" {
		data.AllIXPs = computeAllIXPs(ixLansA, ixLansB)
	}

	return data, nil
}

// computeSharedIXPs finds IXPs where both networks are present by intersecting IX IDs.
func computeSharedIXPs(a, b []*ent.NetworkIxLan) []templates.CompareIXP {
	// Build map of IX ID -> presence for network A.
	ixMapA := make(map[int]*ent.NetworkIxLan, len(a))
	for _, nixl := range a {
		ixMapA[nixl.IxID] = nixl
	}

	var shared []templates.CompareIXP
	seen := make(map[int]bool)

	for _, nixl := range b {
		if nixlA, ok := ixMapA[nixl.IxID]; ok && !seen[nixl.IxID] {
			seen[nixl.IxID] = true
			shared = append(shared, templates.CompareIXP{
				IXID:   nixl.IxID,
				IXName: nixlA.Name,
				Shared: true,
				NetA:   ixPresence(nixlA),
				NetB:   ixPresence(nixl),
			})
		}
	}

	sort.Slice(shared, func(i, j int) bool { return shared[i].IXName < shared[j].IXName })
	return shared
}

// computeSharedFacilities finds facilities where both networks are present by intersecting facility IDs.
func computeSharedFacilities(a, b []*ent.NetworkFacility) []templates.CompareFacility {
	// Build map of facility ID -> presence for network A.
	facMapA := make(map[int]*ent.NetworkFacility, len(a))
	for _, nf := range a {
		if nf.FacID == nil {
			continue
		}
		facMapA[*nf.FacID] = nf
	}

	var shared []templates.CompareFacility
	seen := make(map[int]bool)

	for _, nf := range b {
		if nf.FacID == nil {
			continue
		}
		facID := *nf.FacID
		if nfA, ok := facMapA[facID]; ok && !seen[facID] {
			seen[facID] = true
			cf := templates.CompareFacility{
				FacID:   facID,
				FacName: nfA.Name,
				City:    nfA.City,
				Country: nfA.Country,
				Shared:  true,
				NetA:    &templates.CompareFacPresence{LocalASN: nfA.LocalAsn},
				NetB:    &templates.CompareFacPresence{LocalASN: nf.LocalAsn},
			}
			if fac := nfA.Edges.Facility; fac != nil {
				if fac.Latitude != nil {
					cf.Latitude = *fac.Latitude
				}
				if fac.Longitude != nil {
					cf.Longitude = *fac.Longitude
				}
			}
			shared = append(shared, cf)
		}
	}

	sort.Slice(shared, func(i, j int) bool { return shared[i].FacName < shared[j].FacName })
	return shared
}

// computeSharedCampuses derives shared campuses from both networks' facility presences.
// For each network, it finds which facilities belong to a campus, then intersects campus IDs.
func (s *CompareService) computeSharedCampuses(ctx context.Context, facNetsA, facNetsB []*ent.NetworkFacility) ([]templates.CompareCampus, error) {
	facIDsA := extractFacIDs(facNetsA)
	facIDsB := extractFacIDs(facNetsB)

	if len(facIDsA) == 0 || len(facIDsB) == 0 {
		return nil, nil
	}

	// Query facilities with campus edges for both networks.
	campusMapA, err := s.facCampusMap(ctx, facIDsA)
	if err != nil {
		return nil, fmt.Errorf("campus map for network A: %w", err)
	}

	campusMapB, err := s.facCampusMap(ctx, facIDsB)
	if err != nil {
		return nil, fmt.Errorf("campus map for network B: %w", err)
	}

	// Build campus ID -> facility IDs for each network.
	campusFacsA := groupByCampus(campusMapA)
	campusFacsB := groupByCampus(campusMapB)

	// Intersect campus IDs.
	var shared []templates.CompareCampus
	for campusID, facsA := range campusFacsA {
		facsB, ok := campusFacsB[campusID]
		if !ok {
			continue
		}

		// Find campus name from either side.
		var campusName string
		for _, f := range facsA {
			if f.campusName != "" {
				campusName = f.campusName
				break
			}
		}

		// Find facilities present in both networks within this campus.
		facSetA := make(map[int]bool, len(facsA))
		for _, f := range facsA {
			facSetA[f.facID] = true
		}

		var sharedFacs []templates.CompareCampusFacility
		for _, f := range facsB {
			if facSetA[f.facID] {
				sharedFacs = append(sharedFacs, templates.CompareCampusFacility{
					FacID:   f.facID,
					FacName: f.facName,
				})
			}
		}

		shared = append(shared, templates.CompareCampus{
			CampusID:         campusID,
			CampusName:       campusName,
			SharedFacilities: sharedFacs,
		})
	}

	sort.Slice(shared, func(i, j int) bool { return shared[i].CampusName < shared[j].CampusName })
	return shared, nil
}

// facCampusInfo holds a facility's campus membership details.
type facCampusInfo struct {
	facID      int
	facName    string
	campusID   int
	campusName string
}

// facCampusMap queries facilities by ID that have a campus and returns a mapping.
func (s *CompareService) facCampusMap(ctx context.Context, facIDs []int) ([]facCampusInfo, error) {
	facs, err := s.client.Facility.Query().
		Where(facility.IDIn(facIDs...), facility.HasCampus()).
		WithCampus().
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("query campus facilities: %w", err)
	}

	result := make([]facCampusInfo, 0, len(facs))
	for _, f := range facs {
		if f.Edges.Campus == nil {
			continue
		}
		result = append(result, facCampusInfo{
			facID:      f.ID,
			facName:    f.Name,
			campusID:   f.Edges.Campus.ID,
			campusName: f.Edges.Campus.Name,
		})
	}
	return result, nil
}

// groupByCampus groups facCampusInfo entries by campus ID.
func groupByCampus(infos []facCampusInfo) map[int][]facCampusInfo {
	m := make(map[int][]facCampusInfo)
	for _, info := range infos {
		m[info.campusID] = append(m[info.campusID], info)
	}
	return m
}

// extractFacIDs extracts facility IDs from network facility presences, skipping nil FacIDs.
func extractFacIDs(nfs []*ent.NetworkFacility) []int {
	ids := make([]int, 0, len(nfs))
	for _, nf := range nfs {
		if nf.FacID != nil {
			ids = append(ids, *nf.FacID)
		}
	}
	return ids
}

// computeAllIXPs produces a union of both networks' IXP presences with shared flags.
func computeAllIXPs(a, b []*ent.NetworkIxLan) []templates.CompareIXP {
	type ixEntry struct {
		ixID   int
		ixName string
		netA   *templates.CompareIXPresence
		netB   *templates.CompareIXPresence
	}

	entries := make(map[int]*ixEntry)

	for _, nixl := range a {
		entries[nixl.IxID] = &ixEntry{
			ixID:   nixl.IxID,
			ixName: nixl.Name,
			netA:   ixPresence(nixl),
		}
	}

	for _, nixl := range b {
		if e, ok := entries[nixl.IxID]; ok {
			e.netB = ixPresence(nixl)
		} else {
			entries[nixl.IxID] = &ixEntry{
				ixID:   nixl.IxID,
				ixName: nixl.Name,
				netB:   ixPresence(nixl),
			}
		}
	}

	result := make([]templates.CompareIXP, 0, len(entries))
	for _, e := range entries {
		result = append(result, templates.CompareIXP{
			IXID:   e.ixID,
			IXName: e.ixName,
			Shared: e.netA != nil && e.netB != nil,
			NetA:   e.netA,
			NetB:   e.netB,
		})
	}

	sort.Slice(result, func(i, j int) bool { return result[i].IXName < result[j].IXName })
	return result
}

// computeAllFacilities produces a union of both networks' facility presences with shared flags.
func computeAllFacilities(a, b []*ent.NetworkFacility) []templates.CompareFacility {
	type facEntry struct {
		facID   int
		facName string
		city    string
		country string
		lat     float64
		lng     float64
		netA    *templates.CompareFacPresence
		netB    *templates.CompareFacPresence
	}

	// extractCoords populates lat/lng from the eager-loaded Facility edge.
	extractCoords := func(nf *ent.NetworkFacility, e *facEntry) {
		if fac := nf.Edges.Facility; fac != nil {
			if fac.Latitude != nil {
				e.lat = *fac.Latitude
			}
			if fac.Longitude != nil {
				e.lng = *fac.Longitude
			}
		}
	}

	entries := make(map[int]*facEntry)

	for _, nf := range a {
		if nf.FacID == nil {
			continue
		}
		e := &facEntry{
			facID:   *nf.FacID,
			facName: nf.Name,
			city:    nf.City,
			country: nf.Country,
			netA:    &templates.CompareFacPresence{LocalASN: nf.LocalAsn},
		}
		extractCoords(nf, e)
		entries[*nf.FacID] = e
	}

	for _, nf := range b {
		if nf.FacID == nil {
			continue
		}
		facID := *nf.FacID
		if e, ok := entries[facID]; ok {
			e.netB = &templates.CompareFacPresence{LocalASN: nf.LocalAsn}
		} else {
			e := &facEntry{
				facID:   facID,
				facName: nf.Name,
				city:    nf.City,
				country: nf.Country,
				netB:    &templates.CompareFacPresence{LocalASN: nf.LocalAsn},
			}
			extractCoords(nf, e)
			entries[facID] = e
		}
	}

	result := make([]templates.CompareFacility, 0, len(entries))
	for _, e := range entries {
		result = append(result, templates.CompareFacility{
			FacID:     e.facID,
			FacName:   e.facName,
			City:      e.city,
			Country:   e.country,
			Latitude:  e.lat,
			Longitude: e.lng,
			Shared:    e.netA != nil && e.netB != nil,
			NetA:      e.netA,
			NetB:      e.netB,
		})
	}

	sort.Slice(result, func(i, j int) bool { return result[i].FacName < result[j].FacName })
	return result
}

// ixPresence converts an ent NetworkIxLan to a template CompareIXPresence.
func ixPresence(nixl *ent.NetworkIxLan) *templates.CompareIXPresence {
	p := &templates.CompareIXPresence{
		Speed:       nixl.Speed,
		IsRSPeer:    nixl.IsRsPeer,
		Operational: nixl.Operational,
	}
	if nixl.Ipaddr4 != nil {
		p.IPAddr4 = *nixl.Ipaddr4
	}
	if nixl.Ipaddr6 != nil {
		p.IPAddr6 = *nixl.Ipaddr6
	}
	return p
}
