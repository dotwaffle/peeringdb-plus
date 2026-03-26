package termrender

// columnThresholds defines the minimum ?w= value at which a field is visible.
// Fields not listed are always shown. Width 0 means "no restriction" (show all).
// Columns are dropped progressively as width decreases -- values are never
// truncated with ellipsis, only entire columns are removed.
var columnThresholds = map[string]map[string]int{
	"net-ix": {
		"name":     0,   // always shown
		"speed":    50,  // drop below 50
		"ipv4":     80,  // drop below 80
		"crossref": 90,  // drop below 90
		"rs":       70,  // drop below 70
		"ipv6":     100, // drop below 100
	},
	"net-fac": {
		"name":     0,
		"crossref": 70,
		"location": 60,
	},
	"ix-participants": {
		"name":  0,
		"asn":   0,   // always (it's the crossref)
		"speed": 50,
		"ipv4":  80,
		"rs":    70,
		"ipv6":  100,
	},
	"ix-facilities": {
		"name":     0,
		"crossref": 70,
		"location": 60,
	},
	"ix-prefixes": {
		"prefix":   0,
		"protocol": 0,
		"dfz":      60,
	},
	"fac-networks": {
		"name":     0,
		"crossref": 60,
	},
	"fac-ixps": {
		"name":     0,
		"crossref": 60,
	},
	"fac-carriers": {
		"name":     0,
		"crossref": 60,
	},
}

// TruncateName returns name truncated to maxWidth with "..." suffix.
// Returns name unchanged if it fits within maxWidth or maxWidth <= 3.
func TruncateName(name string, maxWidth int) string {
	if len(name) <= maxWidth || maxWidth <= 3 {
		return name
	}
	return name[:maxWidth-3] + "..."
}

// ShouldShowField reports whether a field should be rendered at a given terminal
// width. context identifies the entity-section (e.g., "net-ix"), field is the
// column name, and width is the terminal width in characters.
// Returns true if width is 0 (no restriction), the context is unknown, or
// the field is not listed (unlisted = always shown).
func ShouldShowField(context, field string, width int) bool {
	if width == 0 {
		return true
	}
	thresholds, ok := columnThresholds[context]
	if !ok {
		return true
	}
	threshold, ok := thresholds[field]
	if !ok {
		return true
	}
	return width >= threshold
}
