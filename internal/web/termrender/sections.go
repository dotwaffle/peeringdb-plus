package termrender

import "strings"

// sectionAliases maps user-friendly section names to their canonical identifiers.
// Both the canonical name and common aliases are included so users can type
// either "ix" or "exchanges" to filter to the IX section.
var sectionAliases = map[string]string{
	"ix":           "ix",
	"exchanges":    "ix",
	"fac":          "fac",
	"facilities":   "fac",
	"net":          "net",
	"networks":     "net",
	"participants": "net",
	"carrier":      "carrier",
	"carriers":     "carrier",
	"campus":       "campus",
	"campuses":     "campus",
	"contact":      "contact",
	"contacts":     "contact",
	"prefix":       "prefix",
	"prefixes":     "prefix",
}

// ParseSections parses a comma-separated section filter string into a set of
// canonical section names. Returns nil if raw is empty or contains no recognized
// section names (nil means "show all sections").
func ParseSections(raw string) map[string]bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}

	result := make(map[string]bool)
	for _, part := range strings.Split(raw, ",") {
		name := strings.ToLower(strings.TrimSpace(part))
		if canonical, ok := sectionAliases[name]; ok {
			result[canonical] = true
		}
	}

	if len(result) == 0 {
		return nil
	}
	return result
}

// ShouldShowSection reports whether a section should be rendered given the
// active section filter. If sections is nil (no filter), all sections are shown.
func ShouldShowSection(sections map[string]bool, name string) bool {
	if sections == nil {
		return true
	}
	return sections[name]
}
