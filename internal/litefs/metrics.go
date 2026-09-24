package litefs

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"math"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

// metricsTimeout bounds one scrape of the LiteFS metrics endpoint. The
// endpoint is on the same machine, so a slow answer means LiteFS is in
// trouble. The scrape runs inside the OTel collection callback, which
// must not stall the export.
const metricsTimeout = 2 * time.Second

// maxMetricsBody caps the bytes read from the metrics endpoint. The
// LiteFS exposition is about 8 KiB. A larger body fails the scrape.
const maxMetricsBody = 1 << 20

// Metrics holds the LiteFS values that the app exports, from one scrape
// of the LiteFS Prometheus endpoint (http.addr, :20202 by default). The
// db-labelled values are for one database.
//
// LiteFS creates some db-labelled series only when it first sets them:
// at the first commit, LTX apply or retention pass after it starts. A
// pointer field is nil while its series is absent.
type Metrics struct {
	// TXID is the current transaction ID of the database
	// (litefs_db_txid).
	TXID int64
	// Commits is the number of database commits on this node since
	// LiteFS started (litefs_db_commit_count). LiteFS creates the series
	// at the first commit, so it is 0 until then. A replica applies LTX
	// files and does not commit, so the value does not grow while the
	// node is a replica. A node that LiteFS demoted keeps its count.
	Commits int64
	// LTXBytes is litefs_db_ltx_bytes. Each retention pass (once a
	// minute by default) sets it to the size of the LTX files on disk.
	// A commit sets it to the size of the new LTX file, until the next
	// retention pass. LiteFS keeps each LTX file for its retention
	// period, so a large commit shows in both values.
	LTXBytes *int64
	// LTXFiles is the number of LTX files on disk (litefs_db_ltx_count).
	LTXFiles *int64
	// LTXLagSeconds is the time from the creation of the last LTX file
	// that this node applied to its apply (litefs_db_lag_seconds). A
	// commit on the primary sets it to 0.
	LTXLagSeconds *float64
	// LagSeconds is the time since this node last received a frame
	// (LTX file or heartbeat) from the primary (litefs_lag_seconds). It
	// is 0 on the primary.
	LagSeconds float64
	// Subscribers is the number of replicas connected to this node
	// (litefs_subscriber_count).
	Subscribers int64
}

// metricSample tells ParseMetrics how to read one LiteFS metric.
type metricSample struct {
	// dbScoped is true for a metric with a "db" label. ParseMetrics
	// reads only the sample for the requested database.
	dbScoped bool
	// lazy is true for a series that LiteFS creates only when it first
	// sets it, after it starts. A scrape without it is valid.
	lazy bool
	// setInt stores a count and setFloat stores a float. Each entry
	// sets exactly one of them.
	setInt   func(m *Metrics, v int64)
	setFloat func(m *Metrics, v float64)
}

// metricSamples lists the LiteFS metrics that ParseMetrics reads. A
// scrape must hold each one that is not lazy. The lazy ones follow
// LiteFS 0.5 db.go: commit_count starts at the first commit (CommitWAL,
// CommitJournal and Drop, db.go:1757, 2123, 2264; not while a replica),
// ltx_bytes and ltx_count at the first commit or retention pass
// (db.go:1758-1759, 3555-3556), lag_seconds at the first commit or LTX
// apply (db.go:1760, 2607). A restarted primary and a restarted replica
// both have litefs_db_txid before their first commit
// (testdata/metrics-*-fresh.txt), so it stays required: without it, a
// wrong database name would give a scrape with no database values.
var metricSamples = map[string]metricSample{
	"litefs_db_txid":          {dbScoped: true, setInt: func(m *Metrics, v int64) { m.TXID = v }},
	"litefs_db_commit_count":  {dbScoped: true, lazy: true, setInt: func(m *Metrics, v int64) { m.Commits = v }},
	"litefs_db_ltx_bytes":     {dbScoped: true, lazy: true, setInt: func(m *Metrics, v int64) { m.LTXBytes = new(v) }},
	"litefs_db_ltx_count":     {dbScoped: true, lazy: true, setInt: func(m *Metrics, v int64) { m.LTXFiles = new(v) }},
	"litefs_db_lag_seconds":   {dbScoped: true, lazy: true, setFloat: func(m *Metrics, v float64) { m.LTXLagSeconds = new(v) }},
	"litefs_lag_seconds":      {setFloat: func(m *Metrics, v float64) { m.LagSeconds = v }},
	"litefs_subscriber_count": {setInt: func(m *Metrics, v int64) { m.Subscribers = v }},
}

// ParseMetrics reads a Prometheus text exposition from r and returns the
// LiteFS values for the database named db (the base name of the database
// file, the value of the "db" label). It ignores lines it cannot parse
// and metrics it does not use. It returns an error when a metric in
// metricSamples that is not lazy has no sample, or when a sample value
// is not finite or a count is not an int64. A wrong URL, a wrong
// database name or a changed LiteFS exposition then gives an error, not
// zero values.
func ParseMetrics(r io.Reader, db string) (Metrics, error) {
	var m Metrics
	seen := make(map[string]bool, len(metricSamples))
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		name, labels, value, ok := parseSample(line)
		if !ok {
			continue
		}
		spec, ok := metricSamples[name]
		if !ok || (spec.dbScoped && labels["db"] != db) {
			continue
		}
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return Metrics{}, fmt.Errorf("litefs metric %s: value %v is not finite", name, value)
		}
		if spec.setInt != nil {
			if value != math.Trunc(value) || value < math.MinInt64 || value >= 1<<63 {
				return Metrics{}, fmt.Errorf("litefs metric %s: value %v is not an int64", name, value)
			}
			spec.setInt(&m, int64(value))
		} else {
			spec.setFloat(&m, value)
		}
		seen[name] = true
	}
	if err := sc.Err(); err != nil {
		return Metrics{}, fmt.Errorf("read litefs metrics: %w", err)
	}
	for _, name := range slices.Sorted(maps.Keys(metricSamples)) {
		if !seen[name] && !metricSamples[name].lazy {
			return Metrics{}, fmt.Errorf("litefs metrics for database %q: no %s sample", db, name)
		}
	}
	return m, nil
}

// parseSample parses one sample line of the Prometheus text format:
//
//	name value
//	name{label="value",...} value [timestamp]
//
// It reports ok=false for a line that does not have this form.
func parseSample(line string) (name string, labels map[string]string, value float64, ok bool) {
	i := strings.IndexAny(line, "{ \t")
	if i <= 0 {
		return "", nil, 0, false
	}
	name, rest := line[:i], line[i:]
	if rest[0] == '{' {
		var n int
		labels, n, ok = parseLabels(rest)
		if !ok {
			return "", nil, 0, false
		}
		rest = rest[n:]
	}
	fields := strings.Fields(rest)
	if len(fields) == 0 {
		return "", nil, 0, false
	}
	v, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return "", nil, 0, false
	}
	return name, labels, v, true
}

// parseLabels parses a label set that starts at s[0] == '{'. It returns
// the labels and the number of bytes of s that the set uses, closing
// brace included.
func parseLabels(s string) (map[string]string, int, bool) {
	labels := map[string]string{}
	i := 1
	for {
		for i < len(s) && (s[i] == ' ' || s[i] == ',') {
			i++
		}
		if i >= len(s) {
			return nil, 0, false
		}
		if s[i] == '}' {
			return labels, i + 1, true
		}
		eq := strings.IndexByte(s[i:], '=')
		if eq <= 0 || i+eq+1 >= len(s) || s[i+eq+1] != '"' {
			return nil, 0, false
		}
		key := strings.TrimSpace(s[i : i+eq])
		i += eq + 2
		var val strings.Builder
		for {
			if i >= len(s) {
				return nil, 0, false
			}
			c := s[i]
			if c == '"' {
				i++
				break
			}
			if c == '\\' && i+1 < len(s) {
				i++
				switch s[i] {
				case 'n':
					c = '\n'
				default:
					c = s[i]
				}
			}
			val.WriteByte(c)
			i++
		}
		labels[key] = val.String()
	}
}

// MetricsScraper reads the LiteFS metrics endpoint for one database.
type MetricsScraper struct {
	client *http.Client
	url    string
	db     string
	logger *slog.Logger
	// failing is true after a failed scrape, until a scrape succeeds.
	// It limits the WARN log to the first failure of a run of failures.
	failing atomic.Bool
}

// NewMetricsScraper returns a scraper for the LiteFS metrics at endpoint
// (for example http://localhost:20202/metrics) and the database named db
// (the base name of the database file).
func NewMetricsScraper(endpoint, db string, logger *slog.Logger) *MetricsScraper {
	return &MetricsScraper{
		client: &http.Client{Timeout: metricsTimeout},
		url:    endpoint,
		db:     db,
		logger: logger,
	}
}

// Scrape fetches and parses the LiteFS metrics. The first failure after
// a success (or at start) logs a WARN, later failures log at DEBUG, and
// the first success after a failure logs at INFO.
func (s *MetricsScraper) Scrape(ctx context.Context) (Metrics, error) {
	m, err := s.fetch(ctx)
	if err != nil {
		level := slog.LevelDebug
		if !s.failing.Swap(true) {
			level = slog.LevelWarn
		}
		s.logger.LogAttrs(ctx, level, "litefs metrics scrape failed",
			slog.String("url", s.url),
			slog.String("db", s.db),
			slog.Any("error", err),
		)
		return Metrics{}, err
	}
	if s.failing.Swap(false) {
		s.logger.LogAttrs(ctx, slog.LevelInfo, "litefs metrics scrape recovered",
			slog.String("url", s.url),
			slog.String("db", s.db),
		)
	}
	return m, nil
}

func (s *MetricsScraper) fetch(ctx context.Context) (Metrics, error) {
	ctx, cancel := context.WithTimeout(ctx, metricsTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.url, nil)
	if err != nil {
		return Metrics{}, fmt.Errorf("build litefs metrics request: %w", err)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return Metrics{}, fmt.Errorf("get litefs metrics: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return Metrics{}, fmt.Errorf("get litefs metrics: status %s", resp.Status)
	}
	// Read one byte more than the limit, so that a body at the limit
	// is an error and not a truncated exposition.
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxMetricsBody+1))
	if err != nil {
		return Metrics{}, fmt.Errorf("read litefs metrics: %w", err)
	}
	if len(body) > maxMetricsBody {
		return Metrics{}, fmt.Errorf("litefs metrics body is larger than %d bytes", maxMetricsBody)
	}
	return ParseMetrics(bytes.NewReader(body), s.db)
}
