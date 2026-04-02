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
// Uses klauspost/compress/gzhttp which handles Content-Encoding headers,
// ETag suffixing (appends --gzip), and minimum size thresholds automatically.
func Compression() func(http.Handler) http.Handler {
	wrapper, err := gzhttp.NewWrapper(
		gzhttp.ExceptContentTypes([]string{
			"application/grpc",
			"application/grpc+proto",
			"application/connect+proto",
		}),
	)
	if err != nil {
		panic(fmt.Sprintf("compression middleware: %v", err))
	}

	return func(next http.Handler) http.Handler {
		return wrapper(next)
	}
}
