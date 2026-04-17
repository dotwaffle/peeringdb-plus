package termrender

import (
	"io"
	"reflect"

	"github.com/dotwaffle/peeringdb-plus/internal/web/templates"
)

// renderers maps concrete data types to their terminal render functions.
// Populated at init time by Register calls; looked up by RenderPage.
var renderers = map[reflect.Type]func(any, io.Writer, *Renderer) error{}

// Register associates a data type T with a render function for terminal output.
// Registered functions are called by RenderPage when the data argument matches type T.
func Register[T any](fn func(T, io.Writer, *Renderer) error) {
	renderers[reflect.TypeFor[T]()] = func(v any, w io.Writer, r *Renderer) error {
		return fn(v.(T), w, r)
	}
}

func init() {
	Register(func(d templates.NetworkDetail, w io.Writer, r *Renderer) error {
		return r.RenderNetworkDetail(w, d)
	})
	Register(func(d templates.IXDetail, w io.Writer, r *Renderer) error {
		return r.RenderIXDetail(w, d)
	})
	Register(func(d templates.FacilityDetail, w io.Writer, r *Renderer) error {
		return r.RenderFacilityDetail(w, d)
	})
	Register(func(d templates.OrgDetail, w io.Writer, r *Renderer) error {
		return r.RenderOrgDetail(w, d)
	})
	Register(func(d templates.CampusDetail, w io.Writer, r *Renderer) error {
		return r.RenderCampusDetail(w, d)
	})
	Register(func(d templates.CarrierDetail, w io.Writer, r *Renderer) error {
		return r.RenderCarrierDetail(w, d)
	})
	Register(func(d []templates.SearchGroup, w io.Writer, r *Renderer) error {
		return r.RenderSearch(w, d)
	})
	Register(func(d *templates.CompareData, w io.Writer, r *Renderer) error {
		return r.RenderCompare(w, d)
	})
	// /ui/about dispatches via the AboutPageData bundle so both the
	// freshness and the Phase 61 Privacy & Sync payload reach the
	// terminal renderer in a single registered type.
	Register(func(d templates.AboutPageData, w io.Writer, r *Renderer) error {
		return r.RenderAboutPage(w, d.Freshness, d.Privacy)
	})
}
