package termrender

import (
	"regexp"
	"strings"
	"testing"
)

// ansiRE strips ANSI escape sequences for test assertions.
var ansiRE = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func TestFormatSpeed(t *testing.T) {
	t.Parallel()

	tests := []struct {
		mbps int
		want string
	}{
		{0, "0M"},
		{500, "500M"},
		{1000, "1G"},
		{10000, "10G"},
		{100000, "100G"},
		{400000, "400G"},
		{1000000, "1T"},
		{2500000, "2T"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			t.Parallel()
			got := FormatSpeed(tt.mbps)
			if got != tt.want {
				t.Errorf("FormatSpeed(%d) = %q, want %q", tt.mbps, got, tt.want)
			}
		})
	}
}

func TestFormatBandwidth(t *testing.T) {
	t.Parallel()

	tests := []struct {
		mbps int
		want string
	}{
		{0, "0 Mbps"},
		{500, "500 Mbps"},
		{1000, "1 Gbps"},
		{10000, "10 Gbps"},
		{100000, "100 Gbps"},
		{1000000, "1.0 Tbps"},
		{1500000, "1.5 Tbps"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			t.Parallel()
			got := FormatBandwidth(tt.mbps)
			if got != tt.want {
				t.Errorf("FormatBandwidth(%d) = %q, want %q", tt.mbps, got, tt.want)
			}
		})
	}
}

func TestSpeedStyle(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		mbps int
	}{
		{"sub-1G", 500},
		{"1G", 1000},
		{"10G", 10000},
		{"100G", 100000},
		{"400G", 400000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			style := SpeedStyle(tt.mbps)
			rendered := style.Render("X")
			if !strings.Contains(rendered, "\x1b[") {
				t.Errorf("SpeedStyle(%d).Render() should contain ANSI codes, got: %q", tt.mbps, rendered)
			}
		})
	}

	// Verify 400G+ tier produces bold.
	t.Run("400G-bold", func(t *testing.T) {
		t.Parallel()
		style := SpeedStyle(400000)
		rendered := style.Render("X")
		if !strings.Contains(rendered, "\x1b[1") {
			t.Errorf("SpeedStyle(400000) should produce bold, got: %q", rendered)
		}
	})
}

func TestPolicyStyle(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		policy   string
		wantANSI bool
	}{
		{"Open", "Open", true},
		{"Selective", "Selective", true},
		{"Restrictive", "Restrictive", true},
		{"open-lower", "open", true},
		{"OPEN-upper", "OPEN", true},
		{"Unknown", "Unknown", true},
		{"empty", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := PolicyStyle(tt.policy)
			hasANSI := strings.Contains(got, "\x1b[")
			if tt.wantANSI && !hasANSI {
				t.Errorf("PolicyStyle(%q) should contain ANSI codes, got: %q", tt.policy, got)
			}
			// All outputs must contain the original policy text.
			if tt.policy != "" && !strings.Contains(got, tt.policy) {
				t.Errorf("PolicyStyle(%q) should contain policy text, got: %q", tt.policy, got)
			}
		})
	}

	// Verify case-insensitive matching produces colored output.
	t.Run("case-insensitive", func(t *testing.T) {
		t.Parallel()
		openLower := PolicyStyle("open")
		openTitle := PolicyStyle("Open")
		// Both should contain ANSI codes (not fall through to default).
		if !strings.Contains(openLower, "\x1b[") {
			t.Errorf("PolicyStyle(\"open\") should contain ANSI codes, got: %q", openLower)
		}
		if !strings.Contains(openTitle, "\x1b[") {
			t.Errorf("PolicyStyle(\"Open\") should contain ANSI codes, got: %q", openTitle)
		}
	})
}

func TestCrossRef(t *testing.T) {
	t.Parallel()

	got := CrossRef("/ui/ix/31")
	stripped := ansiRE.ReplaceAllString(got, "")
	if !strings.Contains(stripped, "[/ui/ix/31]") {
		t.Errorf("CrossRef(\"/ui/ix/31\") stripped text should contain \"[/ui/ix/31]\", got raw: %q stripped: %q", got, stripped)
	}
	if !strings.Contains(got, "\x1b[") {
		t.Errorf("CrossRef() output should contain ANSI codes, got: %q", got)
	}
}

func TestWriteKV(t *testing.T) {
	t.Parallel()

	t.Run("normal", func(t *testing.T) {
		t.Parallel()
		var buf strings.Builder
		writeKV(&buf, "Label", "Value", 10)
		got := buf.String()
		if !strings.Contains(got, "Label") {
			t.Errorf("writeKV output should contain label, got: %q", got)
		}
		if !strings.Contains(got, "Value") {
			t.Errorf("writeKV output should contain value, got: %q", got)
		}
		if !strings.HasSuffix(got, "\n") {
			t.Errorf("writeKV output should end with newline, got: %q", got)
		}
	})

	t.Run("empty-value", func(t *testing.T) {
		t.Parallel()
		var buf strings.Builder
		writeKV(&buf, "Label", "", 10)
		if buf.Len() != 0 {
			t.Errorf("writeKV with empty value should produce no output, got: %q", buf.String())
		}
	})

	t.Run("alignment", func(t *testing.T) {
		t.Parallel()
		var buf strings.Builder
		writeKV(&buf, "X", "val", 8)
		got := buf.String()
		// The label should be right-padded to 8 chars (contains spaces before X).
		// Output has ANSI codes around the label, so just check it contains "  " separator.
		if !strings.Contains(got, "  ") {
			t.Errorf("writeKV output should contain two-space separator, got: %q", got)
		}
	})
}
