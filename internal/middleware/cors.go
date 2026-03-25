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

// CORS returns middleware that adds CORS headers per OPS-06.
// Origins are configured via the AllowedOrigins field.
func CORS(in CORSInput) func(http.Handler) http.Handler {
	origins := strings.Split(in.AllowedOrigins, ",")
	for i := range origins {
		origins[i] = strings.TrimSpace(origins[i])
	}
	// Merge application headers with Connect/gRPC/gRPC-Web protocol headers.
	allowedHeaders := append([]string{"Content-Type", "Authorization"}, connectcors.AllowedHeaders()...)
	allowedMethods := append([]string{"GET", "OPTIONS"}, connectcors.AllowedMethods()...)
	exposedHeaders := connectcors.ExposedHeaders()

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
