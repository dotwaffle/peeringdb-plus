package middleware

import (
	"net/http"
	"strings"
)

// maxBytesSkipPrefixes are URL path prefixes that bypass the MaxBytesBody
// middleware. ConnectRPC and gRPC paths must not be body-capped because
// streaming RPCs produce effectively unbounded bodies; wrapping them would
// break the protocol. Per-route body limits at cmd/peeringdb-plus/main.go
// remain as belt-and-suspenders for non-gRPC POST handlers.
var maxBytesSkipPrefixes = []string{
	"/peeringdb.v1.",   // ConnectRPC service routes (13 services)
	"/grpc.",           // general gRPC catch-all (future-proofing)
	"/grpc.health.v1.", // gRPC health check (subset of /grpc. — explicit for readability)
}

// MaxBytesBodyInput configures the MaxBytesBody middleware.
type MaxBytesBodyInput struct {
	// MaxBytes is the per-request body size cap in bytes. When a request
	// body exceeds this limit, the next Read call returns *http.MaxBytesError
	// and the ResponseWriter writes 413 Request Entity Too Large.
	MaxBytes int64
}

// MaxBytesBody returns middleware that wraps r.Body with http.MaxBytesReader
// for every non-gRPC request. gRPC and ConnectRPC paths bypass entirely so
// streaming RPCs are not truncated.
//
// Per-route wraps at cmd/peeringdb-plus/main.go (/sync, /graphql) remain as
// tighter belt-and-suspenders — innermost wins, which is harmless here because
// the global cap and the per-route caps share the same maxRequestBodySize
// constant (1 MB).
func MaxBytesBody(in MaxBytesBodyInput) func(http.Handler) http.Handler {
	maxBytes := in.MaxBytes
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			for _, prefix := range maxBytesSkipPrefixes {
				if strings.HasPrefix(r.URL.Path, prefix) {
					next.ServeHTTP(w, r)
					return
				}
			}
			r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			next.ServeHTTP(w, r)
		})
	}
}
