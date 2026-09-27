package main

// uiCSPPolicy builds the browser policy with the validated tile origin.
// The last four directives do not fall back to default-src (base-uri,
// frame-ancestors, form-action) or need a stricter value than it
// (object-src): the UI embeds no plugins, sets no <base>, is never framed
// (X-Frame-Options DENY also says so) and submits forms only to itself.
func uiCSPPolicy(tileSource string) string {
	imageSources := "'self' data:"
	if tileSource != "" {
		imageSources += " " + tileSource
	}

	return "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src " + imageSources + "; connect-src 'self'; font-src 'self'" +
		"; object-src 'none'; base-uri 'self'; frame-ancestors 'none'; form-action 'self'"
}
