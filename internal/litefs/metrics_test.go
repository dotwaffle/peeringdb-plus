package litefs_test

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dotwaffle/peeringdb-plus/internal/litefs"
)

// TestParseMetrics_ProductionSamples parses the output of production
// nodes (LiteFS 0.5, 2026-09-24). The fresh samples come from nodes that
// had not committed since a restart, so they have no
// litefs_db_commit_count series.
func TestParseMetrics_ProductionSamples(t *testing.T) {
	t.Parallel()
	tests := []struct {
		file string
		want litefs.Metrics
	}{
		{
			file: "testdata/metrics-primary.txt",
			want: litefs.Metrics{
				TXID:          58649,
				Commits:       12,
				LTXBytes:      new(int64(203607)),
				LTXFiles:      new(int64(6)),
				LTXLagSeconds: new(float64(0)),
				LagSeconds:    0,
				Subscribers:   7,
			},
		},
		{
			file: "testdata/metrics-primary-fresh.txt",
			want: litefs.Metrics{
				TXID:          58661,
				Commits:       0,
				LTXBytes:      new(int64(886)),
				LTXFiles:      new(int64(1)),
				LTXLagSeconds: new(385.649),
				LagSeconds:    0,
				Subscribers:   7,
			},
		},
		{
			file: "testdata/metrics-replica-fresh.txt",
			want: litefs.Metrics{
				TXID:          58661,
				Commits:       0,
				LTXBytes:      new(int64(54882450)),
				LTXFiles:      new(int64(1)),
				LTXLagSeconds: new(18.921),
				LagSeconds:    0.575,
				Subscribers:   0,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			t.Parallel()
			f, err := os.Open(tt.file)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = f.Close() })

			got, err := litefs.ParseMetrics(f, "peeringdb-plus.db")
			if err != nil {
				t.Fatalf("ParseMetrics: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ParseMetrics = %s, want %s", describe(got), describe(tt.want))
			}
		})
	}
}

// describe formats m with the values of its pointer fields.
func describe(m litefs.Metrics) string {
	opt := func(p any) string {
		switch v := p.(type) {
		case *int64:
			if v != nil {
				return fmt.Sprint(*v)
			}
		case *float64:
			if v != nil {
				return fmt.Sprint(*v)
			}
		}
		return "nil"
	}
	return fmt.Sprintf("{TXID:%d Commits:%d LTXBytes:%s LTXFiles:%s LTXLagSeconds:%s LagSeconds:%v Subscribers:%d}",
		m.TXID, m.Commits, opt(m.LTXBytes), opt(m.LTXFiles), opt(m.LTXLagSeconds), m.LagSeconds, m.Subscribers)
}

// baseExposition holds one sample of each metric that ParseMetrics
// reads, for the database "a.db".
const baseExposition = `litefs_db_txid{db="a.db"} 12
litefs_db_commit_count{db="a.db"} 3
litefs_db_ltx_bytes{db="a.db"} 100
litefs_db_ltx_count{db="a.db"} 2
litefs_db_lag_seconds{db="a.db"} 0.25
litefs_lag_seconds 1.5
litefs_subscriber_count 0
`

var baseMetrics = litefs.Metrics{
	TXID:          12,
	Commits:       3,
	LTXBytes:      new(int64(100)),
	LTXFiles:      new(int64(2)),
	LTXLagSeconds: new(0.25),
	LagSeconds:    1.5,
}

func TestParseMetrics(t *testing.T) {
	t.Parallel()
	replace := func(from, to string) string {
		if !strings.Contains(baseExposition, from) {
			t.Fatalf("baseExposition has no %q", from)
		}
		return strings.Replace(baseExposition, from, to, 1)
	}
	withBytes := baseMetrics
	withBytes.LTXBytes = new(int64(1234500))
	lazyAbsent := litefs.Metrics{TXID: 12, LagSeconds: 1.5}

	tests := []struct {
		name    string
		input   string
		want    litefs.Metrics
		wantErr string
	}{
		{name: "all samples", input: baseExposition, want: baseMetrics},
		{
			name: "other database ignored",
			input: `litefs_db_txid{db="other.db"} 99
litefs_db_ltx_bytes{db="other.db"} 1000
` + baseExposition,
			want: baseMetrics,
		},
		{
			name:  "escaped label value, extra labels, timestamp",
			input: replace(`litefs_db_txid{db="a.db"} 12`, `litefs_db_txid{x="q\"}\\",db="a.db",} 12 1727180000000`),
			want:  baseMetrics,
		},
		{
			name:  "float exposition of a count",
			input: replace(`litefs_db_ltx_bytes{db="a.db"} 100`, `litefs_db_ltx_bytes{db="a.db"} 1.2345e+06`),
			want:  withBytes,
		},
		{
			name: "malformed lines skipped",
			input: `litefs_db_txid{db="a.db" 1
litefs_db_txid{db="a.db"}
litefs_db_commit_count{db="a.db"} NaNx
{db="a.db"} 3
` + baseExposition,
			want: baseMetrics,
		},
		{
			name: "lazy series absent",
			input: `litefs_db_txid{db="a.db"} 12
litefs_lag_seconds 1.5
litefs_subscriber_count 0
`,
			want: lazyAbsent,
		},
		{
			name:    "database missing",
			input:   strings.ReplaceAll(baseExposition, `db="a.db"`, `db="other.db"`),
			wantErr: `database "a.db": no litefs_db_txid sample`,
		},
		{
			name:    "txid missing",
			input:   replace(`litefs_db_txid{db="a.db"} 12`+"\n", ""),
			wantErr: "no litefs_db_txid sample",
		},
		{
			name:    "sample missing",
			input:   replace("litefs_subscriber_count 0\n", ""),
			wantErr: "no litefs_subscriber_count sample",
		},
		{
			name:    "NaN count",
			input:   replace(`litefs_db_txid{db="a.db"} 12`, `litefs_db_txid{db="a.db"} NaN`),
			wantErr: "litefs_db_txid: value NaN is not finite",
		},
		{
			name:    "infinite lag",
			input:   replace("litefs_lag_seconds 1.5", "litefs_lag_seconds +Inf"),
			wantErr: "litefs_lag_seconds: value +Inf is not finite",
		},
		{
			name:    "fractional count",
			input:   replace(`litefs_db_commit_count{db="a.db"} 3`, `litefs_db_commit_count{db="a.db"} 3.5`),
			wantErr: "litefs_db_commit_count: value 3.5 is not an int64",
		},
		{
			name:    "count out of range",
			input:   replace(`litefs_db_ltx_count{db="a.db"} 2`, `litefs_db_ltx_count{db="a.db"} 1e19`),
			wantErr: "litefs_db_ltx_count: value 1e+19 is not an int64",
		},
		{
			name:    "not an exposition",
			input:   "<html>not found</html>\n",
			wantErr: "no litefs_db_txid sample",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := litefs.ParseMetrics(strings.NewReader(tt.input), "a.db")
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("ParseMetrics error = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseMetrics: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ParseMetrics = %s, want %s", describe(got), describe(tt.want))
			}
		})
	}
}

// TestMetricsScraper_Scrape checks the HTTP path and the state-change
// logging: WARN on the first failure, DEBUG on later failures, INFO on
// recovery.
func TestMetricsScraper_Scrape(t *testing.T) {
	t.Parallel()
	var status atomicStatus
	status.set(http.StatusOK)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		code := status.get()
		w.WriteHeader(code)
		if code == http.StatusOK {
			_, _ = w.Write([]byte(baseExposition))
		}
	}))
	t.Cleanup(srv.Close)

	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	s := litefs.NewMetricsScraper(srv.URL, "a.db", logger)
	ctx := context.Background()

	m, err := s.Scrape(ctx)
	if err != nil || !reflect.DeepEqual(m, baseMetrics) {
		t.Fatalf("Scrape = %s, %v; want %s", describe(m), err, describe(baseMetrics))
	}
	if logs.Len() != 0 {
		t.Errorf("successful scrape logged %q", logs.String())
	}

	status.set(http.StatusServiceUnavailable)
	for range 2 {
		if _, err := s.Scrape(ctx); err == nil || !strings.Contains(err.Error(), "503") {
			t.Fatalf("Scrape error = %v, want status 503", err)
		}
	}
	status.set(http.StatusOK)
	if _, err := s.Scrape(ctx); err != nil {
		t.Fatalf("Scrape after recovery: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(logs.String()), "\n")
	wantLevels := []string{"level=WARN", "level=DEBUG", "level=INFO"}
	if len(lines) != len(wantLevels) {
		t.Fatalf("got %d log lines, want %d:\n%s", len(lines), len(wantLevels), logs.String())
	}
	for i, want := range wantLevels {
		if !strings.Contains(lines[i], want) {
			t.Errorf("log line %d = %q, want %s", i, lines[i], want)
		}
	}
}

// TestMetricsScraper_ScrapeBodyTooLarge checks that a body over the
// 1 MiB limit fails the scrape. Without the check, the parser reads a
// truncated exposition and reports zero for the samples after the cut.
func TestMetricsScraper_ScrapeBodyTooLarge(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`litefs_db_txid{db="a.db"} 12` + "\n"))
		_, _ = w.Write([]byte(strings.Repeat("# padding\n", 120_000)))
		_, _ = w.Write([]byte(baseExposition))
	}))
	t.Cleanup(srv.Close)

	s := litefs.NewMetricsScraper(srv.URL, "a.db", slog.New(slog.DiscardHandler))
	if _, err := s.Scrape(context.Background()); err == nil || !strings.Contains(err.Error(), "larger than") {
		t.Fatalf("Scrape error = %v, want a body size error", err)
	}
}

func TestMetricsScraper_ScrapeTimeout(t *testing.T) {
	t.Parallel()
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(func() {
		close(release)
		srv.Close()
	})

	s := litefs.NewMetricsScraper(srv.URL, "a.db", slog.New(slog.DiscardHandler))
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err := s.Scrape(ctx); err == nil {
		t.Fatal("Scrape returned no error for a server that never answers")
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("Scrape took %s, want it to stop at the context deadline", elapsed)
	}
}

// atomicStatus is an HTTP status code shared with a test server.
type atomicStatus struct{ v atomic.Int64 }

func (a *atomicStatus) set(code int) { a.v.Store(int64(code)) }
func (a *atomicStatus) get() int     { return int(a.v.Load()) }
