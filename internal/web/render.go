package web

import (
	"context"
	"net/http"

	"github.com/a-h/templ"
	"github.com/dotwaffle/peeringdb-plus/internal/web/templates"
)

// PageContent holds the title and body component for a page render.
// Defined per CS-5 to avoid >2 non-ctx arguments in renderPage.
type PageContent struct {
	Title   string
	Content templ.Component
}

// renderPage renders a templ component as either a full page (with layout)
// or an htmx fragment, based on the HX-Request header.
// Full page renders include the HTML document shell (DOCTYPE, head, nav, footer).
// Fragment renders return only the content for htmx partial updates.
// Every response sets Vary: HX-Request to prevent caching conflicts.
//
// Note on signature: ctx is excluded from arg count per CS-5. w and r are the
// standard http.Handler pair. title and content are grouped into PageContent
// per CS-5 MUST rule (>2 args require input struct).
func renderPage(ctx context.Context, w http.ResponseWriter, r *http.Request, page PageContent) error {
	w.Header().Set("Vary", "HX-Request")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	if r.Header.Get("HX-Request") == "true" {
		return page.Content.Render(ctx, w)
	}
	return templates.Layout(page.Title, page.Content).Render(ctx, w)
}
