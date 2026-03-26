package termrender

import (
	"testing"
)

func TestShouldShowField_Wide(t *testing.T) {
	t.Parallel()
	if !ShouldShowField("net-ix", "ipv6", 120) {
		t.Error("ShouldShowField(\"net-ix\", \"ipv6\", 120) = false, want true")
	}
}

func TestShouldShowField_Narrow(t *testing.T) {
	t.Parallel()
	if ShouldShowField("net-ix", "ipv6", 80) {
		t.Error("ShouldShowField(\"net-ix\", \"ipv6\", 80) = true, want false")
	}
}

func TestShouldShowField_Default(t *testing.T) {
	t.Parallel()
	if !ShouldShowField("net-ix", "name", 40) {
		t.Error("ShouldShowField(\"net-ix\", \"name\", 40) = false, want true (name always shown)")
	}
}

func TestShouldShowField_NoWidth(t *testing.T) {
	t.Parallel()
	if !ShouldShowField("net-ix", "ipv6", 0) {
		t.Error("ShouldShowField(\"net-ix\", \"ipv6\", 0) = false, want true (0 = no restriction)")
	}
}

func TestShouldShowField_UnknownContext(t *testing.T) {
	t.Parallel()
	if !ShouldShowField("bogus-context", "anything", 10) {
		t.Error("ShouldShowField with unknown context should return true")
	}
}

func TestShouldShowField_UnknownField(t *testing.T) {
	t.Parallel()
	if !ShouldShowField("net-ix", "unknown-field", 10) {
		t.Error("ShouldShowField with unknown field should return true (unlisted = always shown)")
	}
}

func TestShouldShowField_ExactThreshold(t *testing.T) {
	t.Parallel()
	// ipv6 threshold for net-ix is 100
	if !ShouldShowField("net-ix", "ipv6", 100) {
		t.Error("ShouldShowField at exact threshold should return true (width >= threshold)")
	}
}

func TestShouldShowField_BelowThreshold(t *testing.T) {
	t.Parallel()
	// ipv6 threshold for net-ix is 100
	if ShouldShowField("net-ix", "ipv6", 99) {
		t.Error("ShouldShowField below threshold should return false")
	}
}

func TestTruncateName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		maxWidth int
		want     string
	}{
		{"fits within maxWidth", "short", 10, "short"},
		{"truncated with ellipsis", "a very long entity name", 10, "a very ..."},
		{"exact fit", "abc", 3, "abc"},
		{"maxWidth <= 3 returns unchanged", "abcd", 3, "abcd"},
		{"empty string", "", 10, ""},
		{"zero width returns unchanged", "test", 0, "test"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := TruncateName(tt.input, tt.maxWidth)
			if got != tt.want {
				t.Errorf("TruncateName(%q, %d) = %q, want %q",
					tt.input, tt.maxWidth, got, tt.want)
			}
		})
	}
}

func TestShouldShowField_IXParticipants(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		field string
		width int
		want  bool
	}{
		{"name always shown", "name", 10, true},
		{"asn always shown", "asn", 10, true},
		{"speed shown at 50", "speed", 50, true},
		{"speed hidden at 49", "speed", 49, false},
		{"ipv4 shown at 80", "ipv4", 80, true},
		{"ipv4 hidden at 79", "ipv4", 79, false},
		{"rs shown at 70", "rs", 70, true},
		{"rs hidden at 69", "rs", 69, false},
		{"ipv6 shown at 100", "ipv6", 100, true},
		{"ipv6 hidden at 99", "ipv6", 99, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := ShouldShowField("ix-participants", tt.field, tt.width)
			if got != tt.want {
				t.Errorf("ShouldShowField(\"ix-participants\", %q, %d) = %v, want %v",
					tt.field, tt.width, got, tt.want)
			}
		})
	}
}
