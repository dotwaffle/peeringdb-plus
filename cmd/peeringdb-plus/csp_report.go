package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"golang.org/x/time/rate"

	pdbotel "github.com/dotwaffle/peeringdb-plus/internal/otel"
)

// cspReportPath receives the Content-Security-Policy violation reports
// of the UI and GraphiQL pages.
const cspReportPath = "/csp-report"

const (
	// cspReportMaxBytes caps a report body. A browser report is about
	// 1 KB, and the Reporting API can batch a few reports in one body.
	cspReportMaxBytes = 16 << 10
	// cspReportMaxEntries is the number of reports of one body that the
	// handler reads. The rest are dropped.
	cspReportMaxEntries = 20
	// cspReportFieldMax is the length that logged fields are cut to.
	cspReportFieldMax = 200
)

// cspDirectives are the directive values that the reports counter uses
// as labels. The directive comes from the client, so any other value
// counts as "other".
var cspDirectives = []string{
	"default-src", "script-src", "script-src-elem", "script-src-attr",
	"style-src", "style-src-elem", "style-src-attr", "img-src",
	"connect-src", "font-src", "object-src", "base-uri",
	"frame-ancestors", "form-action", "frame-src", "worker-src",
	"media-src", "manifest-src", "child-src",
}

// cspReportInput holds the dependencies of the CSP report handler.
type cspReportInput struct {
	Logger *slog.Logger
	// LogLimit allows a log line per report while it has tokens. The
	// endpoint takes anonymous input, so the limit caps log volume.
	// Reports over the limit are counted but not logged.
	LogLimit *rate.Limiter
	// Count records one report with its directive label.
	Count func(ctx context.Context, directive string)
}

// newCSPReportLogLimit returns the log limit of the report handler: a
// burst of 5 lines, then one line per 10 seconds.
func newCSPReportLogLimit() *rate.Limiter {
	return rate.NewLimiter(rate.Every(10*time.Second), 5)
}

// countCSPReport adds a report to the pdbplus.csp.reports counter.
func countCSPReport(ctx context.Context, directive string) {
	pdbotel.CSPReports.Add(ctx, 1, metric.WithAttributes(attribute.String("directive", directive)))
}

// cspViolation holds the fields of one report that the handler logs.
type cspViolation struct {
	directive   string
	blocked     string
	document    string
	source      string
	line        int
	disposition string
}

// legacyCSPReport is the body that report-uri sends
// (Content-Type: application/csp-report).
type legacyCSPReport struct {
	Report struct {
		DocumentURI        string `json:"document-uri"`
		ViolatedDirective  string `json:"violated-directive"`
		EffectiveDirective string `json:"effective-directive"`
		BlockedURI         string `json:"blocked-uri"`
		SourceFile         string `json:"source-file"`
		LineNumber         int    `json:"line-number"`
		Disposition        string `json:"disposition"`
	} `json:"csp-report"`
}

// reportingAPIReport is one entry of the body that report-to sends
// (Content-Type: application/reports+json).
type reportingAPIReport struct {
	Type string `json:"type"`
	Body struct {
		DocumentURL        string `json:"documentURL"`
		EffectiveDirective string `json:"effectiveDirective"`
		BlockedURL         string `json:"blockedURL"`
		SourceFile         string `json:"sourceFile"`
		LineNumber         int    `json:"lineNumber"`
		Disposition        string `json:"disposition"`
	} `json:"body"`
}

// newCSPReportHandler returns the handler of cspReportPath. It answers
// 204 to a report it can read, 400 to a body it cannot decode, 413 to a
// body over cspReportMaxBytes and 415 to another media type.
func newCSPReportHandler(in cspReportInput) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mediaType, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
		body := http.MaxBytesReader(w, r.Body, cspReportMaxBytes)
		var violations []cspViolation
		var err error
		switch mediaType {
		case "application/csp-report", "application/json":
			violations, err = decodeLegacyCSPReport(body)
		case "application/reports+json":
			violations, err = decodeReportingAPIReports(body)
		default:
			http.Error(w, "unsupported report type", http.StatusUnsupportedMediaType)
			return
		}
		if err != nil {
			if maxErr, ok := errors.AsType[*http.MaxBytesError](err); ok {
				http.Error(w, maxErr.Error(), http.StatusRequestEntityTooLarge)
				return
			}
			http.Error(w, "invalid report", http.StatusBadRequest)
			return
		}
		for _, v := range violations {
			directive := v.directive
			if !slices.Contains(cspDirectives, directive) {
				directive = "other"
			}
			in.Count(r.Context(), directive)
			if !in.LogLimit.Allow() {
				continue
			}
			in.Logger.LogAttrs(r.Context(), slog.LevelWarn, "csp violation",
				slog.String("directive", directive),
				slog.String("blocked_uri", reportURL(v.blocked)),
				slog.String("document_uri", reportURL(v.document)),
				slog.String("source_file", reportURL(v.source)),
				slog.Int("line", v.line),
				slog.String("disposition", cspField(v.disposition)),
			)
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

func decodeLegacyCSPReport(body io.Reader) ([]cspViolation, error) {
	var report legacyCSPReport
	if err := json.NewDecoder(body).Decode(&report); err != nil {
		return nil, err
	}
	r := report.Report
	directive := r.EffectiveDirective
	if directive == "" {
		// Older browsers send only violated-directive, which holds the
		// directive name and its value.
		directive, _, _ = strings.Cut(r.ViolatedDirective, " ")
	}
	return []cspViolation{{
		directive:   directive,
		blocked:     r.BlockedURI,
		document:    r.DocumentURI,
		source:      r.SourceFile,
		line:        r.LineNumber,
		disposition: r.Disposition,
	}}, nil
}

func decodeReportingAPIReports(body io.Reader) ([]cspViolation, error) {
	var reports []reportingAPIReport
	if err := json.NewDecoder(body).Decode(&reports); err != nil {
		return nil, err
	}
	var out []cspViolation
	for _, report := range reports {
		if report.Type != "csp-violation" {
			continue
		}
		if len(out) == cspReportMaxEntries {
			break
		}
		b := report.Body
		out = append(out, cspViolation{
			directive:   b.EffectiveDirective,
			blocked:     b.BlockedURL,
			document:    b.DocumentURL,
			source:      b.SourceFile,
			line:        b.LineNumber,
			disposition: b.Disposition,
		})
	}
	return out, nil
}

// reportURL returns a URL of a report without its query and fragment,
// which can hold search terms, cut to cspReportFieldMax bytes. A value
// that is not a URL ("inline", "eval") is only cut.
func reportURL(s string) string {
	if u, err := url.Parse(s); err == nil && u.Scheme != "" {
		u.RawQuery, u.ForceQuery, u.Fragment, u.RawFragment = "", false, "", ""
		u.User = nil
		s = u.String()
	}
	return cspField(s)
}

func cspField(s string) string {
	if len(s) <= cspReportFieldMax {
		return s
	}
	return s[:cspReportFieldMax]
}
