package termrender

import (
	"fmt"
	"io"

	"github.com/dotwaffle/peeringdb-plus/internal/web/templates"
)

// RenderShort writes a single pipe-delimited summary line for the given entity.
// Designed for scripting and shell integration (?format=short). Each entity type
// produces a compact one-liner terminated by \n. (DIF-01, D-01 through D-05)
func (r *Renderer) RenderShort(w io.Writer, data any) error {
	var line string
	switch d := data.(type) {
	case templates.NetworkDetail:
		line = fmt.Sprintf("AS%d | %s | %s | %d IXs\n", d.ASN, d.Name, d.PolicyGeneral, d.IXCount)
	case templates.IXDetail:
		line = fmt.Sprintf("%s | %d peers | %s\n", d.Name, d.NetCount, formatLocation(d.City, d.Country))
	case templates.FacilityDetail:
		line = fmt.Sprintf("%s | %d nets | %s\n", d.Name, d.NetCount, formatLocation(d.City, d.Country))
	case templates.OrgDetail:
		line = fmt.Sprintf("%s | %d nets | %d facs\n", d.Name, d.NetCount, d.FacCount)
	case templates.CampusDetail:
		line = fmt.Sprintf("%s | %d facs | %s\n", d.Name, d.FacCount, formatLocation(d.City, d.Country))
	case templates.CarrierDetail:
		line = fmt.Sprintf("%s | %d facs\n", d.Name, d.FacCount)
	case []templates.SearchGroup:
		line = "Use ?format=json for search results in short mode.\n"
	case *templates.CompareData:
		line = "Use ?format=json for comparison data in short mode.\n"
	default:
		line = "Unknown entity type\n"
	}

	_, err := io.WriteString(w, line)
	return err
}
