package termrender

import (
	"bytes"
	"testing"

	"github.com/dotwaffle/peeringdb-plus/internal/web/templates"
)

func TestRenderShort_Network(t *testing.T) {
	t.Parallel()

	r := NewRenderer(ModePlain, false)
	var buf bytes.Buffer

	data := templates.NetworkDetail{
		ASN:           13335,
		Name:          "Cloudflare, Inc.",
		PolicyGeneral: "Open",
		IXCount:       304,
	}

	if err := r.RenderShort(&buf, data); err != nil {
		t.Fatalf("RenderShort() error: %v", err)
	}

	want := "AS13335 | Cloudflare, Inc. | Open | 304 IXs\n"
	if got := buf.String(); got != want {
		t.Errorf("RenderShort(NetworkDetail) =\n  %q\nwant:\n  %q", got, want)
	}
}

func TestRenderShort_IX(t *testing.T) {
	t.Parallel()

	r := NewRenderer(ModePlain, false)
	var buf bytes.Buffer

	data := templates.IXDetail{
		Name:     "DE-CIX Frankfurt",
		NetCount: 900,
		City:     "Frankfurt",
		Country:  "DE",
	}

	if err := r.RenderShort(&buf, data); err != nil {
		t.Fatalf("RenderShort() error: %v", err)
	}

	want := "DE-CIX Frankfurt | 900 peers | Frankfurt, DE\n"
	if got := buf.String(); got != want {
		t.Errorf("RenderShort(IXDetail) =\n  %q\nwant:\n  %q", got, want)
	}
}

func TestRenderShort_Facility(t *testing.T) {
	t.Parallel()

	r := NewRenderer(ModePlain, false)
	var buf bytes.Buffer

	data := templates.FacilityDetail{
		Name:     "Equinix DC1",
		NetCount: 85,
		City:     "Ashburn",
		Country:  "US",
	}

	if err := r.RenderShort(&buf, data); err != nil {
		t.Fatalf("RenderShort() error: %v", err)
	}

	want := "Equinix DC1 | 85 nets | Ashburn, US\n"
	if got := buf.String(); got != want {
		t.Errorf("RenderShort(FacilityDetail) =\n  %q\nwant:\n  %q", got, want)
	}
}

func TestRenderShort_Org(t *testing.T) {
	t.Parallel()

	r := NewRenderer(ModePlain, false)
	var buf bytes.Buffer

	data := templates.OrgDetail{
		Name:     "Cloudflare, Inc.",
		NetCount: 3,
		FacCount: 0,
	}

	if err := r.RenderShort(&buf, data); err != nil {
		t.Fatalf("RenderShort() error: %v", err)
	}

	want := "Cloudflare, Inc. | 3 nets | 0 facs\n"
	if got := buf.String(); got != want {
		t.Errorf("RenderShort(OrgDetail) =\n  %q\nwant:\n  %q", got, want)
	}
}

func TestRenderShort_Campus(t *testing.T) {
	t.Parallel()

	r := NewRenderer(ModePlain, false)
	var buf bytes.Buffer

	data := templates.CampusDetail{
		Name:     "Ashburn Campus",
		FacCount: 5,
		City:     "Ashburn",
		Country:  "US",
	}

	if err := r.RenderShort(&buf, data); err != nil {
		t.Fatalf("RenderShort() error: %v", err)
	}

	want := "Ashburn Campus | 5 facs | Ashburn, US\n"
	if got := buf.String(); got != want {
		t.Errorf("RenderShort(CampusDetail) =\n  %q\nwant:\n  %q", got, want)
	}
}

func TestRenderShort_Carrier(t *testing.T) {
	t.Parallel()

	r := NewRenderer(ModePlain, false)
	var buf bytes.Buffer

	data := templates.CarrierDetail{
		Name:     "Zayo",
		FacCount: 12,
	}

	if err := r.RenderShort(&buf, data); err != nil {
		t.Fatalf("RenderShort() error: %v", err)
	}

	want := "Zayo | 12 facs\n"
	if got := buf.String(); got != want {
		t.Errorf("RenderShort(CarrierDetail) =\n  %q\nwant:\n  %q", got, want)
	}
}

func TestRenderShort_SearchGroups(t *testing.T) {
	t.Parallel()

	r := NewRenderer(ModePlain, false)
	var buf bytes.Buffer

	data := []templates.SearchGroup{
		{TypeName: "Networks", HasMore: false},
	}

	if err := r.RenderShort(&buf, data); err != nil {
		t.Fatalf("RenderShort() error: %v", err)
	}

	got := buf.String()
	want := "Use ?format=json for search results in short mode.\n"
	if got != want {
		t.Errorf("RenderShort([]SearchGroup) =\n  %q\nwant:\n  %q", got, want)
	}
}

func TestRenderShort_CompareData(t *testing.T) {
	t.Parallel()

	r := NewRenderer(ModePlain, false)
	var buf bytes.Buffer

	data := &templates.CompareData{}

	if err := r.RenderShort(&buf, data); err != nil {
		t.Fatalf("RenderShort() error: %v", err)
	}

	got := buf.String()
	want := "Use ?format=json for comparison data in short mode.\n"
	if got != want {
		t.Errorf("RenderShort(*CompareData) =\n  %q\nwant:\n  %q", got, want)
	}
}

func TestRenderShort_Unknown(t *testing.T) {
	t.Parallel()

	r := NewRenderer(ModePlain, false)
	var buf bytes.Buffer

	if err := r.RenderShort(&buf, "unexpected type"); err != nil {
		t.Fatalf("RenderShort() error: %v", err)
	}

	got := buf.String()
	want := "Unknown entity type\n"
	if got != want {
		t.Errorf("RenderShort(string) =\n  %q\nwant:\n  %q", got, want)
	}
}
