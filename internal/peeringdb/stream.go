package peeringdb

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// StreamAll fetches objects of the given type and invokes handler for each
// element of the "data" array as a json.RawMessage. Both {"meta":{...},"data":[...]}
// and {"data":[...],"meta":{...}} key orderings are supported — the decoder
// token-walks the outer object and dispatches based on key name.
//
// The handler is invoked synchronously from within the HTTP response body
// read loop; the HTTP response is held open for the duration of the stream.
// The raw message passed to the handler is only valid until the handler
// returns — if the caller needs to retain it, it must copy (bytes.Clone or
// json.Unmarshal).
//
// If the handler returns an error, StreamAll aborts, closes the response
// body, and returns the error wrapped as "handler %s element %d: %w".
//
// For callers that need the full []json.RawMessage slice (e.g. the
// pdbcompat-check CLI), FetchAll wraps StreamAll with a simple append-all
// handler. Do NOT rewrite FetchAll's callers to decode into the tx —
// PERF-05 option (b) in internal/sync/worker.go requires the batch
// materialised before the tx opens.
func (c *Client) StreamAll(ctx context.Context, objectType string, handler func(raw json.RawMessage) error, opts ...FetchOption) (FetchMeta, error) {
	var cfg fetchConfig
	for _, opt := range opts {
		opt(&cfg)
	}

	tracer := otel.Tracer("peeringdb")
	ctx, span := tracer.Start(ctx, "peeringdb.stream/"+objectType)
	defer span.End()

	// Full sync path: single unpaginated request.
	if cfg.since.IsZero() {
		url := fmt.Sprintf("%s/api/%s?depth=0", c.baseURL, objectType)
		resp, err := c.doWithRetry(ctx, url)
		if err != nil {
			span.RecordError(err)
			return FetchMeta{}, fmt.Errorf("fetch %s: %w", objectType, err)
		}
		meta, count, decErr := streamDecodeResponse(resp.Body, objectType, handler)
		closeErr := resp.Body.Close()
		if decErr != nil {
			span.RecordError(decErr)
			return FetchMeta{}, decErr
		}
		if closeErr != nil {
			c.logger.LogAttrs(ctx, slog.LevelWarn, "stream body close",
				slog.String("type", objectType),
				slog.Any("error", closeErr),
			)
		}
		span.AddEvent("streamed", trace.WithAttributes(attribute.Int("count", count)))
		c.logger.LogAttrs(ctx, slog.LevelDebug, "streamed all",
			slog.String("type", objectType),
			slog.Int("count", count),
		)
		return meta, nil
	}

	// Incremental path: paginate through delta results.
	var combined FetchMeta
	totalCount := 0
	for skip := 0; ; skip += pageSize {
		page := skip / pageSize
		url := fmt.Sprintf("%s/api/%s?limit=%d&skip=%d&depth=0&since=%d",
			c.baseURL, objectType, pageSize, skip, cfg.since.Unix())
		resp, err := c.doWithRetry(ctx, url)
		if err != nil {
			span.RecordError(err)
			return FetchMeta{}, fmt.Errorf("fetch %s page %d: %w", objectType, page, err)
		}

		pageMeta, pageCount, decErr := streamDecodeResponse(resp.Body, objectType, handler)
		_ = resp.Body.Close()
		if decErr != nil {
			span.RecordError(decErr)
			return FetchMeta{}, decErr
		}
		if pageCount == 0 {
			break
		}
		totalCount += pageCount
		// Track earliest generated timestamp across pages, matching the
		// legacy FetchAll aggregation rule.
		if !pageMeta.Generated.IsZero() {
			if combined.Generated.IsZero() || pageMeta.Generated.Before(combined.Generated) {
				combined.Generated = pageMeta.Generated
			}
		}
	}
	span.AddEvent("streamed", trace.WithAttributes(attribute.Int("count", totalCount)))
	return combined, nil
}

// streamDecodeResponse walks a PeeringDB response body with json.Decoder,
// invoking handler for each element of the "data" array. Both key orderings
// ({meta,data} and {data,meta}) are supported via a token-walk state machine.
// Returns the parsed FetchMeta, the number of elements passed to handler,
// and any error encountered.
func streamDecodeResponse(body io.Reader, objectType string, handler func(raw json.RawMessage) error) (FetchMeta, int, error) {
	dec := json.NewDecoder(body)

	// Consume opening '{' of the outer object.
	tok, err := dec.Token()
	if err != nil {
		return FetchMeta{}, 0, fmt.Errorf("decode %s: %w", objectType, err)
	}
	if delim, ok := tok.(json.Delim); !ok || delim != '{' {
		return FetchMeta{}, 0, fmt.Errorf("decode %s: expected object, got %v", objectType, tok)
	}

	var meta FetchMeta
	count := 0

	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return FetchMeta{}, count, fmt.Errorf("decode %s key: %w", objectType, err)
		}
		key, ok := keyTok.(string)
		if !ok {
			return FetchMeta{}, count, fmt.Errorf("decode %s: expected string key, got %v", objectType, keyTok)
		}

		switch key {
		case "data":
			n, err := streamDataArray(dec, objectType, handler)
			if err != nil {
				return FetchMeta{}, count, err
			}
			count += n
		case "meta":
			var m struct {
				Generated float64 `json:"generated"`
			}
			if err := dec.Decode(&m); err != nil {
				return FetchMeta{}, count, fmt.Errorf("decode %s meta: %w", objectType, err)
			}
			if m.Generated != 0 {
				meta.Generated = time.Unix(int64(m.Generated), 0)
			}
		default:
			// Unknown top-level key — skip its value to stay in sync with the
			// token stream. Skipping via json.RawMessage consumes exactly one
			// JSON value regardless of whether it is a scalar, array, or object.
			var skip json.RawMessage
			if err := dec.Decode(&skip); err != nil {
				return FetchMeta{}, count, fmt.Errorf("decode %s skip %s: %w", objectType, key, err)
			}
		}
	}

	// Consume closing '}' of the outer object. EOF after the close is fine.
	if _, err := dec.Token(); err != nil && err != io.EOF {
		return FetchMeta{}, count, fmt.Errorf("decode %s close: %w", objectType, err)
	}
	return meta, count, nil
}

// streamDataArray walks a "data": [ ... ] array, invoking handler for each
// element. Assumes the decoder is positioned at the value following the
// "data" key (i.e. about to emit the '[' delim token).
func streamDataArray(dec *json.Decoder, objectType string, handler func(raw json.RawMessage) error) (int, error) {
	// Consume opening '['.
	tok, err := dec.Token()
	if err != nil {
		return 0, fmt.Errorf("decode %s data: %w", objectType, err)
	}
	if delim, ok := tok.(json.Delim); !ok || delim != '[' {
		return 0, fmt.Errorf("decode %s data: expected array, got %v", objectType, tok)
	}

	count := 0
	for dec.More() {
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return count, fmt.Errorf("decode %s element %d: %w", objectType, count, err)
		}
		if err := handler(raw); err != nil {
			return count, fmt.Errorf("handler %s element %d: %w", objectType, count, err)
		}
		count++
	}

	// Consume closing ']'.
	if _, err := dec.Token(); err != nil {
		return count, fmt.Errorf("decode %s data close: %w", objectType, err)
	}
	return count, nil
}
