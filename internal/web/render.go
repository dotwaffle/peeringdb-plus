package web

import (
	"context"
	"net/http"
	"time"

	"github.com/a-h/templ"
	"github.com/dotwaffle/peeringdb-plus/internal/web/templates"
	"github.com/dotwaffle/peeringdb-plus/internal/web/termrender"
)

// PageContent holds the title and body component for a page render.
// Defined per CS-5 to avoid >2 non-ctx arguments in renderPage.
type PageContent struct {
	Title   string
	Content templ.Component
	Data    any // Raw data struct for terminal/JSON rendering. Nil for pages without entity data.
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
	case termrender.ModeRich, termrender.ModePlain:
		w.Header().Set("Vary", "HX-Request, User-Agent, Accept")
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		renderer := termrender.NewRenderer(mode, noColor)
		switch page.Title {
		case "Not Found":
			return renderer.RenderError(w, http.StatusNotFound, "Not Found",
				"The page you're looking for doesn't exist. Try searching instead.")
		case "Server Error":
			return renderer.RenderError(w, http.StatusInternalServerError, "Internal Server Error",
				"An unexpected error occurred. Please try again later.")
		case "Home":
			// Freshness not available without db access; zero time omits the line.
			return renderer.RenderHelp(w, time.Time{})
		default:
			return renderer.RenderPage(w, page.Title, page.Data)
		}

	case termrender.ModeJSON:
		w.Header().Set("Vary", "HX-Request, User-Agent, Accept")
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		if page.Data != nil {
			return termrender.RenderJSON(w, page.Data)
		}
		switch page.Title {
		case "Not Found":
			return termrender.RenderJSON(w, map[string]any{"error": "not found", "status": 404})
		case "Server Error":
			return termrender.RenderJSON(w, map[string]any{"error": "internal server error", "status": 500})
		default:
			return termrender.RenderJSON(w, map[string]string{"title": page.Title})
		}

	case termrender.ModeWHOIS:
		w.Header().Set("Vary", "HX-Request, User-Agent, Accept")
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		renderer := termrender.NewRenderer(mode, true) // noColor always true for WHOIS
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
