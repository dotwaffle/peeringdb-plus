package pdbcompat

import (
	"container/list"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"golang.org/x/sync/singleflight"

	pdbotel "github.com/dotwaffle/peeringdb-plus/internal/otel"
)

// The mirror stores the logo URL of an object, not the file. Upstream
// /api/asset sends the file as base64 (2.83.0 serializers.py:5268-5294),
// so the mirror reads the file from the upstream media bucket when a
// caller asks for it, and keeps it in memory.

// LogoMediaHost is the host of the upstream media bucket, which every
// stored logo URL names. The fetcher reads no other host, so a stored
// URL cannot make the mirror send requests to another address.
const LogoMediaHost = "peeringdb-media-prod.s3.amazonaws.com"

const (
	// logoFetchTimeout bounds one read of a logo file.
	logoFetchTimeout = 10 * time.Second
	// logoMaxBytes is the largest file that the fetcher accepts.
	// Upstream accepts no logo above 50 KiB (settings/__init__.py:1728).
	logoMaxBytes = 1 << 20
	// logoCacheBytes is the size of the logo cache of one process.
	logoCacheBytes = 16 << 20
	// logoMaxInFlight is the number of reads that can run at once.
	logoMaxInFlight = 4
	// logoFailureBackoff is the time after a failed read of a URL during
	// which the fetcher does not read the URL again.
	logoFailureBackoff = 5 * time.Minute
)

// errLogoBackoff is the error of a URL whose last read failed less than
// logoFailureBackoff ago.
var errLogoBackoff = errors.New("logo read failed recently")

// logoFetcher reads logo files from the media host and keeps them in a
// least-recently-used cache of logoCacheBytes. A logo URL has a random
// part that changes with each upload (serializers.py:5189-5198), so the
// file at a URL does not change and a cached file never goes stale.
// Concurrent reads of one URL share one request.
type logoFetcher struct {
	client *http.Client
	host   string
	sem    chan struct{}
	group  singleflight.Group
	now    func() time.Time

	mu      sync.Mutex
	entries map[string]*list.Element // value: *logoEntry
	order   *list.List               // front: most recently used
	size    int
	failed  map[string]time.Time
}

// logoEntry is one cached file.
type logoEntry struct {
	url  string
	data []byte
}

// newLogoFetcher returns a fetcher that reads files from host with a
// copy of client that follows no redirect: a redirect to another host
// would go around the host check.
func newLogoFetcher(client *http.Client, host string) *logoFetcher {
	c := *client
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &logoFetcher{
		client:  &c,
		host:    host,
		sem:     make(chan struct{}, logoMaxInFlight),
		now:     time.Now,
		entries: make(map[string]*list.Element),
		order:   list.New(),
		failed:  make(map[string]time.Time),
	}
}

// SetLogoSource makes the asset route read logo files with client from
// the https URLs of host (LogoMediaHost in production). Without a
// source, the route sends file_data null.
func (h *Handler) SetLogoSource(client *http.Client, host string) {
	h.logos = newLogoFetcher(client, host)
}

// get returns the file at rawURL. ok is false when the URL is not an
// https URL of the media host, or when the read fails. Upstream sends
// file_data null when it cannot read the file (serializers.py:5277-5283),
// so the caller does the same.
func (f *logoFetcher) get(ctx context.Context, rawURL string) (data []byte, ok bool) {
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme != "https" || u.Host != f.host {
		recordLogoFetch(ctx, "skipped")
		return nil, false
	}
	if data, ok := f.cached(rawURL); ok {
		recordLogoFetch(ctx, "hit")
		return data, true
	}
	v, err, _ := f.group.Do(rawURL, func() (any, error) {
		return f.fetch(ctx, rawURL)
	})
	if err != nil {
		recordLogoFetch(ctx, "error")
		return nil, false
	}
	recordLogoFetch(ctx, "miss")
	return v.([]byte), true
}

// cached returns the cached file at rawURL and marks it as used.
func (f *logoFetcher) cached(rawURL string) ([]byte, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	el, ok := f.entries[rawURL]
	if !ok {
		return nil, false
	}
	f.order.MoveToFront(el)
	return el.Value.(*logoEntry).data, true
}

// fetch reads the file at rawURL and caches it. The read does not stop
// when the context of the first caller ends: other callers can wait for
// the same read.
func (f *logoFetcher) fetch(ctx context.Context, rawURL string) ([]byte, error) {
	f.mu.Lock()
	failedAt, failed := f.failed[rawURL]
	f.mu.Unlock()
	if failed && f.now().Sub(failedAt) < logoFailureBackoff {
		return nil, errLogoBackoff
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), logoFetchTimeout)
	defer cancel()
	select {
	case f.sem <- struct{}{}:
		defer func() { <-f.sem }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	data, err := f.read(ctx, rawURL)
	f.mu.Lock()
	defer f.mu.Unlock()
	if err != nil {
		f.failed[rawURL] = f.now()
		f.pruneFailed()
		return nil, err
	}
	delete(f.failed, rawURL)
	f.store(rawURL, data)
	return data, nil
}

// read sends the request for rawURL and returns the body of a 200
// response.
func (f *logoFetcher) read(ctx context.Context, rawURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := f.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("logo read: status %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, logoMaxBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > logoMaxBytes {
		return nil, fmt.Errorf("logo read: file larger than %d bytes", logoMaxBytes)
	}
	return data, nil
}

// store adds a file to the cache and removes the least recently used
// files until the cache fits in logoCacheBytes. f.mu must be held.
func (f *logoFetcher) store(rawURL string, data []byte) {
	if _, ok := f.entries[rawURL]; ok || len(data) > logoCacheBytes {
		return
	}
	f.entries[rawURL] = f.order.PushFront(&logoEntry{url: rawURL, data: data})
	f.size += len(data)
	for f.size > logoCacheBytes {
		el := f.order.Back()
		e := el.Value.(*logoEntry)
		f.order.Remove(el)
		delete(f.entries, e.url)
		f.size -= len(e.data)
	}
}

// pruneFailed removes the failures older than logoFailureBackoff when
// the failure map is large. f.mu must be held.
func (f *logoFetcher) pruneFailed() {
	if len(f.failed) < 1000 {
		return
	}
	now := f.now()
	for u, at := range f.failed {
		if now.Sub(at) >= logoFailureBackoff {
			delete(f.failed, u)
		}
	}
}

// recordLogoFetch counts one logo read with its result.
func recordLogoFetch(ctx context.Context, result string) {
	pdbotel.AssetLogoFetches.Add(ctx, 1, metric.WithAttributes(attribute.String("result", result)))
}
