package pdbcompat

import (
	"log/slog"
	"net/http"
	"regexp"

	"github.com/dotwaffle/peeringdb-plus/ent/organization"
)

// This file ports the anonymous responses of the organization users
// routes of upstream PeeringDB 2.83.0 (OrganizationUsersViewSet,
// rest.py:1649-1972, routes rest.py:2089-2108). The routes list, add,
// change and remove the users of an organization. The mirror has no
// user data and is read-only, so it answers every request as upstream
// answers a caller without an API key: 404 when the organization is
// not an ok row, else 403.

// orgUsersRoute is one route of OrganizationUsersViewSet.
type orgUsersRoute struct {
	// pattern is the upstream route. Python \d matches every Unicode
	// decimal digit, so the pattern uses \p{Nd}.
	pattern *regexp.Regexp
	// method is the only method that the route maps, and allow is the
	// Allow header of its 405. DRF maps HEAD to the GET action
	// (viewsets.py:105-106).
	method, allow string
}

// orgUsersRoutes are the upstream routes, anchored as Django matches a
// pattern that ends with "$" (re.fullmatch).
var orgUsersRoutes = []orgUsersRoute{
	{regexp.MustCompile(`^org/(\p{Nd}+)/users/?$`), http.MethodGet, "GET, HEAD"},
	{regexp.MustCompile(`^org/(\p{Nd}+)/users/add/?$`), http.MethodPost, "POST"},
	{regexp.MustCompile(`^org/(\p{Nd}+)/users/\p{Nd}+/?$`), http.MethodPut, "PUT"},
	{regexp.MustCompile(`^org/(\p{Nd}+)/users/remove/?$`), http.MethodDelete, "DELETE"},
}

// matchOrgUsers returns the route that rest, the path after /api/,
// matches, and the organization id text of the path.
func matchOrgUsers(rest string) (orgUsersRoute, string, bool) {
	for _, route := range orgUsersRoutes {
		if m := route.pattern.FindStringSubmatch(rest); m != nil {
			return route, m[1], true
		}
	}
	return orgUsersRoute{}, "", false
}

// errOrgUsersAuth is the PermissionDenied text of
// check_permissions_for_action for a caller without an API key
// (rest.py:1751). The handler returns it as {"detail": ...} with
// 403, which the renderer puts in meta.error (renderers.py:134-139).
const errOrgUsersAuth = "Invalid authentication"

// errOrgNotFound is the Http404 text of get_object_or_404(Organization,
// ...) (django/shortcuts.py:88-94).
const errOrgNotFound = "No Organization matches the given query."

// serveOrgUsers answers a request that an organization users route
// matches. DRF runs content negotiation first and then the method
// check (views.py:404-421, :513-521). The throttle of the view is not
// mirrored (docs/API.md § Known Divergences). The handler of each
// route first calls check_permissions_for_action (rest.py:1696-1751):
// get_object_or_404 with status "ok" on the id of the path, and then,
// for a caller without an API key, PermissionDenied.
func (h *Handler) serveOrgUsers(w http.ResponseWriter, r *http.Request, route orgUsersRoute, orgID string) {
	if !negotiate(w, r, "") {
		return
	}
	method := r.Method
	if method == http.MethodHead {
		method = http.MethodGet
	}
	if method != route.method {
		writeMethodNotAllowed(w, r, route.allow)
		return
	}
	// The path holds only digits, so int() cannot fail. A value that
	// does not fit saturates and matches no row, as the Django lookup
	// of an out-of-range value matches none.
	id, _, _ := pyInt(orgID)
	ok, err := h.client.Organization.Query().
		Where(organization.ID(id), organization.StatusEQ("ok")).
		Exist(r.Context())
	if err != nil {
		slog.ErrorContext(r.Context(), "pdbcompat: organization users query failed",
			slog.String("error", err.Error()),
		)
		writeError(w, r, apiError{
			Status: http.StatusInternalServerError,
			Detail: "failed to query record",
		})
		return
	}
	if !ok {
		writeDetailNotFound(w, r, errOrgNotFound)
		return
	}
	writeError(w, r, apiError{Status: http.StatusForbidden, Detail: errOrgUsersAuth})
}
