package termrender

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/dotwaffle/peeringdb-plus/internal/web/templates"
)

// whoisKeyWidth is the column width for RPSL-style key alignment.
// Standard whois output uses keys followed by colon then spaces to column 16.
const whoisKeyWidth = 16

// writeWHOISField writes a single RPSL-formatted key-value line.
// Empty values are omitted. Key is left-aligned, padded to whoisKeyWidth.
func writeWHOISField(buf *strings.Builder, key, value string) {
	if value == "" {
		return
	}
	buf.WriteString(fmt.Sprintf("%-*s%s\n", whoisKeyWidth, key+":", value))
}

// writeWHOISMulti writes multiple values for the same key using RPSL repeated
// key convention (D-14). Each value gets its own line with the same key.
func writeWHOISMulti(buf *strings.Builder, key string, values []string) {
	for _, v := range values {
		writeWHOISField(buf, key, v)
	}
}

// writeWHOISHeader writes the standard WHOIS header comments (D-15).
// Includes source identification and query echo.
func writeWHOISHeader(buf *strings.Builder, query string) {
	buf.WriteString("% Source: PeeringDB-Plus\n")
	buf.WriteString(fmt.Sprintf("%% Query: %s\n", query))
	buf.WriteString("\n")
}

// RenderWHOIS renders entity data in RPSL-like WHOIS format.
// Dispatches to per-entity WHOIS renderers based on data type.
// Returns an "unsupported" message for search/compare views.
// (RND-17, D-10 through D-15)
func (r *Renderer) RenderWHOIS(w io.Writer, _ string, data any) error {
	switch d := data.(type) {
	case templates.NetworkDetail:
		return r.whoisNetwork(w, d)
	case templates.IXDetail:
		return r.whoisIX(w, d)
	case templates.FacilityDetail:
		return r.whoisFacility(w, d)
	case templates.OrgDetail:
		return r.whoisOrg(w, d)
	case templates.CampusDetail:
		return r.whoisCampus(w, d)
	case templates.CarrierDetail:
		return r.whoisCarrier(w, d)
	default:
		// Search/Compare don't have WHOIS representation.
		_, err := fmt.Fprintf(w, "%% WHOIS format is not available for this view.\n%% Use ?format=plain or ?format=json instead.\n")
		return err
	}
}

// whoisNetwork renders a network entity in RPSL aut-num format (D-11, RFC 2622).
func (r *Renderer) whoisNetwork(w io.Writer, data templates.NetworkDetail) error {
	var buf strings.Builder
	buf.Grow(len(data.IXPresences)*40 + len(data.FacPresences)*40 + 500)

	writeWHOISHeader(&buf, fmt.Sprintf("AS%d", data.ASN))

	writeWHOISField(&buf, "aut-num", fmt.Sprintf("AS%d", data.ASN))
	writeWHOISField(&buf, "as-name", data.Name)
	descr := data.NameLong
	if descr == "" {
		descr = data.Name
	}
	writeWHOISField(&buf, "descr", descr)
	writeWHOISField(&buf, "org", data.OrgName)
	writeWHOISField(&buf, "website", data.Website)
	writeWHOISField(&buf, "irr-as-set", data.IRRAsSet)
	writeWHOISField(&buf, "info-type", data.InfoType)
	writeWHOISField(&buf, "policy", data.PolicyGeneral)
	writeWHOISField(&buf, "traffic", data.InfoTraffic)
	writeWHOISField(&buf, "ratio", data.InfoRatio)
	writeWHOISField(&buf, "scope", data.InfoScope)
	if data.InfoPrefixes4 > 0 {
		writeWHOISField(&buf, "prefixes-v4", strconv.Itoa(data.InfoPrefixes4))
	}
	if data.InfoPrefixes6 > 0 {
		writeWHOISField(&buf, "prefixes-v6", strconv.Itoa(data.InfoPrefixes6))
	}
	if data.IXCount > 0 {
		writeWHOISField(&buf, "ix-count", strconv.Itoa(data.IXCount))
	}
	if data.FacCount > 0 {
		writeWHOISField(&buf, "fac-count", strconv.Itoa(data.FacCount))
	}

	// Multi-value IX and facility lines (D-14).
	ixNames := make([]string, 0, len(data.IXPresences))
	for _, ix := range data.IXPresences {
		ixNames = append(ixNames, ix.IXName)
	}
	writeWHOISMulti(&buf, "ix", ixNames)

	facNames := make([]string, 0, len(data.FacPresences))
	for _, fac := range data.FacPresences {
		facNames = append(facNames, fac.FacName)
	}
	writeWHOISMulti(&buf, "fac", facNames)

	writeWHOISField(&buf, "source", "PEERINGDB-PLUS")

	_, err := w.Write([]byte(buf.String()))
	return err
}

// whoisIX renders an IX entity in RPSL-like format (D-12, custom ix: class).
func (r *Renderer) whoisIX(w io.Writer, data templates.IXDetail) error {
	var buf strings.Builder
	buf.Grow(500)

	writeWHOISHeader(&buf, fmt.Sprintf("IX %d", data.ID))

	writeWHOISField(&buf, "ix", strconv.Itoa(data.ID))
	writeWHOISField(&buf, "ix-name", data.Name)
	descr := data.NameLong
	if descr == "" {
		descr = data.Name
	}
	writeWHOISField(&buf, "descr", descr)
	writeWHOISField(&buf, "org", data.OrgName)
	writeWHOISField(&buf, "website", data.Website)
	writeWHOISField(&buf, "city", data.City)
	writeWHOISField(&buf, "country", data.Country)
	writeWHOISField(&buf, "region", data.RegionContinent)
	writeWHOISField(&buf, "media", data.Media)

	// Proto field: comma-separated list of supported protocols.
	var protos []string
	if data.ProtoUnicast {
		protos = append(protos, "unicast")
	}
	if data.ProtoMulticast {
		protos = append(protos, "multicast")
	}
	if data.ProtoIPv6 {
		protos = append(protos, "IPv6")
	}
	if len(protos) > 0 {
		writeWHOISField(&buf, "proto", strings.Join(protos, ", "))
	}

	if data.NetCount > 0 {
		writeWHOISField(&buf, "net-count", strconv.Itoa(data.NetCount))
	}
	if data.FacCount > 0 {
		writeWHOISField(&buf, "fac-count", strconv.Itoa(data.FacCount))
	}
	if data.PrefixCount > 0 {
		writeWHOISField(&buf, "prefix-count", strconv.Itoa(data.PrefixCount))
	}
	if data.AggregateBW > 0 {
		writeWHOISField(&buf, "bandwidth", FormatBandwidth(data.AggregateBW))
	}

	writeWHOISField(&buf, "source", "PEERINGDB-PLUS")

	_, err := w.Write([]byte(buf.String()))
	return err
}

// whoisFacility renders a facility entity in RPSL-like format (D-12, site: class).
func (r *Renderer) whoisFacility(w io.Writer, data templates.FacilityDetail) error {
	var buf strings.Builder
	buf.Grow(500)

	writeWHOISHeader(&buf, fmt.Sprintf("FAC %d", data.ID))

	writeWHOISField(&buf, "site", strconv.Itoa(data.ID))
	writeWHOISField(&buf, "site-name", data.Name)
	descr := data.NameLong
	if descr == "" {
		descr = data.Name
	}
	writeWHOISField(&buf, "descr", descr)
	writeWHOISField(&buf, "org", data.OrgName)

	// Address as multi-value lines.
	var addrs []string
	if data.Address1 != "" {
		addrs = append(addrs, data.Address1)
	}
	if data.Address2 != "" {
		addrs = append(addrs, data.Address2)
	}
	writeWHOISMulti(&buf, "address", addrs)

	writeWHOISField(&buf, "city", data.City)
	writeWHOISField(&buf, "country", data.Country)
	writeWHOISField(&buf, "region", data.RegionContinent)
	writeWHOISField(&buf, "clli", data.CLLI)
	writeWHOISField(&buf, "website", data.Website)

	if data.NetCount > 0 {
		writeWHOISField(&buf, "net-count", strconv.Itoa(data.NetCount))
	}
	if data.IXCount > 0 {
		writeWHOISField(&buf, "ix-count", strconv.Itoa(data.IXCount))
	}
	if data.CarrierCount > 0 {
		writeWHOISField(&buf, "carrier-count", strconv.Itoa(data.CarrierCount))
	}

	writeWHOISField(&buf, "source", "PEERINGDB-PLUS")

	_, err := w.Write([]byte(buf.String()))
	return err
}

// whoisOrg renders an organization entity in RPSL-like format (D-13, organisation: class).
func (r *Renderer) whoisOrg(w io.Writer, data templates.OrgDetail) error {
	var buf strings.Builder
	buf.Grow(500)

	writeWHOISHeader(&buf, fmt.Sprintf("ORG %d", data.ID))

	writeWHOISField(&buf, "organisation", strconv.Itoa(data.ID))
	writeWHOISField(&buf, "org-name", data.Name)

	// Address as multi-value lines.
	var addrs []string
	if data.Address1 != "" {
		addrs = append(addrs, data.Address1)
	}
	if data.Address2 != "" {
		addrs = append(addrs, data.Address2)
	}
	writeWHOISMulti(&buf, "address", addrs)

	writeWHOISField(&buf, "city", data.City)
	writeWHOISField(&buf, "country", data.Country)
	writeWHOISField(&buf, "website", data.Website)

	if data.NetCount > 0 {
		writeWHOISField(&buf, "net-count", strconv.Itoa(data.NetCount))
	}
	if data.FacCount > 0 {
		writeWHOISField(&buf, "fac-count", strconv.Itoa(data.FacCount))
	}
	if data.IXCount > 0 {
		writeWHOISField(&buf, "ix-count", strconv.Itoa(data.IXCount))
	}

	writeWHOISField(&buf, "source", "PEERINGDB-PLUS")

	_, err := w.Write([]byte(buf.String()))
	return err
}

// whoisCampus renders a campus entity in RPSL-like format (D-13, campus: class).
func (r *Renderer) whoisCampus(w io.Writer, data templates.CampusDetail) error {
	var buf strings.Builder
	buf.Grow(300)

	writeWHOISHeader(&buf, fmt.Sprintf("CAMPUS %d", data.ID))

	writeWHOISField(&buf, "campus", strconv.Itoa(data.ID))
	writeWHOISField(&buf, "campus-name", data.Name)
	writeWHOISField(&buf, "org", data.OrgName)
	writeWHOISField(&buf, "city", data.City)
	writeWHOISField(&buf, "country", data.Country)
	writeWHOISField(&buf, "website", data.Website)

	if data.FacCount > 0 {
		writeWHOISField(&buf, "fac-count", strconv.Itoa(data.FacCount))
	}

	writeWHOISField(&buf, "source", "PEERINGDB-PLUS")

	_, err := w.Write([]byte(buf.String()))
	return err
}

// whoisCarrier renders a carrier entity in RPSL-like format (D-13, carrier: class).
func (r *Renderer) whoisCarrier(w io.Writer, data templates.CarrierDetail) error {
	var buf strings.Builder
	buf.Grow(300)

	writeWHOISHeader(&buf, fmt.Sprintf("CARRIER %d", data.ID))

	writeWHOISField(&buf, "carrier", strconv.Itoa(data.ID))
	writeWHOISField(&buf, "carrier-name", data.Name)
	writeWHOISField(&buf, "org", data.OrgName)
	writeWHOISField(&buf, "website", data.Website)

	if data.FacCount > 0 {
		writeWHOISField(&buf, "fac-count", strconv.Itoa(data.FacCount))
	}

	writeWHOISField(&buf, "source", "PEERINGDB-PLUS")

	_, err := w.Write([]byte(buf.String()))
	return err
}
