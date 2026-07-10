package web

import (
	"context"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/a-h/templ"
	"github.com/dotwaffle/peeringdb-plus/internal/httperr"
	"github.com/dotwaffle/peeringdb-plus/internal/web/templates"
	"github.com/dotwaffle/peeringdb-plus/internal/web/termrender"
)

// maxTerminalWidth is the maximum allowed terminal width for text rendering.
// Values exceeding this are silently capped.
const maxTerminalWidth = 500

// PageContent holds the title and body component for a page render.
// Defined to avoid >2 non-ctx arguments in renderPage.
type PageContent struct {
	Title     string
	Content   templ.Component
	Data      any       // Raw data struct for terminal/JSON rendering. Nil for pages without entity data.
	Freshness time.Time // Freshness is the last successful sync time for terminal footer display.
	Status    int       // HTTP status (0 means 200). Committed by renderPage AFTER headers — WriteHeader first drops Vary/Content-Type.
	NeedsMap  bool      // NeedsMap emits the Leaflet/markercluster head includes; set only on pages that render a MapContainer.
}

// renderPage renders a response in the appropriate format based on terminal detection.
// Priority: query params > Accept header > User-Agent > HX-Request > default (HTML).
// Terminal clients (curl, wget, HTTPie) receive text/plain or application/json.
// Browser and htmx requests receive text/html as before.
// Every response sets Vary: HX-Request, User-Agent, Accept to prevent caching conflicts.
//
// Note on signature: ctx is excluded from arg count. w and r are the
// standard http.Handler pair. title and content are grouped into PageContent
// because >2 args require an input struct.
func renderPage(ctx context.Context, w http.ResponseWriter, r *http.Request, page PageContent) error {
	mode := termrender.Detect(termrender.DetectInput{
		Query:     r.URL.Query(),
		Accept:    r.Header.Get("Accept"),
		UserAgent: r.Header.Get("User-Agent"),
		HXRequest: r.Header.Get("HX-Request") == "true",
	})
	noColor := termrender.HasNoColor(termrender.DetectInput{Query: r.URL.Query()})

	// Add (not Set): the outer Compression middleware (gzhttp) already
	// added Vary: Accept-Encoding before dispatch; Set would clobber it
	// and let shared caches replay a gzipped variant to identity clients.
	w.Header().Add("Vary", "HX-Request, User-Agent, Accept")

	// setHead sets the negotiated Content-Type and THEN commits the
	// response status. Order matters: net/http drops header mutations
	// made after WriteHeader, which previously stripped Vary and let the
	// body sniffer override Content-Type on 404/500 pages.
	setHead := func(contentType string) {
		w.Header().Set("Content-Type", contentType)
		if page.Status != 0 && page.Status != http.StatusOK {
			w.WriteHeader(page.Status)
		}
	}

	switch mode { //nolint:exhaustive // default case handles remaining modes (ModeHTML)
	case termrender.ModeShort:
		setHead("text/plain; charset=utf-8")
		renderer := termrender.NewRenderer(mode, noColor)
		renderer.Sections = termrender.ParseSections(r.URL.Query().Get("section"))
		if wStr := r.URL.Query().Get("w"); wStr != "" {
			if wVal, err := strconv.Atoi(wStr); err == nil && wVal > 0 {
				if wVal > maxTerminalWidth {
					wVal = maxTerminalWidth
				}
				renderer.Width = wVal
			}
		}
		if err := renderer.RenderShort(w, page.Data); err != nil {
			return err
		}
		if footer := termrender.FormatFreshness(page.Freshness); footer != "" {
			_, err := io.WriteString(w, footer)
			return err
		}
		return nil

	case termrender.ModeRich, termrender.ModePlain:
		setHead("text/plain; charset=utf-8")
		renderer := termrender.NewRenderer(mode, noColor)
		renderer.Sections = termrender.ParseSections(r.URL.Query().Get("section"))
		if wStr := r.URL.Query().Get("w"); wStr != "" {
			if wVal, err := strconv.Atoi(wStr); err == nil && wVal > 0 {
				if wVal > maxTerminalWidth {
					wVal = maxTerminalWidth
				}
				renderer.Width = wVal
			}
		}
		switch page.Title {
		case "Not Found":
			return renderer.RenderError(w, http.StatusNotFound, "Not Found",
				"The page you're looking for doesn't exist. Try searching instead.")
		case "Server Error":
			return renderer.RenderError(w, http.StatusInternalServerError, "Internal Server Error",
				"An unexpected error occurred. Please try again later.")
		case "Home":
			return renderer.RenderHelp(w, page.Freshness)
		default:
			if err := renderer.RenderPage(w, page.Title, page.Data); err != nil {
				return err
			}
			if footer := termrender.FormatFreshness(page.Freshness); footer != "" {
				_, err := io.WriteString(w, footer)
				return err
			}
			return nil
		}

	case termrender.ModeJSON:
		setHead("application/json; charset=utf-8")
		if page.Data != nil {
			return termrender.RenderJSON(w, page.Data)
		}
		switch page.Title {
		case "Not Found":
			return termrender.RenderJSON(w, httperr.NewProblemDetail(httperr.WriteProblemInput{
				Status: http.StatusNotFound,
				Detail: "The page you're looking for doesn't exist.",
			}))
		case "Server Error":
			return termrender.RenderJSON(w, httperr.NewProblemDetail(httperr.WriteProblemInput{
				Status: http.StatusInternalServerError,
				Detail: "An unexpected error occurred.",
			}))
		default:
			return termrender.RenderJSON(w, map[string]string{"title": page.Title})
		}

	case termrender.ModeWHOIS:
		setHead("text/plain; charset=utf-8")
		renderer := termrender.NewRenderer(mode, true) // noColor always true for WHOIS
		renderer.Sections = termrender.ParseSections(r.URL.Query().Get("section"))
		if wStr := r.URL.Query().Get("w"); wStr != "" {
			if wVal, err := strconv.Atoi(wStr); err == nil && wVal > 0 {
				if wVal > maxTerminalWidth {
					wVal = maxTerminalWidth
				}
				renderer.Width = wVal
			}
		}
		return renderer.RenderWHOIS(w, page.Title, page.Data)

	case termrender.ModeHTMX:
		setHead("text/html; charset=utf-8")
		return page.Content.Render(ctx, w)

	default: // ModeHTML
		setHead("text/html; charset=utf-8")
		return templates.Layout(templates.LayoutOptions{Title: page.Title, NeedsMap: page.NeedsMap}, page.Content).Render(ctx, w)
	}
}
