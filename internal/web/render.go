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

// PageContent holds the title and body component for a page render.
// Defined per CS-5 to avoid >2 non-ctx arguments in renderPage.
type PageContent struct {
	Title     string
	Content   templ.Component
	Data      any       // Raw data struct for terminal/JSON rendering. Nil for pages without entity data.
	Freshness time.Time // Freshness is the last successful sync time for terminal footer display.
}

// renderPage renders a response in the appropriate format based on terminal detection.
// Priority: query params > Accept header > User-Agent > HX-Request > default (HTML).
// Terminal clients (curl, wget, HTTPie) receive text/plain or application/json.
// Browser and htmx requests receive text/html as before.
// Every response sets Vary: HX-Request, User-Agent, Accept to prevent caching conflicts.
//
// Note on signature: ctx is excluded from arg count per CS-5. w and r are the
// standard http.Handler pair. title and content are grouped into PageContent
// per CS-5 MUST rule (>2 args require input struct).
func renderPage(ctx context.Context, w http.ResponseWriter, r *http.Request, page PageContent) error {
	mode := termrender.Detect(termrender.DetectInput{
		Query:     r.URL.Query(),
		Accept:    r.Header.Get("Accept"),
		UserAgent: r.Header.Get("User-Agent"),
		HXRequest: r.Header.Get("HX-Request") == "true",
	})
	noColor := termrender.HasNoColor(termrender.DetectInput{Query: r.URL.Query()})

	switch mode {
	case termrender.ModeShort:
		w.Header().Set("Vary", "HX-Request, User-Agent, Accept")
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		renderer := termrender.NewRenderer(mode, noColor)
		renderer.Sections = termrender.ParseSections(r.URL.Query().Get("section"))
		if wStr := r.URL.Query().Get("w"); wStr != "" {
			if wVal, err := strconv.Atoi(wStr); err == nil && wVal > 0 {
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
		w.Header().Set("Vary", "HX-Request, User-Agent, Accept")
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		renderer := termrender.NewRenderer(mode, noColor)
		renderer.Sections = termrender.ParseSections(r.URL.Query().Get("section"))
		if wStr := r.URL.Query().Get("w"); wStr != "" {
			if wVal, err := strconv.Atoi(wStr); err == nil && wVal > 0 {
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
		w.Header().Set("Vary", "HX-Request, User-Agent, Accept")
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
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
		w.Header().Set("Vary", "HX-Request, User-Agent, Accept")
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		renderer := termrender.NewRenderer(mode, true) // noColor always true for WHOIS
		renderer.Sections = termrender.ParseSections(r.URL.Query().Get("section"))
		if wStr := r.URL.Query().Get("w"); wStr != "" {
			if wVal, err := strconv.Atoi(wStr); err == nil && wVal > 0 {
				renderer.Width = wVal
			}
		}
		return renderer.RenderWHOIS(w, page.Title, page.Data)

	case termrender.ModeHTMX:
		w.Header().Set("Vary", "HX-Request, User-Agent, Accept")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		return page.Content.Render(ctx, w)

	default: // ModeHTML
		w.Header().Set("Vary", "HX-Request, User-Agent, Accept")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		return templates.Layout(page.Title, page.Content).Render(ctx, w)
	}
}
