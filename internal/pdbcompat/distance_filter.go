package pdbcompat

import (
	"errors"
	"math"
	"net/url"
	"strconv"
	"strings"

	"entgo.io/ent/dialect/sql"

	"github.com/dotwaffle/peeringdb-plus/internal/peeringdb"
)

// This file ports the distance search of fac and org (PeeringDB 2.83.0
// prepare_spatial_search, serializers.py:1837-1905, called at
// :2203-2208 for fac and :4985-4990 for org). The mirror has no
// geocoder, so the search needs latitude and longitude.

// distanceTypes holds the types whose prepare_query runs
// prepare_spatial_search. Campus inherits SpatialSearchMixin
// (serializers.py:4779), but its prepare_query does not call it.
var distanceTypes = map[string]bool{peeringdb.TypeFac: true, peeringdb.TypeOrg: true}

// spatialSkipKeys holds the keys that the upstream filter loop skips
// when the query is a distance search (rest.py:569-581). Exact keys
// only: city__contains, state__in or latitude__gt still filter.
var spatialSkipKeys = map[string]bool{
	"latitude":  true,
	"longitude": true,
	"address1":  true,
	"city":      true,
	"city__in":  true,
	"state":     true,
	"zipcode":   true,
}

// errDistanceInvalid is the text of single_url_param for a value that
// float() rejects (serializers.py:443-460).
var errDistanceInvalid = errors.New("Invalid value") //nolint:staticcheck // exact upstream message text

// distanceSearch is a distance search: the rows within km kilometers of
// the point (lat, lng).
type distanceSearch struct {
	lat, lng, km float64
}

// parseDistanceSearch returns the distance search of a fac or org
// request, or nil when the request is not a distance search. Only the
// distance key decides: upstream checks for the key in prepare_query
// (serializers.py:2205, :4987) and takes its first value
// (single_url_param). A value of 0 or less is a no-op (:1839-1840). A
// positive value needs latitude and longitude; upstream finds the
// coordinates of a city and country through a geocoder (:1842-1880),
// which the mirror does not have.
//
// The error text starts with the key, for example "distance: Invalid
// value".
func parseDistanceSearch(typ string, params url.Values) (*distanceSearch, error) {
	if !distanceTypes[typ] {
		return nil, nil
	}
	vals, ok := params["distance"]
	if !ok || len(vals) == 0 {
		return nil, nil
	}
	km, err := parsePyFloat(vals[0])
	// Upstream passes nan on (nan <= 0 is false). SQLite binds NaN as
	// NULL, which matches no row with no error, so the mirror rejects
	// it (a registered divergence).
	if err != nil || math.IsNaN(km) {
		return nil, errors.New("distance: " + errDistanceInvalid.Error())
	}
	if km <= 0 {
		return nil, nil
	}
	latVals, hasLat := params["latitude"]
	lngVals, hasLng := params["longitude"]
	if hasLat && hasLng && len(latVals) > 0 && len(lngVals) > 0 {
		// Upstream passes the raw lists of URL values to the database
		// (serializers.py:1881-1898). The mirror parses the first value
		// and rejects a value that is not a finite number (a registered
		// divergence).
		lat, err := parseCoordinate(latVals[0])
		if err != nil {
			return nil, errors.New("latitude: " + err.Error())
		}
		lng, err := parseCoordinate(lngVals[0])
		if err != nil {
			return nil, errors.New("longitude: " + err.Error())
		}
		return &distanceSearch{lat: lat, lng: lng, km: km}, nil
	}
	return nil, missingCoordinatesError(typ, params)
}

// missingCoordinatesError returns the error of a distance search without
// latitude and longitude. Upstream requires country (or country__in)
// and city, in that order, and names only the missing keys
// (serializers.py:1852-1865). The fac override first looks for a
// location through name_search when city and country are not both
// given (:2213-2280). With the address keys present, upstream geocodes
// them; the mirror cannot.
func missingCoordinatesError(typ string, params url.Values) error {
	hasKey := func(k string) bool { _, ok := params[k]; return ok }
	hasCity := hasKey("city")
	hasCountry := hasKey("country") || hasKey("country__in")
	geocode := errors.New("distance: needs latitude and longitude, the mirror cannot find the coordinates of an address")
	if typ == peeringdb.TypeFac && hasKey("name_search") && (!hasCity || !hasCountry) {
		return geocode
	}
	var missing []string
	if !hasCountry {
		missing = append(missing, "country: Required for distance filtering")
	}
	if !hasCity {
		missing = append(missing, "city: Required for distance filtering")
	}
	if len(missing) > 0 {
		return errors.New(strings.Join(missing, "; "))
	}
	return geocode
}

// parseCoordinate parses a latitude or longitude value with the rules
// of parsePyFloat. A value that is not a finite number is an error.
func parseCoordinate(v string) (float64, error) {
	f, err := parsePyFloat(v)
	if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
		return 0, errDistanceInvalid
	}
	return f, nil
}

// expr returns the great-circle distance in kilometers between the
// point and the row, the upstream formula (serializers.py:1893). SQLite
// has no greatest() or least(): the scalar max() and min() are the
// same, and NULL in, NULL out. The clamp keeps acos() in its domain:
// acos(1.0000000000000002) is NULL. A row with a NULL latitude or
// longitude has a NULL distance and never matches.
func (d *distanceSearch) expr(s *sql.Selector) sql.Querier {
	lat, lng := s.C("latitude"), s.C("longitude")
	return sql.ExprP("6371 * acos(min(max(cos(radians(?)) * cos(radians("+lat+
		")) * cos(radians("+lng+") - radians(?)) + sin(radians(?)) * sin(radians("+lat+
		")), -1), 1))", d.lat, d.lng, d.lat)
}

// predicate keeps the rows within the distance, inclusive
// (distance__lte, serializers.py:1898).
func (d *distanceSearch) predicate() func(*sql.Selector) {
	return func(s *sql.Selector) {
		s.Where(sql.P(func(b *sql.Builder) {
			b.Join(d.expr(s)).WriteString(" <= ").Arg(d.km)
		}))
	}
}

// order sorts the rows nearest first (order_by("distance"),
// serializers.py:1897).
func (d *distanceSearch) order() func(*sql.Selector) {
	return func(s *sql.Selector) { s.OrderExpr(d.expr(s)) }
}

// parsePyFloat parses a value as Python float() does: white space at
// the ends is ignored, a value out of range is +Inf or -Inf and not an
// error (float("1e999") is inf), and hex is rejected (Go ParseFloat
// accepts "0x1p3"). Go ParseFloat already accepts "inf", "Infinity",
// "nan" and single underscores between digits, as float() does.
// Non-ASCII digits are not mapped, which float() accepts.
func parsePyFloat(v string) (float64, error) {
	s := strings.TrimSpace(v)
	if strings.ContainsAny(s, "xX") {
		return 0, strconv.ErrSyntax
	}
	f, err := strconv.ParseFloat(s, 64)
	if errors.Is(err, strconv.ErrRange) {
		// ParseFloat returns +-Inf for an overflow and 0 for an
		// underflow, as float() does.
		return f, nil
	}
	return f, err
}
