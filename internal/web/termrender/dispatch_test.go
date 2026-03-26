package termrender

import (
	"bytes"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/dotwaffle/peeringdb-plus/internal/web/templates"
)

func TestRenderPageDispatch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		data     any
		wantSub  string // substring expected in output
		wantStub bool   // true if stub fallback expected
	}{
		{
			name:    "NetworkDetail dispatches to network renderer",
			data:    templates.NetworkDetail{Name: "TestNet", ASN: 64496},
			wantSub: "TestNet",
		},
		{
			name:    "IXDetail dispatches to IX renderer",
			data:    templates.IXDetail{Name: "TestIX"},
			wantSub: "TestIX",
		},
		{
			name:    "FacilityDetail dispatches to facility renderer",
			data:    templates.FacilityDetail{Name: "TestFac"},
			wantSub: "TestFac",
		},
		{
			name:    "OrgDetail dispatches to org renderer",
			data:    templates.OrgDetail{Name: "TestOrg"},
			wantSub: "TestOrg",
		},
		{
			name:    "CampusDetail dispatches to campus renderer",
			data:    templates.CampusDetail{Name: "TestCampus"},
			wantSub: "TestCampus",
		},
		{
			name:    "CarrierDetail dispatches to carrier renderer",
			data:    templates.CarrierDetail{Name: "TestCarrier"},
			wantSub: "TestCarrier",
		},
		{
			name: "SearchGroup slice dispatches to search renderer",
			data: []templates.SearchGroup{
				{TypeName: "Networks", HasMore: false, Results: []templates.SearchResult{{Name: "SearchHit"}}},
			},
			wantSub: "SearchHit",
		},
		{
			name:     "unregistered type falls back to stub",
			data:     42,
			wantSub:  "future update",
			wantStub: true,
		},
		{
			name:     "nil data falls back to stub",
			data:     nil,
			wantSub:  "future update",
			wantStub: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r := NewRenderer(ModePlain, true)
			var buf bytes.Buffer
			err := r.RenderPage(&buf, "Stub Title", tt.data)
			if err != nil {
				t.Fatalf("RenderPage() error: %v", err)
			}
			output := buf.String()
			if !strings.Contains(output, tt.wantSub) {
				t.Errorf("RenderPage() output missing %q, got:\n%s", tt.wantSub, output)
			}
		})
	}
}

func TestRegisterCustomType(t *testing.T) {
	t.Parallel()

	type customData struct{ Msg string }

	called := false
	Register(func(d customData, w io.Writer, r *Renderer) error {
		called = true
		_, err := w.Write([]byte("custom:" + d.Msg))
		return err
	})
	t.Cleanup(func() {
		// Remove the test registration to avoid polluting other tests.
		var zero customData
		delete(renderers, reflect.TypeOf(zero))
	})

	r := NewRenderer(ModePlain, true)
	var buf bytes.Buffer
	err := r.RenderPage(&buf, "ignored", customData{Msg: "hello"})
	if err != nil {
		t.Fatalf("RenderPage() error: %v", err)
	}
	if !called {
		t.Error("custom render function was not called")
	}
	if got := buf.String(); got != "custom:hello" {
		t.Errorf("RenderPage() = %q, want %q", got, "custom:hello")
	}
}
