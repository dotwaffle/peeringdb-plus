package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/dotwaffle/peeringdb-plus/internal/peeringdb"
)

// discoverIDs queries `/api/<type>?limit=1` for each of the 13
// entity types and returns a map[type]firstID. Used to seed
// get-by-id and ConnectRPC Get<Type> request bodies with real ids
// so the smoke run doesn't 404 on entities where the lower id range
// is sparse on the live mirror (org, poc, netfac, netixlan all have
// first-active-id well above 1).
//
// On any error per type, the map entry is omitted; downstream code
// falls back to id=1 for that type. Stderr gets a single warning
// when verbose is on so the operator knows which lookups missed.
//
// The 13 lookups fan out concurrently. With ~50ms per request to a
// healthy mirror, total wall-clock is bounded by the slowest single
// lookup, not the sum.
func discoverIDs(ctx context.Context, cfg Config, out io.Writer) map[string]int {
	types := []string{
		peeringdb.TypeOrg, peeringdb.TypeNet, peeringdb.TypeFac,
		peeringdb.TypeIX, peeringdb.TypePoc, peeringdb.TypeIXLan,
		peeringdb.TypeIXPfx, peeringdb.TypeNetIXLan, peeringdb.TypeNetFac,
		peeringdb.TypeIXFac, peeringdb.TypeCarrier, peeringdb.TypeCarrierFac,
		peeringdb.TypeCampus,
	}
	out2 := out
	if !cfg.Verbose {
		out2 = io.Discard
	}

	type result struct {
		t  string
		id int
	}
	ch := make(chan result, len(types))
	var wg sync.WaitGroup
	for _, t := range types {
		wg.Add(1)
		go func(t string) {
			defer wg.Done()
			id, err := lookupFirstID(ctx, cfg, t)
			if err != nil {
				fmt.Fprintf(out2, "  discoverIDs: %s -> %v (falling back to id=1)\n", t, err)
				return
			}
			ch <- result{t, id}
		}(t)
	}
	go func() { wg.Wait(); close(ch) }()

	ids := map[string]int{}
	for r := range ch {
		ids[r.t] = r.id
	}

	if cfg.Verbose {
		fmt.Fprintf(out, "discovered ids:")
		for _, t := range types {
			if id, ok := ids[t]; ok {
				fmt.Fprintf(out, " %s=%d", t, id)
			} else {
				fmt.Fprintf(out, " %s=miss", t)
			}
		}
		fmt.Fprintln(out)
	}
	return ids
}

// discoverRampIDs prefetches a stable set of real IDs (and ASNs for
// entity=="net") from the deployed mirror so that the ramp loop can
// round-robin through them without warming a single row's cache. The
// fetch is one HTTP call: GET /api/<entity>?limit=<count>.
//
// For entity=="net" the asns slice is populated in parallel with ids
// (one ASN per network row) so the Web UI surface can hit
// /ui/asn/<asn> rather than /ui/net/<id> — the project's network
// detail page is keyed on ASN per the templ route table. For
// entity=="org" asns is returned nil; the Web UI surface uses
// /ui/org/<id> directly.
//
// The 5s timeout matches lookupFirstID's per-type budget; ramp's
// total prefetch wall-clock is bounded by this single call. An empty
// data array is fatal — the operator has pointed --target at a
// deployment that has no rows of this entity, and the ramp cannot
// proceed without IDs to drive against.
func discoverRampIDs(ctx context.Context, cfg Config, entity string, count int) ([]int, []int, error) {
	if count <= 0 {
		return nil, nil, fmt.Errorf("count must be > 0, got %d", count)
	}
	switch entity {
	case "net", "org":
	default:
		return nil, nil, fmt.Errorf("unsupported entity %q (want net|org)", entity)
	}

	u, err := url.Parse(cfg.Base)
	if err != nil {
		return nil, nil, fmt.Errorf("parse base: %w", err)
	}
	u.Path = "/api/" + entity
	u.RawQuery = fmt.Sprintf("limit=%d", count)

	reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, nil, err
	}
	if cfg.AuthToken != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.AuthToken)
	}
	resp, err := cfg.HTTPClient.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("ramp prefetch: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode/100 != 2 {
		return nil, nil, fmt.Errorf("ramp prefetch %s: status %d", u.String(), resp.StatusCode)
	}

	var body struct {
		Data []struct {
			ID  int `json:"id"`
			ASN int `json:"asn"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, nil, fmt.Errorf("ramp prefetch decode: %w", err)
	}
	if len(body.Data) == 0 {
		return nil, nil, fmt.Errorf("ramp prefetch %s: empty data array (target has no %s rows)", u.String(), entity)
	}

	ids := make([]int, 0, len(body.Data))
	var asns []int
	if entity == "net" {
		asns = make([]int, 0, len(body.Data))
	}
	for _, row := range body.Data {
		if row.ID <= 0 {
			continue
		}
		ids = append(ids, row.ID)
		if entity == "net" {
			asns = append(asns, row.ASN)
		}
	}
	if len(ids) == 0 {
		return nil, nil, fmt.Errorf("ramp prefetch %s: no positive ids in %d-row response", u.String(), len(body.Data))
	}
	return ids, asns, nil
}

// lookupFirstID fetches /api/<t>?limit=1 and returns the id of the
// first record. Tolerates the wrapper `{"data":[{...}],"meta":{}}`
// shape used by pdbcompat.
func lookupFirstID(ctx context.Context, cfg Config, t string) (int, error) {
	u, err := url.Parse(cfg.Base)
	if err != nil {
		return 0, fmt.Errorf("parse base: %w", err)
	}
	u.Path = "/api/" + t
	u.RawQuery = "limit=1"

	reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, "GET", u.String(), nil)
	if err != nil {
		return 0, err
	}
	if cfg.AuthToken != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.AuthToken)
	}
	resp, err := cfg.HTTPClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode/100 != 2 {
		return 0, fmt.Errorf("status %d", resp.StatusCode)
	}
	var body struct {
		Data []struct {
			ID int `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return 0, fmt.Errorf("decode: %w", err)
	}
	if len(body.Data) == 0 {
		return 0, fmt.Errorf("empty data array")
	}
	if body.Data[0].ID <= 0 {
		return 0, fmt.Errorf("non-positive id %d", body.Data[0].ID)
	}
	return body.Data[0].ID, nil
}
