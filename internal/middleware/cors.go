// Package middleware provides HTTP middleware for the PeeringDB Plus server.
package middleware

import (
	"net/http"
	"strings"

	connectcors "connectrpc.com/cors"
	"github.com/rs/cors"
)

// CORSInput holds configuration for the CORS middleware.
type CORSInput struct {
	// AllowedOrigins is a comma-separated list of allowed origins. Use "*" for all origins.
	AllowedOrigins string
}

// CORS returns middleware that adds CORS headers.
// Origins are configured via the AllowedOrigins field.
func CORS(in CORSInput) func(http.Handler) http.Handler {
	origins := strings.Split(in.AllowedOrigins, ",")
	for i := range origins {
		origins[i] = strings.TrimSpace(origins[i])
	}
	// Merge application headers with Connect/gRPC/gRPC-Web and MCP Streamable
	// HTTP protocol headers. Last-Event-ID is used when an MCP client resumes
	// an event stream; the current MCP server is stateless but allowing it keeps
	// the transport interoperable if that policy changes later.
	allowedHeaders := append(
		[]string{
			"Content-Type",
			"Authorization",
			"MCP-Protocol-Version",
			"MCP-Session-Id",
			"Last-Event-ID",
		},
		connectcors.AllowedHeaders()...,
	)
	allowedMethods := append([]string{"GET", "DELETE", "OPTIONS"}, connectcors.AllowedMethods()...)
	exposedHeaders := append([]string{"MCP-Session-Id"}, connectcors.ExposedHeaders()...)

	c := cors.New(cors.Options{
		AllowedOrigins:   origins,
		AllowedMethods:   allowedMethods,
		AllowedHeaders:   allowedHeaders,
		ExposedHeaders:   exposedHeaders,
		AllowCredentials: false,
		MaxAge:           86400,
	})
	return c.Handler
}
