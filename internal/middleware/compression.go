package middleware

import (
	"fmt"
	"net/http"

	"github.com/klauspost/compress/gzhttp"
)

// Compression returns middleware that gzip-compresses HTTP responses when the
// client advertises Accept-Encoding: gzip. gRPC content types are excluded
// because ConnectRPC manages its own compression.
//
// Uses klauspost/compress/gzhttp which handles Content-Encoding headers
// and minimum size thresholds automatically. ETag suffixing
// (gzhttp.SuffixETag) is deliberately NOT configured: our ETags are weak
// (W/ prefix — semantic, not byte-for-byte, equivalence), so the same
// tag is correct for both the plain and gzipped representation and a
// --gzip suffix would only fragment client caches.
func Compression() func(http.Handler) http.Handler {
	wrapper, err := gzhttp.NewWrapper(
		gzhttp.ExceptContentTypes([]string{
			"application/grpc",
			"application/grpc+proto",
			"application/connect+proto",
			"text/event-stream",
		}),
	)
	if err != nil {
		panic(fmt.Sprintf("compression middleware: %v", err))
	}

	return func(next http.Handler) http.Handler {
		return wrapper(next)
	}
}
