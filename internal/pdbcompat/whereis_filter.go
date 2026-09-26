package pdbcompat

import (
	"encoding/json"
	"fmt"
	"net/netip"
	"strings"

	"entgo.io/ent/dialect/sql"

	"github.com/dotwaffle/peeringdb-plus/ent/ixprefix"
	"github.com/dotwaffle/peeringdb-plus/internal/peeringdb"
)

// This file ports the ixpfx key whereis (PeeringDB 2.83.0
// IXLanPrefixSerializer.prepare_query, serializers.py:4154-4168, and
// IXLanPrefix.whereis_ip, models.py:5179-5197). The key keeps the
// prefixes that contain one IP address.

// lookupWhereisKey reports whether key is a whereis form on typ, and
// whether it is the __in form. get_relation_filters maps whereis and
// whereis__{lt,lte,gt,gte,contains,startswith} to the same lookup
// (serializers.py:614-656), and whereis_ip does not use the operator.
// whereis__in gives whereis_ip a list. Upstream ignores the other forms,
// so they are not keys here.
func lookupWhereisKey(typ, key string) (inList, ok bool) {
	rest, found := strings.CutPrefix(key, "whereis")
	if typ != peeringdb.TypeIXPfx || !found {
		return false, false
	}
	switch rest {
	case "", "__lt", "__lte", "__gt", "__gte", "__contains", "__startswith":
		return false, true
	case "__in":
		return true, true
	}
	return false, false
}

// buildWhereisPredicate keeps the rows whose prefix contains the address
// in value. It returns an error, which is a 400, for the __in form and
// for a value that ipaddress.ip_address rejects upstream
// (rest.py:493-500). The value is not trimmed: Python does not strip it.
func buildWhereisPredicate(value string, inList bool) (func(*sql.Selector), error) {
	if inList {
		// Upstream splits the value, and ip_address(list) raises
		// ValueError.
		return nil, fmt.Errorf("%q does not appear to be an IPv4 or IPv6 address", value)
	}
	addr, err := netip.ParseAddr(value)
	// Python rejects a zone that holds '%' or '/' (ipaddress.py:1905-1920).
	// Go accepts it.
	if err != nil || strings.ContainsAny(addr.Zone(), "%/") {
		return nil, fmt.Errorf("%q does not appear to be an IPv4 or IPv6 address", value)
	}
	list, err := json.Marshal(whereisCandidates(addr))
	if err != nil {
		return nil, fmt.Errorf("marshal whereis prefixes: %w", err)
	}
	// The candidates bind as one JSON array, so the lookup adds one
	// parameter and no SQL term per prefix length.
	return func(s *sql.Selector) {
		s.Where(sql.ExprP(s.C(ixprefix.FieldPrefix)+" IN (SELECT value FROM json_each(?))", string(list)))
	}, nil
}

// whereisCandidates returns every prefix that contains addr, one for
// each length from 0 to addr.BitLen(), in the canonical form that
// upstream stores (str(ip_network), which does not accept host bits). An
// IPv4-mapped address stays IPv6, as in Python, so its candidates never
// equal an IPv4 prefix. Addr.Prefix drops the zone.
func whereisCandidates(addr netip.Addr) []string {
	out := make([]string, 0, addr.BitLen()+1)
	for bits := 0; bits <= addr.BitLen(); bits++ {
		p, err := addr.Prefix(bits)
		if err != nil {
			// Prefix fails only for a length out of range.
			continue
		}
		out = append(out, p.String())
	}
	return out
}
