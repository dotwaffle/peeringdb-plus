package termrender

import (
	"image/color"

	lipgloss "charm.land/lipgloss/v2"
)

// Color constants mapping Tailwind CSS color tiers to ANSI 256-color codes.
// These correspond to the port speed color tiers used in the web UI (D-09).
var (
	// ColorSpeedSub1G maps to Tailwind gray-400 for sub-1G port speeds.
	ColorSpeedSub1G = lipgloss.Color("245")
	// ColorSpeed1G maps to Tailwind neutral-300 for 1G port speeds.
	ColorSpeed1G = lipgloss.Color("250")
	// ColorSpeed10G maps to Tailwind blue-500 for 10G port speeds.
	ColorSpeed10G = lipgloss.Color("33")
	// ColorSpeed100G maps to Tailwind emerald-500 for 100G port speeds.
	ColorSpeed100G = lipgloss.Color("42")
	// ColorSpeed400G maps to Tailwind amber-500 for 400G+ port speeds.
	ColorSpeed400G = lipgloss.Color("214")
)

// General-purpose color constants for terminal text rendering.
var (
	// ColorHeading is emerald for section headings.
	ColorHeading color.Color = lipgloss.Color("42")
	// ColorLabel is gray for field labels.
	ColorLabel color.Color = lipgloss.Color("245")
	// ColorValue is bright white for field values.
	ColorValue color.Color = lipgloss.Color("255")
	// ColorLink is blue for URLs and cross-references.
	ColorLink color.Color = lipgloss.Color("33")
	// ColorError is red for error messages.
	ColorError color.Color = lipgloss.Color("196")
	// ColorWarning is amber for warning messages.
	ColorWarning color.Color = lipgloss.Color("214")
	// ColorSuccess is green/emerald for success messages.
	ColorSuccess color.Color = lipgloss.Color("42")
	// ColorMuted is dim gray for secondary text.
	ColorMuted color.Color = lipgloss.Color("240")
)

// Peering policy color constants.
var (
	// ColorPolicyOpen is green for Open peering policy.
	ColorPolicyOpen color.Color = lipgloss.Color("42")
	// ColorPolicySelective is yellow for Selective peering policy.
	ColorPolicySelective color.Color = lipgloss.Color("214")
	// ColorPolicyRestrictive is red for Restrictive peering policy.
	ColorPolicyRestrictive color.Color = lipgloss.Color("196")
)

// Predefined lipgloss styles for consistent terminal text formatting.
var (
	// StyleHeading renders bold emerald text for section headings.
	StyleHeading = lipgloss.NewStyle().Bold(true).Foreground(ColorHeading)
	// StyleLabel renders gray text for field labels.
	StyleLabel = lipgloss.NewStyle().Foreground(ColorLabel)
	// StyleValue renders bright white text for field values.
	StyleValue = lipgloss.NewStyle().Foreground(ColorValue)
	// StyleMuted renders dim gray text for secondary content.
	StyleMuted = lipgloss.NewStyle().Foreground(ColorMuted)
	// StyleError renders bold red text for error messages.
	StyleError = lipgloss.NewStyle().Bold(true).Foreground(ColorError)
	// StyleLink renders underlined blue text for URLs and cross-references.
	StyleLink = lipgloss.NewStyle().Foreground(ColorLink).Underline(true)
	// StyleBold renders bold text without color.
	StyleBold = lipgloss.NewStyle().Bold(true)
)

// TableBorder returns the appropriate lipgloss border style for a given mode.
// Rich mode uses Unicode box-drawing characters; plain mode uses ASCII (D-10).
func TableBorder(mode RenderMode) lipgloss.Border {
	if mode == ModePlain {
		return lipgloss.ASCIIBorder()
	}
	return lipgloss.NormalBorder()
}
