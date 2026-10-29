package pdbcompat

import (
	_ "embed"
	"encoding/json"
	"net/http"
	"strings"
	"unicode"

	"github.com/dotwaffle/peeringdb-plus/internal/pdbtypes"
)

// drfMetadata is the body of the DRF OPTIONS response of a view:
// SimpleMetadata.determine_metadata (DRF 3.18.1 metadata.py:59-72), with
// the fields in DRF key order. Upstream renders it in the data envelope
// (renderers.py:123-130).
type drfMetadata struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Renders     []string        `json:"renders"`
	Parses      []string        `json:"parses"`
	Actions     json.RawMessage `json:"actions,omitempty"`
}

// drfRenders holds the media type of the upstream renderer,
// MetaJSONRenderer (2.83.0 settings/__init__.py:1459,
// renderers.py:84).
var drfRenders = []string{"application/json"}

// drfParses holds the media types of the DRF default parsers, which
// upstream does not change (DRF settings.py:35-39).
var drfParses = []string{"application/json", "application/x-www-form-urlencoded", "multipart/form-data"}

// selfMetadata is the OPTIONS body of the self route. DRF takes the name
// from the function name of the api_view and the description from its
// docstring (DRF views.py:23-66, 2.83.0 rest.py:1603-1606).
var selfMetadata = drfMetadata{
	Name:        "View Self Entity",
	Description: "This API View redirect self entity API to the corresponding url",
	Renders:     drfRenders,
	Parses:      drfParses,
}

// orgUsersDescription is the dedented docstring of
// OrganizationUsersViewSet (2.83.0 rest.py:1650-1669).
const orgUsersDescription = `ViewSet for managing users within an organization.

This ViewSet provides endpoints for:
- Listing all users in an organization
- Adding new users to an organization
- Updating user roles within an organization
- Removing users from an organization

All operations require:
1. Valid user OR organization API keys
2. User must be an admin of the organization OR organization key that has specific permission
3. API key must be active (InactiveKeyBlock)

Rate Limits:
- All endpoints are rate-limited to 1 request per 1 second (OrganizationUsersThrottle)

Error Handling:
- Returns appropriate DRF's Response status codes (400, 403, 404, 429)
- Provides detailed error messages in response`

// orgUsersAddActions is the actions key of the OPTIONS body of the
// organization users add route: the fields of UserSerializer
// (2.83.0 serializers.py:5297-5346) for POST, as SimpleMetadata
// describes them (DRF metadata.py:74-151). The file holds the output of
// DRF 3.18.1 for the upstream serializer and the fields of the upstream
// User model (models.py:6651-6706).
//
//go:embed org_users_actions.json
var orgUsersAddActions json.RawMessage

// orgUsersMetadata returns the OPTIONS body of an organization users
// route with the actions actions.
func orgUsersMetadata(actions json.RawMessage) drfMetadata {
	return drfMetadata{
		Name:        "Organization Users",
		Description: orgUsersDescription,
		Renders:     drfRenders,
		Parses:      drfParses,
		Actions:     actions,
	}
}

// writeDRFMetadata writes the 200 OPTIONS response with the body m.
func writeDRFMetadata(w http.ResponseWriter, m drfMetadata) {
	WriteResponse(w, []drfMetadata{m})
}

// readAllow lists only methods that the mirror supports on an ordinary
// API route. Upstream's GET-only routes keep their method restrictions.
func readAllow(typeName string) string {
	if getOnly(typeName) {
		return "GET"
	}
	return "GET, HEAD, OPTIONS"
}

// readMetadata describes a read-only route without advertising writes.
// DRF 3.18.1 routers.py adds List and Instance to viewset names.
func readMetadata(typeName string, detail bool) drfMetadata {
	name := "API Root"
	if typeName != "" {
		model, _ := pdbtypes.DjangoModelOf(typeName)
		name = modelWords(model)
		if detail {
			name += " Instance"
		} else {
			name += " List"
		}
	}
	return drfMetadata{Name: name, Description: "", Renders: drfRenders, Parses: drfParses}
}

func modelWords(model string) string {
	runes := []rune(model)
	var b strings.Builder
	for i, r := range runes {
		if i > 0 && unicode.IsUpper(r) && (unicode.IsLower(runes[i-1]) ||
			i+1 < len(runes) && unicode.IsLower(runes[i+1])) {
			b.WriteByte(' ')
		}
		b.WriteRune(r)
	}
	return b.String()
}
