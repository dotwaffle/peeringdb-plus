package pdbcompat

import (
	"context"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/dotwaffle/peeringdb-plus/ent"
	"github.com/dotwaffle/peeringdb-plus/ent/campus"
	"github.com/dotwaffle/peeringdb-plus/ent/carrier"
	"github.com/dotwaffle/peeringdb-plus/ent/facility"
	"github.com/dotwaffle/peeringdb-plus/ent/internetexchange"
	"github.com/dotwaffle/peeringdb-plus/ent/network"
	"github.com/dotwaffle/peeringdb-plus/ent/organization"
	"github.com/dotwaffle/peeringdb-plus/internal/pdbtypes"
	"github.com/dotwaffle/peeringdb-plus/internal/peeringdb"
)

// This file ports the anonymous responses of the upstream PeeringDB
// 2.83.0 asset route (AssetViewSet, rest.py:1426-1590, route
// rest.py:2109-2119, serializers.py:5000-5294). The route reads,
// uploads and deletes the logo of an object. The mirror is read-only,
// so it answers a write as upstream answers a caller without an API
// key.

// assetRoute is the upstream route, anchored as Django matches a
// pattern that ends with "$". Python \d matches every Unicode decimal
// digit. The route has no format suffix: "logo.json" is an asset type.
var assetRoute = regexp.MustCompile(`^asset/([^/]+)/(\p{Nd}+)/([^/]+)/?$`)

// assetAllow is the Allow header of each response of the route (DRF
// views.py:159-165, :443-449): the mapped methods, with HEAD for GET
// (viewsets.py:105-106) and OPTIONS.
const assetAllow = "GET, POST, PUT, DELETE, HEAD, OPTIONS"

// assetPath holds the path parameters of the asset route.
type assetPath struct {
	refTag, refID, assetType string
}

// matchAsset returns the path parameters when rest, the path after
// /api/, matches the asset route.
func matchAsset(rest string) (assetPath, bool) {
	m := assetRoute.FindStringSubmatch(rest)
	if m == nil {
		return assetPath{}, false
	}
	return assetPath{refTag: m[1], refID: m[2], assetType: m[3]}, true
}

// assetRefTags are the choices of the ref_tag field, in upstream order
// (serializers.py:5047-5051).
var assetRefTags = []string{
	peeringdb.TypeOrg, peeringdb.TypeFac, peeringdb.TypeNet,
	peeringdb.TypeIX, peeringdb.TypeCarrier, peeringdb.TypeCampus,
}

// assetDescription is the dedented docstring of AssetViewSet
// (rest.py:1427-1437).
const assetDescription = `Unified API endpoint for managing logos across all entity types.

Supports GET, POST, PUT, DELETE operations for logos on:
- Organizations (ref_tag="org")
- Facilities (ref_tag="fac")
- Networks (ref_tag="net")
- Internet Exchanges (ref_tag="ix")
- Carriers (ref_tag="carrier")
- Campuses (ref_tag="campus")`

// assetActions is the actions key of the OPTIONS body. DRF describes
// the serializer of the view for POST and PUT (metadata.py:74-100).
// view.action is None for OPTIONS, so get_serializer_class returns
// AssetReadSerializer, whose fields are all read-only. The file holds
// the output of DRF 3.18.1 for the upstream serializer. Upstream
// builds the key from a set, so the order of POST and PUT changes with
// the Python hash seed; the file has POST first.
//
//go:embed asset_actions.json
var assetActions json.RawMessage

// assetMetadata is the OPTIONS body of the asset route.
var assetMetadata = drfMetadata{
	Name:        "Asset",
	Description: assetDescription,
	Renders:     drfRenders,
	Parses:      drfParses,
	Actions:     assetActions,
}

// Upstream texts of the asset route.
const (
	// errAssetNotFound is the NotFound text of AssetDeleteSerializer
	// for an object without a logo (serializers.py:5086-5087).
	errAssetNotFound = "Asset does not exist"
	// errAssetNoDelete is the 403 text of destroy for a caller without
	// delete permission (rest.py:1576-1580).
	errAssetNoDelete = "No delete permissions to this entity"
	// errAssetNoWrite is the 403 text of create and update for a caller
	// without write permission (rest.py:1512-1516, :1546-1550).
	errAssetNoWrite = "No write permissions to this entity"
	// assetMaxIDLength is the IntegerField MAX_STRING_LENGTH of DRF
	// (fields.py:917, :928-929).
	assetMaxIDLength = 1000
)

// assetEntity is the object of an asset request.
type assetEntity struct {
	id               int
	logo             *string
	created, updated time.Time
}

// serveAsset answers a request that the asset route matches. DRF runs
// content negotiation first and then the method check (views.py:
// 404-421, :513-521). The permission classes of the view pass for a
// caller without an API key: the view has no Grainy decorator
// (permissions.py:295-302). WriteRateThrottle is not mirrored
// (docs/API.md § Known Divergences). OPTIONS gets the view metadata,
// with no database read.
func (h *Handler) serveAsset(w http.ResponseWriter, r *http.Request, p assetPath) {
	w.Header().Set("Allow", assetAllow)
	if !negotiate(w, r, "") {
		return
	}
	switch r.Method {
	case http.MethodOptions:
		writeDRFMetadata(w, assetMetadata)
	case http.MethodGet, http.MethodHead:
		h.serveAssetRead(w, r, p)
	case http.MethodDelete:
		h.serveAssetDelete(w, r, p)
	case http.MethodPost, http.MethodPut:
		h.serveAssetWrite(w, r, p)
	default:
		writeMethodNotAllowed(w, r, assetAllow)
	}
}

// serveAssetRead answers GET and HEAD (retrieve, rest.py:1465-1493).
// Upstream checks read permission on the object. The Guest group, whose
// permissions an anonymous caller gets (settings/__init__.py:1712),
// reads every object of an organization, so the check passes.
func (h *Handler) serveAssetRead(w http.ResponseWriter, r *http.Request, p assetPath) {
	e, ok := h.assetLookup(w, r, p)
	if !ok {
		return
	}
	// net/http sends no body for HEAD, so HEAD does not read the file.
	var data []byte
	fetched := false
	if r.Method == http.MethodGet && e.logo != nil && *e.logo != "" && h.logos != nil {
		data, fetched = h.logos.get(r.Context(), *e.logo)
	}
	WriteResponse(w, []assetResponse{assetRow(p.refTag, e, data, fetched)})
}

// serveAssetDelete answers DELETE (destroy, rest.py:1560-1590): 404
// when the object has no logo, else 403.
func (h *Handler) serveAssetDelete(w http.ResponseWriter, r *http.Request, p assetPath) {
	e, ok := h.assetLookup(w, r, p)
	if !ok {
		return
	}
	if e.logo == nil || *e.logo == "" {
		writeDetailNotFound(w, r, errAssetNotFound)
		return
	}
	writeError(w, r, apiError{Status: http.StatusForbidden, Detail: errAssetNoDelete})
}

// assetPathErrors returns the field errors of the path parameters, in
// the field order of AssetLookupSerializer (serializers.py:5040-5059),
// and the id.
func assetPathErrors(p assetPath) (errs []fieldError, id int, idText string) {
	if !slices.Contains(assetRefTags, p.refTag) {
		errs = append(errs, fieldError{Field: "ref_tag", Message: invalidChoice([]rune(p.refTag))})
	}
	// The path holds only digits, so int() cannot fail. A value that
	// does not fit saturates and matches no row, as the Django lookup
	// of an out-of-range value matches none. The message prints the
	// whole value.
	if utf8.RuneCountInString(p.refID) > assetMaxIDLength {
		errs = append(errs, fieldError{Field: "ref_id", Message: "String value too large."})
	} else {
		id, idText, _ = pyInt(p.refID)
	}
	if p.assetType != "logo" {
		errs = append(errs, fieldError{Field: "asset_type", Message: invalidChoice([]rune(p.assetType))})
	}
	return errs, id, idText
}

// invalidChoice is the DRF ChoiceField invalid_choice text for the
// str() of the value (fields.py:1378, :1403-1408), as WTF-8
// (fieldError).
func invalidChoice(v []rune) string {
	return `"` + wtf8String(v) + `" is not a valid choice.`
}

// assetLookup validates the path and reads the object with status ok
// (validate_asset_lookup, serializers.py:5000-5037). It writes the 400
// of a field error or of a missing object and returns false.
func (h *Handler) assetLookup(w http.ResponseWriter, r *http.Request, p assetPath) (assetEntity, bool) {
	errs, id, idText := assetPathErrors(p)
	if len(errs) > 0 {
		writeError(w, r, apiError{Status: http.StatusBadRequest, Fields: errs})
		return assetEntity{}, false
	}
	e, err := h.assetEntity(r.Context(), p.refTag, id)
	if ent.IsNotFound(err) {
		model, _ := pdbtypes.DjangoModelOf(p.refTag)
		writeError(w, r, apiError{
			Status: http.StatusBadRequest,
			Fields: []fieldError{{Field: "ref_id", Message: model + " with id " + idText + " not found"}},
		})
		return assetEntity{}, false
	}
	if err != nil {
		slog.ErrorContext(r.Context(), "pdbcompat: asset query failed",
			slog.String("ref_tag", p.refTag),
			slog.String("error", err.Error()),
		)
		writeError(w, r, apiError{
			Status: http.StatusInternalServerError,
			Detail: "failed to query record",
		})
		return assetEntity{}, false
	}
	return e, true
}

// assetEntity reads the object of type tag with id and status ok. The
// error is an ent NotFoundError when no such row exists.
func (h *Handler) assetEntity(ctx context.Context, tag string, id int) (assetEntity, error) {
	switch tag {
	case peeringdb.TypeOrg:
		o, err := h.client.Organization.Query().Where(organization.ID(id), organization.StatusEQ("ok")).Only(ctx)
		if err != nil {
			return assetEntity{}, err
		}
		return assetEntity{id: o.ID, logo: o.Logo, created: o.Created, updated: o.Updated}, nil
	case peeringdb.TypeFac:
		f, err := h.client.Facility.Query().Where(facility.ID(id), facility.StatusEQ("ok")).Only(ctx)
		if err != nil {
			return assetEntity{}, err
		}
		return assetEntity{id: f.ID, logo: f.Logo, created: f.Created, updated: f.Updated}, nil
	case peeringdb.TypeNet:
		n, err := h.client.Network.Query().Where(network.ID(id), network.StatusEQ("ok")).Only(ctx)
		if err != nil {
			return assetEntity{}, err
		}
		return assetEntity{id: n.ID, logo: n.Logo, created: n.Created, updated: n.Updated}, nil
	case peeringdb.TypeIX:
		x, err := h.client.InternetExchange.Query().Where(internetexchange.ID(id), internetexchange.StatusEQ("ok")).Only(ctx)
		if err != nil {
			return assetEntity{}, err
		}
		return assetEntity{id: x.ID, logo: x.Logo, created: x.Created, updated: x.Updated}, nil
	case peeringdb.TypeCarrier:
		c, err := h.client.Carrier.Query().Where(carrier.ID(id), carrier.StatusEQ("ok")).Only(ctx)
		if err != nil {
			return assetEntity{}, err
		}
		return assetEntity{id: c.ID, logo: c.Logo, created: c.Created, updated: c.Updated}, nil
	case peeringdb.TypeCampus:
		c, err := h.client.Campus.Query().Where(campus.ID(id), campus.StatusEQ("ok")).Only(ctx)
		if err != nil {
			return assetEntity{}, err
		}
		return assetEntity{id: c.ID, logo: c.Logo, created: c.Created, updated: c.Updated}, nil
	}
	return assetEntity{}, errors.New("unknown asset ref_tag " + tag)
}

// assetResponse is the body row of AssetReadSerializer
// (serializers.py:5256-5294).
type assetResponse struct {
	RefTag    string  `json:"ref_tag"`
	RefID     int     `json:"ref_id"`
	AssetType string  `json:"asset_type"`
	FileType  *string `json:"file_type"`
	FileData  *string `json:"file_data"`
	Created   string  `json:"created"`
	Updated   string  `json:"updated"`
}

// assetRow returns the body row of e. data is the logo file when
// fetched is true. Upstream reads the file type from the suffix of the
// stored file name, and sends file_data null when it cannot read the
// file (serializers.py:5266-5283).
func assetRow(tag string, e assetEntity, data []byte, fetched bool) assetResponse {
	row := assetResponse{
		RefTag:    tag,
		RefID:     e.id,
		AssetType: "logo",
		Created:   pyISOFormat(e.created),
		Updated:   pyISOFormat(e.updated),
	}
	if e.logo != nil && *e.logo != "" {
		name := strings.ToLower(*e.logo)
		var fileType string
		switch {
		case strings.HasSuffix(name, ".png"):
			fileType = "image/png"
		case strings.HasSuffix(name, ".jpg"), strings.HasSuffix(name, ".jpeg"):
			fileType = "image/jpeg"
		}
		if fileType != "" {
			row.FileType = &fileType
		}
	}
	if fetched {
		s := base64.StdEncoding.EncodeToString(data)
		row.FileData = &s
	}
	return row
}

// pyISOFormat renders t as the upstream JSON encoder renders a UTC
// datetime (Python datetime.isoformat): microseconds only when they are
// not zero, and "+00:00" for the zone.
func pyISOFormat(t time.Time) string {
	t = t.UTC()
	layout := "2006-01-02T15:04:05"
	if t.Nanosecond()/1000 != 0 {
		layout += ".000000"
	}
	return t.Format(layout) + "+00:00"
}
