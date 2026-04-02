// Package graphql provides the HTTP handler factory for the PeeringDB Plus GraphQL API.
package graphql

import (
	"context"
	"html/template"
	"net/http"
	"strings"

	"github.com/99designs/gqlgen/graphql"
	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/extension"
	"github.com/vektah/gqlparser/v2/gqlerror"

	"github.com/dotwaffle/peeringdb-plus/graph"
	"github.com/oyyblin/gqlgen-depth-limit-extension/depth"
)

// NewHandler creates the gqlgen GraphQL handler with complexity and depth limits.
// Introspection is always enabled per D-20.
func NewHandler(resolver *graph.Resolver) http.Handler {
	srv := handler.NewDefaultServer(
		graph.NewExecutableSchema(graph.Config{
			Resolvers: resolver,
		}),
	)

	// Query complexity limit per D-04 (500).
	srv.Use(extension.FixedComplexityLimit(500))

	// Query depth limit per D-04 (15).
	srv.Use(depth.FixedDepthLimit(15))

	// Introspection is enabled by default in gqlgen per D-20.

	// Configure error presenter per D-16 for detailed query errors with
	// field paths and validation details. Ensures every GraphQL error includes
	// the field path, a descriptive message, and a machine-readable code extension.
	srv.SetErrorPresenter(func(ctx context.Context, err error) *gqlerror.Error {
		gqlErr := graphql.DefaultErrorPresenter(ctx, err)
		// Ensure path is populated from context if not already set.
		if gqlErr.Path == nil {
			gqlErr.Path = graphql.GetPath(ctx)
		}
		// Add extensions with error classification for client consumption.
		if gqlErr.Extensions == nil {
			gqlErr.Extensions = make(map[string]any)
		}
		gqlErr.Extensions["code"] = classifyError(err)
		return gqlErr
	})

	return srv
}

// classifyError returns a machine-readable error code based on the error type.
func classifyError(err error) string {
	if err == nil {
		return "INTERNAL_ERROR"
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "not found"):
		return "NOT_FOUND"
	case strings.Contains(msg, "validation"):
		return "VALIDATION_ERROR"
	case strings.Contains(msg, "limit must"), strings.Contains(msg, "offset must"):
		return "BAD_REQUEST"
	default:
		return "INTERNAL_ERROR"
	}
}

// defaultQuery is the pre-built example query displayed when the playground opens per D-19.
// Includes commented examples for ASN lookup, IX listing, facility search, and relationship traversal.
const defaultQuery = `# === PeeringDB Plus Example Queries ===
# Uncomment one to try, or run the default syncStatus query below.
#
# --- ASN Lookup ---
# { networkByAsn(asn: 13335) { name asn infoType website } }
#
# --- IX Network Listing ---
# { internetExchanges(first: 5) { edges { node { name city country } } } }
#
# --- Facility Search ---
# { facilitiesList(limit: 5, where: {country: "US"}) { name city state } }
#
# --- Relationship Traversal ---
# { networks(first: 1) { edges { node { name asn organization { name } } } } }

{
  syncStatus {
    status
    lastSyncAt
  }
}
`

// playgroundTmpl is a custom GraphiQL HTML template that includes a defaultQuery prop per D-19.
var playgroundTmpl = template.Must(template.New("playground").Parse(`<!DOCTYPE html>
<html>
  <head>
    <meta charset="utf-8">
    <title>PeeringDB Plus - GraphQL</title>
    <style>
      body { height: 100%; margin: 0; width: 100%; overflow: hidden; }
      #graphiql { height: 100vh; }
    </style>
    <script
      src="https://cdn.jsdelivr.net/npm/react@18.2.0/umd/react.production.min.js"
      integrity="sha256-S0lp+k7zWUMk2ixteM6HZvu8L9Eh//OVrt+ZfbCpmgY="
      crossorigin="anonymous"
    ></script>
    <script
      src="https://cdn.jsdelivr.net/npm/react-dom@18.2.0/umd/react-dom.production.min.js"
      integrity="sha256-IXWO0ITNDjfnNXIu5POVfqlgYoop36bDzhodR6LW5Pc="
      crossorigin="anonymous"
    ></script>
    <link
      rel="stylesheet"
      href="https://cdn.jsdelivr.net/npm/graphiql@3.7.0/graphiql.min.css"
      integrity="sha256-Dbkv2LUWis+0H4Z+IzxLBxM2ka1J133lSjqqtSu49o8="
      crossorigin="anonymous"
    />
  </head>
  <body>
    <div id="graphiql">Loading...</div>
    <script
      src="https://cdn.jsdelivr.net/npm/graphiql@3.7.0/graphiql.min.js"
      integrity="sha256-qsScAZytFdTAEOM8REpljROHu8DvdvxXBK7xhoq5XD0="
      crossorigin="anonymous"
    ></script>
    <script>
      const url = location.protocol + '//' + location.host + {{.Endpoint}};
      const fetcher = GraphiQL.createFetcher({ url });
      ReactDOM.render(
        React.createElement(GraphiQL, {
          fetcher: fetcher,
          defaultQuery: {{.DefaultQuery}},
          isHeadersEditorEnabled: true,
          shouldPersistHeaders: true,
        }),
        document.getElementById('graphiql'),
      );
    </script>
  </body>
</html>
`))

// playgroundData holds template data for the custom playground HTML.
type playgroundData struct {
	Endpoint     string
	DefaultQuery string
}

// PlaygroundHandler returns a GraphiQL playground handler with pre-built example queries per D-17, D-19, D-21.
// Serves at the provided endpoint path.
func PlaygroundHandler(endpoint string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		data := playgroundData{
			Endpoint:     endpoint,
			DefaultQuery: defaultQuery,
		}
		if err := playgroundTmpl.Execute(w, data); err != nil {
			http.Error(w, "failed to render playground", http.StatusInternalServerError)
		}
	}
}
