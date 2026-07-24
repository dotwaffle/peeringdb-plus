// Package mcpserver exposes the local PeeringDB mirror over MCP.
package mcpserver

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/netip"
	"reflect"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"

	"github.com/dotwaffle/peeringdb-plus/ent"
	"github.com/dotwaffle/peeringdb-plus/ent/ixprefix"
	"github.com/dotwaffle/peeringdb-plus/ent/networkixlan"
	"github.com/dotwaffle/peeringdb-plus/internal/catalog"
	pdbsync "github.com/dotwaffle/peeringdb-plus/internal/sync"
)

const (
	defaultPageSize = 20
	maxPageSize     = 100
	maxQueryLength  = 4096
)

var tracer = otel.Tracer("github.com/dotwaffle/peeringdb-plus/internal/mcpserver")

// Input contains the MCP server's explicit dependencies.
type Input struct {
	Client         *ent.Client
	DB             *sql.DB
	Version        string
	Region         string
	AllowedOrigins string
	Logger         *slog.Logger
}

// New returns a stateless Streamable HTTP MCP handler.
func New(input Input) http.Handler {
	server := mcp.NewServer(
		&mcp.Implementation{
			Name:    "peeringdb-plus",
			Title:   "PeeringDB Plus",
			Version: input.Version,
		},
		&mcp.ServerOptions{
			Instructions: "Read-only access to the locally mirrored PeeringDB catalog. Check sync freshness when it matters.",
			Logger:       input.Logger,
		},
	)

	services := toolServices{
		catalog: catalog.NewService(input.Client),
		search:  catalog.NewSearchService(input.Client),
		compare: catalog.NewCompareService(input.Client),
		client:  input.Client,
		db:      input.DB,
	}
	addTools(server, services)
	addResources(server, input)
	addPrompts(server)

	stream := mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return server },
		&mcp.StreamableHTTPOptions{
			Stateless:             true,
			JSONResponse:          true,
			Logger:                input.Logger,
			CrossOriginProtection: sdkCrossOriginProtection(),
		},
	)
	return originGuard(input.AllowedOrigins, stream)
}

func sdkCrossOriginProtection() *http.CrossOriginProtection {
	protection := http.NewCrossOriginProtection()
	// This handler is mounted only at /mcp and is already wrapped by
	// originGuard, which understands the application's exact and wildcard
	// PDBPLUS_CORS_ORIGINS policy. Bypass the SDK's same-origin-only default so
	// configured browser clients are not rejected a second time.
	protection.AddInsecureBypassPattern("/mcp")
	return protection
}

type toolServices struct {
	catalog *catalog.Service
	search  *catalog.SearchService
	compare *catalog.CompareService
	client  *ent.Client
	db      *sql.DB
}

type searchInput struct {
	Query    string `json:"query" jsonschema:"Search text, ASN, or entity name (2-4096 characters)."`
	Type     string `json:"type,omitempty" jsonschema:"Optional entity type: net, ix, fac, org, campus, or carrier."`
	Cursor   string `json:"cursor,omitempty" jsonschema:"Opaque cursor returned by an earlier typed search."`
	PageSize int    `json:"page_size,omitempty" jsonschema:"Results per page; defaults to 20 and is capped at 100."`
}

type networkInput struct {
	ASN      int    `json:"asn" jsonschema:"Autonomous System Number."`
	Relation string `json:"relation,omitempty" jsonschema:"Optional related collection: ix_presences or facilities."`
	Cursor   string `json:"cursor,omitempty" jsonschema:"Opaque relation cursor from an earlier response."`
	PageSize int    `json:"page_size,omitempty" jsonschema:"Related records per page; defaults to 20 and is capped at 100."`
}

type entityInput struct {
	ID       int    `json:"id" jsonschema:"PeeringDB entity ID."`
	Relation string `json:"relation,omitempty" jsonschema:"Optional related collection named by the tool description."`
	Cursor   string `json:"cursor,omitempty" jsonschema:"Opaque relation cursor from an earlier response."`
	PageSize int    `json:"page_size,omitempty" jsonschema:"Related records per page; defaults to 20 and is capped at 100."`
}

type compareInput struct {
	ASN1 int `json:"asn1" jsonschema:"First Autonomous System Number."`
	ASN2 int `json:"asn2" jsonschema:"Second Autonomous System Number."`
}

type lookupIPInput struct {
	IP string `json:"ip" jsonschema:"IPv4 or IPv6 address to identify."`
}

type emptyInput struct{}

type cursorPayload struct {
	Version   int    `json:"v"`
	Scope     string `json:"scope"`
	Parent    int    `json:"parent"`
	Offset    int    `json:"offset"`
	Watermark string `json:"watermark"`
}

type relationPage struct {
	Items      any    `json:"items"`
	NextCursor string `json:"next_cursor,omitempty"`
}

type entityOutput struct {
	Entity    any                     `json:"entity"`
	Relations map[string]relationPage `json:"relations,omitempty"`
	Freshness string                  `json:"freshness,omitempty"`
}

func addTools(server *mcp.Server, services toolServices) {
	addReadTool[searchInput](server, "search_peeringdb",
		"Search PeeringDB entities. Omit type for grouped previews; set type for cursor pagination.",
		func(ctx context.Context, input searchInput) (any, error) {
			if len(input.Query) > maxQueryLength {
				return nil, fmt.Errorf("query exceeds %d characters", maxQueryLength)
			}
			if input.Type == "" {
				results, err := services.search.Search(ctx, input.Query)
				if err != nil {
					return nil, err
				}
				for i := range results {
					if len(results[i].Results) > 5 {
						results[i].Results = results[i].Results[:5]
						results[i].HasMore = true
					}
				}
				return map[string]any{"groups": results}, nil
			}

			watermark, err := freshness(ctx, services.db)
			if err != nil {
				return nil, err
			}
			offset, err := cursorOffset(input.Cursor, "search:"+input.Type, 0, watermark)
			if err != nil {
				return nil, err
			}
			size := pageSize(input.PageSize)
			result, err := services.search.SearchType(ctx, catalog.SearchTypeInput{
				Query: input.Query, TypeSlug: input.Type, Offset: offset, Limit: size,
			})
			if err != nil {
				return nil, err
			}
			next := ""
			if result.HasMore {
				next = encodeCursor(cursorPayload{
					Version: 1, Scope: "search:" + input.Type, Offset: offset + len(result.Hits), Watermark: watermark,
				})
			}
			return map[string]any{
				"type": result.TypeSlug, "type_name": result.TypeName, "items": result.Hits,
				"total": result.Total, "next_cursor": next, "freshness": watermark,
			}, nil
		})

	addReadTool[networkInput](server, "get_network",
		"Get a network by ASN with bounded ix_presences and facilities relations.",
		func(ctx context.Context, input networkInput) (any, error) {
			entity, err := services.catalog.Network(ctx, input.ASN)
			if err != nil {
				return nil, err
			}
			relations := map[string]any{"ix_presences": entity.IXPresences, "facilities": entity.FacPresences}
			entity.IXPresences = nil
			entity.FacPresences = nil
			return pageEntity(ctx, services.db, "network", input.ASN, entity, input.Relation, input.Cursor, input.PageSize,
				relations)
		})

	addReadTool[entityInput](server, "get_exchange",
		"Get an exchange by ID with bounded participants, facilities, and prefixes relations.",
		func(ctx context.Context, input entityInput) (any, error) {
			entity, err := services.catalog.IX(ctx, input.ID)
			if err != nil {
				return nil, err
			}
			relations := map[string]any{"participants": entity.Participants, "facilities": entity.Facilities, "prefixes": entity.Prefixes}
			entity.Participants = nil
			entity.Facilities = nil
			entity.Prefixes = nil
			return pageEntity(ctx, services.db, "exchange", input.ID, entity, input.Relation, input.Cursor, input.PageSize,
				relations)
		})

	addReadTool[entityInput](server, "get_facility",
		"Get a facility by ID with bounded networks, exchanges, and carriers relations.",
		func(ctx context.Context, input entityInput) (any, error) {
			entity, err := services.catalog.Facility(ctx, input.ID)
			if err != nil {
				return nil, err
			}
			relations := map[string]any{"networks": entity.Networks, "exchanges": entity.IXPs, "carriers": entity.Carriers}
			entity.Networks = nil
			entity.IXPs = nil
			entity.Carriers = nil
			return pageEntity(ctx, services.db, "facility", input.ID, entity, input.Relation, input.Cursor, input.PageSize,
				relations)
		})

	addReadTool[entityInput](server, "get_organization",
		"Get an organization by ID with bounded networks, exchanges, facilities, campuses, and carriers relations.",
		func(ctx context.Context, input entityInput) (any, error) {
			entity, err := services.catalog.Organization(ctx, input.ID)
			if err != nil {
				return nil, err
			}
			relations := map[string]any{
				"networks": entity.Networks, "exchanges": entity.IXPs, "facilities": entity.Facs,
				"campuses": entity.Campuses, "carriers": entity.Carriers,
			}
			entity.Networks = nil
			entity.IXPs = nil
			entity.Facs = nil
			entity.Campuses = nil
			entity.Carriers = nil
			return pageEntity(ctx, services.db, "organization", input.ID, entity, input.Relation, input.Cursor, input.PageSize,
				relations)
		})

	addReadTool[entityInput](server, "get_campus",
		"Get a campus by ID with a bounded facilities relation.",
		func(ctx context.Context, input entityInput) (any, error) {
			entity, err := services.catalog.Campus(ctx, input.ID)
			if err != nil {
				return nil, err
			}
			relations := map[string]any{"facilities": entity.Facilities}
			entity.Facilities = nil
			return pageEntity(ctx, services.db, "campus", input.ID, entity, input.Relation, input.Cursor, input.PageSize,
				relations)
		})

	addReadTool[entityInput](server, "get_carrier",
		"Get a carrier by ID with a bounded facilities relation.",
		func(ctx context.Context, input entityInput) (any, error) {
			entity, err := services.catalog.Carrier(ctx, input.ID)
			if err != nil {
				return nil, err
			}
			relations := map[string]any{"facilities": entity.Facilities}
			entity.Facilities = nil
			return pageEntity(ctx, services.db, "carrier", input.ID, entity, input.Relation, input.Cursor, input.PageSize,
				relations)
		})

	addReadTool[compareInput](server, "compare_networks",
		"Compare two ASNs across shared exchanges, facilities, and campuses.",
		func(ctx context.Context, input compareInput) (any, error) {
			result, err := services.compare.Compare(ctx, catalog.CompareInput{ASN1: input.ASN1, ASN2: input.ASN2, ViewMode: "full"})
			if err != nil {
				return nil, err
			}
			return boundedComparison(result), nil
		})

	addReadTool[lookupIPInput](server, "lookup_ip",
		"Find an exact network peering address and the containing exchange prefix.",
		func(ctx context.Context, input lookupIPInput) (any, error) {
			return services.lookupIP(ctx, input.IP)
		})

	addReadTool[emptyInput](server, "get_sync_status",
		"Get mirror freshness and the latest synchronization result.",
		func(ctx context.Context, _ emptyInput) (any, error) {
			status, err := pdbsync.GetLastStatus(ctx, services.db)
			if err != nil {
				return nil, err
			}
			if status == nil {
				return map[string]any{"status": "never_synced"}, nil
			}
			return map[string]any{
				"status": status.Status, "last_sync_at": status.LastSyncAt.UTC().Format(time.RFC3339),
				"duration_ms": status.Duration.Milliseconds(), "object_counts": status.ObjectCounts,
			}, nil
		})
}

func addReadTool[In any](server *mcp.Server, name, description string, handler func(context.Context, In) (any, error)) {
	no := false
	mcp.AddTool[In, any](server, &mcp.Tool{
		Name:        name,
		Description: description,
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint: true, IdempotentHint: true, DestructiveHint: &no, OpenWorldHint: &no,
		},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input In) (*mcp.CallToolResult, any, error) {
		ctx, span := tracer.Start(ctx, "mcp.tool/"+name)
		span.SetAttributes(attribute.String("mcp.tool.name", name))
		defer span.End()
		output, err := handler(ctx, input)
		if err != nil {
			span.RecordError(err)
		}
		return &mcp.CallToolResult{}, output, err
	})
}

func pageEntity(
	ctx context.Context,
	db *sql.DB,
	scope string,
	parent int,
	entity any,
	selected string,
	cursor string,
	requestedSize int,
	relations map[string]any,
) (entityOutput, error) {
	watermark, err := freshness(ctx, db)
	if err != nil {
		return entityOutput{}, err
	}
	if selected != "" {
		if _, ok := relations[selected]; !ok {
			return entityOutput{}, fmt.Errorf("unknown relation %q", selected)
		}
	}
	if cursor != "" && selected == "" {
		return entityOutput{}, errors.New("relation is required when cursor is set")
	}

	output := entityOutput{Entity: entity, Relations: make(map[string]relationPage), Freshness: watermark}
	for name, items := range relations {
		if selected != "" && selected != name {
			continue
		}
		offset, err := cursorOffset(cursor, scope+":"+name, parent, watermark)
		if err != nil {
			return entityOutput{}, err
		}
		page, more, count, err := slicePage(items, offset, pageSize(requestedSize))
		if err != nil {
			return entityOutput{}, fmt.Errorf("page %s: %w", name, err)
		}
		relation := relationPage{Items: page}
		if more {
			relation.NextCursor = encodeCursor(cursorPayload{
				Version: 1, Scope: scope + ":" + name, Parent: parent, Offset: offset + count, Watermark: watermark,
			})
		}
		output.Relations[name] = relation
	}
	return output, nil
}

func slicePage(items any, offset, size int) (any, bool, int, error) {
	value := reflect.ValueOf(items)
	if value.Kind() != reflect.Slice {
		return nil, false, 0, errors.New("relation is not a slice")
	}
	if offset < 0 || offset > value.Len() {
		return nil, false, 0, errors.New("cursor is outside the result set")
	}
	end := min(offset+size, value.Len())
	return value.Slice(offset, end).Interface(), end < value.Len(), end - offset, nil
}

func boundedComparison(result *catalog.CompareData) map[string]any {
	return map[string]any{
		"network_a": result.NetA,
		"network_b": result.NetB,
		"shared_exchanges": map[string]any{
			"items": result.SharedIXPs[:min(len(result.SharedIXPs), maxPageSize)],
			"total": len(result.SharedIXPs),
		},
		"shared_facilities": map[string]any{
			"items": result.SharedFacilities[:min(len(result.SharedFacilities), maxPageSize)],
			"total": len(result.SharedFacilities),
		},
		"shared_campuses": map[string]any{
			"items": result.SharedCampuses[:min(len(result.SharedCampuses), maxPageSize)],
			"total": len(result.SharedCampuses),
		},
	}
}

func pageSize(value int) int {
	if value <= 0 {
		return defaultPageSize
	}
	return min(value, maxPageSize)
}

func freshness(ctx context.Context, db *sql.DB) (string, error) {
	value, err := pdbsync.GetLastSuccessfulSyncTime(ctx, db)
	if err != nil {
		return "", fmt.Errorf("read sync freshness: %w", err)
	}
	if value.IsZero() {
		return "unsynced", nil
	}
	return value.UTC().Format(time.RFC3339Nano), nil
}

func encodeCursor(cursor cursorPayload) string {
	data, _ := json.Marshal(cursor)
	return base64.RawURLEncoding.EncodeToString(data)
}

func cursorOffset(value, scope string, parent int, watermark string) (int, error) {
	if value == "" {
		return 0, nil
	}
	data, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return 0, errors.New("invalid cursor encoding")
	}
	var cursor cursorPayload
	if err := json.Unmarshal(data, &cursor); err != nil {
		return 0, errors.New("invalid cursor")
	}
	if cursor.Version != 1 || cursor.Scope != scope || cursor.Parent != parent {
		return 0, errors.New("cursor does not match this request")
	}
	if cursor.Watermark != watermark {
		return 0, errors.New("cursor is stale because the mirror has synchronized")
	}
	return cursor.Offset, nil
}

func (services toolServices) lookupIP(ctx context.Context, raw string) (any, error) {
	address, err := netip.ParseAddr(strings.TrimSpace(raw))
	if err != nil {
		return nil, fmt.Errorf("invalid IP address: %w", err)
	}
	canonical := address.String()
	exact, err := services.client.NetworkIxLan.Query().
		Where(
			networkixlan.StatusIn("ok", "pending"),
			networkixlan.Or(networkixlan.Ipaddr4(canonical), networkixlan.Ipaddr6(canonical)),
		).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("query peering addresses: %w", err)
	}

	protocol := "IPv6"
	if address.Is4() {
		protocol = "IPv4"
	}
	prefixes, err := services.client.IxPrefix.Query().
		Where(ixprefix.StatusIn("ok", "pending"), ixprefix.Protocol(protocol)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("query exchange prefixes: %w", err)
	}
	containing := make([]*ent.IxPrefix, 0, 1)
	for _, candidate := range prefixes {
		prefix, parseErr := netip.ParsePrefix(candidate.Prefix)
		if parseErr == nil && prefix.Contains(address) {
			containing = append(containing, candidate)
		}
	}
	return map[string]any{"ip": canonical, "network_presences": exact, "exchange_prefixes": containing}, nil
}

func addResources(server *mcp.Server, input Input) {
	serviceText := fmt.Sprintf(
		"PeeringDB Plus %s is a read-only local PeeringDB mirror. Region: %s. Use get_sync_status for live freshness.",
		input.Version, input.Region,
	)
	addTextResource(server, "peeringdb-plus://service", "Service context", serviceText)
	addTextResource(server, "peeringdb-plus://guide", "Research guide",
		"Search first, then fetch details by ASN or PeeringDB ID. Related collections are bounded; follow next_cursor with the same relation. Cursors expire after a successful mirror sync.")
}

func addTextResource(server *mcp.Server, uri, name, text string) {
	server.AddResource(&mcp.Resource{
		URI: uri, Name: name, MIMEType: "text/plain", Description: name,
	}, func(context.Context, *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{
			URI: uri, MIMEType: "text/plain", Text: text,
		}}}, nil
	})
}

func addPrompts(server *mcp.Server) {
	server.AddPrompt(&mcp.Prompt{
		Name: "research_network", Description: "Investigate one network and its interconnection footprint",
		Arguments: []*mcp.PromptArgument{{Name: "asn", Description: "Autonomous System Number", Required: true}},
	}, func(_ context.Context, request *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		asn := request.Params.Arguments["asn"]
		return promptResult("Research AS" + asn + ". Check mirror freshness, fetch the network, summarize its exchange and facility footprint, and distinguish facts from inferences."), nil
	})
	server.AddPrompt(&mcp.Prompt{
		Name: "compare_networks", Description: "Compare two networks' interconnection footprints",
		Arguments: []*mcp.PromptArgument{
			{Name: "asn1", Description: "First ASN", Required: true},
			{Name: "asn2", Description: "Second ASN", Required: true},
		},
	}, func(_ context.Context, request *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		args := request.Params.Arguments
		return promptResult("Compare AS" + args["asn1"] + " with AS" + args["asn2"] + " using compare_networks. Report shared and distinct exchanges, facilities, and campuses."), nil
	})
}

func promptResult(text string) *mcp.GetPromptResult {
	return &mcp.GetPromptResult{Messages: []*mcp.PromptMessage{{
		Role: "user", Content: &mcp.TextContent{Text: text},
	}}}
}
