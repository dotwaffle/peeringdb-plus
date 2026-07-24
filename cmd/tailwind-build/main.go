// Command tailwind-build compiles the web UI's static Tailwind CSS
// using the Tailwind standalone CLI — a self-contained binary from the
// tailwindlabs release page. Nobody (contributor or CI) needs Node.js
// or npm: the wrapper downloads the pinned CLI release for the host
// platform into the user cache directory, verifies its sha256, and
// runs it.
//
// Invoked by `go generate` from internal/web (see static.go):
//
//	go run github.com/dotwaffle/peeringdb-plus/cmd/tailwind-build \
//	    -in tailwind.input.css -out static/tailwind.css
//
// The output is committed like the *_templ.go files; the CI drift gate
// re-runs generation and fails on any difference, so a stale
// tailwind.css cannot ship silently.
//
// Upgrading Tailwind: bump cliVersion, refresh cliSHA256 from the
// release's sha256sums.txt, run `go generate ./...`, commit the CSS
// churn alongside.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
)

// cliVersion is the pinned Tailwind CSS standalone CLI release.
const cliVersion = "v4.3.3"

// cliSHA256 maps GOOS/GOARCH to the release asset name and its sha256
// from the release's sha256sums.txt. Platforms not listed here (e.g.
// Windows) fail fast with an actionable message rather than running an
// unverified binary.
var cliSHA256 = map[string]struct {
	asset  string
	sha256 string
}{
	"linux/amd64":  {"tailwindcss-linux-x64", "dc61b3ac6b8c9ca874c0cc4c57b2409791a64c5540404ca5f5367360babc313a"},
	"linux/arm64":  {"tailwindcss-linux-arm64", "55fd0b241214eff3de1e8ee4f22796662f2d2e7a49bcfca7477cfd0bac398195"},
	"darwin/amd64": {"tailwindcss-macos-x64", "7922e0953f2110c05976e3bf58f14e643d90427575e766b7d433f5f80cbee7e1"},
	"darwin/arm64": {"tailwindcss-macos-arm64", "cdf646702987a743464dff4d9c60fd4480d1c1e73dd819a9a67f1078815dce9d"},
}

func main() {
	in := flag.String("in", "", "input CSS file (with @import \"tailwindcss\" and @source directives)")
	out := flag.String("out", "", "output CSS file")
	flag.Parse()
	if *in == "" || *out == "" {
		fmt.Fprintln(os.Stderr, "tailwind-build: -in and -out are required")
		os.Exit(2)
	}

	bin, err := ensureCLI()
	if err != nil {
		fmt.Fprintf(os.Stderr, "tailwind-build: %v\n", err)
		os.Exit(1)
	}

	// bin comes from ensureCLI: a sha256-verified binary in our own
	// cache dir, not attacker-influenced input.
	cmd := exec.Command(bin, "--input", *in, "--output", *out, "--minify") //nolint:gosec // path is the verified cached CLI
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "tailwind-build: %s: %v\n", bin, err)
		os.Exit(1)
	}
}

// ensureCLI returns the path to a verified Tailwind CLI binary for the
// host platform, downloading it into the user cache dir on first use.
func ensureCLI() (string, error) {
	plat := runtime.GOOS + "/" + runtime.GOARCH
	rel, ok := cliSHA256[plat]
	if !ok {
		return "", fmt.Errorf("no pinned Tailwind CLI for %s — add the asset+sha256 for this platform to cmd/tailwind-build (from the %s release's sha256sums.txt)", plat, cliVersion)
	}

	cacheRoot, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("resolve user cache dir: %w", err)
	}
	dir := filepath.Join(cacheRoot, "peeringdb-plus")
	bin := filepath.Join(dir, rel.asset+"-"+cliVersion)

	if ok, err := verifyFile(bin, rel.sha256); err == nil && ok {
		return bin, nil
	}

	if err := os.MkdirAll(dir, 0o750); err != nil {
		return "", fmt.Errorf("create cache dir %s: %w", dir, err)
	}
	url := "https://github.com/tailwindlabs/tailwindcss/releases/download/" + cliVersion + "/" + rel.asset
	fmt.Fprintf(os.Stderr, "tailwind-build: downloading %s\n", url)
	tmp, err := download(url, dir)
	if err != nil {
		return "", err
	}
	defer os.Remove(tmp) // no-op after successful rename

	if ok, err := verifyFile(tmp, rel.sha256); err != nil {
		return "", err
	} else if !ok {
		return "", fmt.Errorf("sha256 mismatch for %s — release asset changed or download corrupted; refusing to run it", url)
	}
	if err := os.Chmod(tmp, 0o700); err != nil { //nolint:gosec // it is a binary we are about to execute; owner-only
		return "", fmt.Errorf("chmod %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, bin); err != nil {
		return "", fmt.Errorf("install %s: %w", bin, err)
	}
	return bin, nil
}

// download fetches url into a temp file inside dir and returns its path.
func download(url, dir string) (string, error) {
	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Get(url) //nolint:noctx // one-shot codegen tool; the client timeout bounds it
	if err != nil {
		return "", fmt.Errorf("download %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download %s: HTTP %d", url, resp.StatusCode)
	}
	f, err := os.CreateTemp(dir, "tailwindcss-*.partial")
	if err != nil {
		return "", fmt.Errorf("create temp file: %w", err)
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		_ = f.Close()
		_ = os.Remove(f.Name())
		return "", fmt.Errorf("write %s: %w", f.Name(), err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(f.Name())
		return "", fmt.Errorf("close %s: %w", f.Name(), err)
	}
	return f.Name(), nil
}

// verifyFile reports whether path exists and hashes to wantHex.
func verifyFile(path, wantHex string) (bool, error) {
	f, err := os.Open(path) //nolint:gosec // path is built from our own cache dir + pinned asset name
	if err != nil {
		return false, nil // treat missing/unreadable as "not verified"
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return false, fmt.Errorf("hash %s: %w", path, err)
	}
	return hex.EncodeToString(h.Sum(nil)) == wantHex, nil
}
