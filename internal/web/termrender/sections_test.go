package termrender

import (
	"testing"
)

func TestParseSections_Empty(t *testing.T) {
	t.Parallel()
	got := ParseSections("")
	if got != nil {
		t.Errorf("ParseSections(\"\") = %v, want nil", got)
	}
}

func TestParseSections_Single(t *testing.T) {
	t.Parallel()
	got := ParseSections("ix")
	if !got["ix"] {
		t.Errorf("ParseSections(\"ix\") missing \"ix\", got %v", got)
	}
	if len(got) != 1 {
		t.Errorf("ParseSections(\"ix\") has %d entries, want 1", len(got))
	}
}

func TestParseSections_Multiple(t *testing.T) {
	t.Parallel()
	got := ParseSections("ix,fac")
	if !got["ix"] || !got["fac"] {
		t.Errorf("ParseSections(\"ix,fac\") = %v, want ix and fac", got)
	}
	if len(got) != 2 {
		t.Errorf("ParseSections(\"ix,fac\") has %d entries, want 2", len(got))
	}
}

func TestParseSections_Aliases(t *testing.T) {
	t.Parallel()
	got := ParseSections("exchanges,facilities")
	if !got["ix"] {
		t.Errorf("ParseSections(\"exchanges,facilities\") missing \"ix\", got %v", got)
	}
	if !got["fac"] {
		t.Errorf("ParseSections(\"exchanges,facilities\") missing \"fac\", got %v", got)
	}
}

func TestParseSections_Whitespace(t *testing.T) {
	t.Parallel()
	got := ParseSections(" ix , fac ")
	if !got["ix"] || !got["fac"] {
		t.Errorf("ParseSections(\" ix , fac \") = %v, want ix and fac", got)
	}
}

func TestParseSections_Unknown(t *testing.T) {
	t.Parallel()
	got := ParseSections("ix,bogus")
	if !got["ix"] {
		t.Errorf("ParseSections(\"ix,bogus\") missing \"ix\", got %v", got)
	}
	if len(got) != 1 {
		t.Errorf("ParseSections(\"ix,bogus\") has %d entries, want 1 (bogus ignored)", len(got))
	}
}

func TestParseSections_CaseInsensitive(t *testing.T) {
	t.Parallel()
	got := ParseSections("IX,FAC")
	if !got["ix"] || !got["fac"] {
		t.Errorf("ParseSections(\"IX,FAC\") = %v, want ix and fac", got)
	}
}

func TestParseSections_AllUnknown(t *testing.T) {
	t.Parallel()
	got := ParseSections("bogus,invalid")
	if got != nil {
		t.Errorf("ParseSections(\"bogus,invalid\") = %v, want nil (all unknown)", got)
	}
}

func TestParseSections_WhitespaceOnly(t *testing.T) {
	t.Parallel()
	got := ParseSections("   ")
	if got != nil {
		t.Errorf("ParseSections(\"   \") = %v, want nil", got)
	}
}

func TestShouldShowSection_NilMap(t *testing.T) {
	t.Parallel()
	if !ShouldShowSection(nil, "ix") {
		t.Error("ShouldShowSection(nil, \"ix\") = false, want true")
	}
}

func TestShouldShowSection_Present(t *testing.T) {
	t.Parallel()
	sections := map[string]bool{"ix": true}
	if !ShouldShowSection(sections, "ix") {
		t.Error("ShouldShowSection(map[ix:true], \"ix\") = false, want true")
	}
}

func TestShouldShowSection_Absent(t *testing.T) {
	t.Parallel()
	sections := map[string]bool{"fac": true}
	if ShouldShowSection(sections, "ix") {
		t.Error("ShouldShowSection(map[fac:true], \"ix\") = true, want false")
	}
}
