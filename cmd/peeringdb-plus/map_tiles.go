package main

// uiCSPPolicy builds the browser policy with the validated tile origin.
func uiCSPPolicy(tileSource string) string {
	imageSources := "'self' data:"
	if tileSource != "" {
		imageSources += " " + tileSource
	}
	imageSources += " https://cdn.jsdelivr.net"

	return "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline' https://cdn.jsdelivr.net; img-src " + imageSources + "; connect-src 'self'; font-src 'self' https://cdn.jsdelivr.net"
}
