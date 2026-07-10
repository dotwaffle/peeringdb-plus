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
const cliVersion = "v4.3.2"

// cliSHA256 maps GOOS/GOARCH to the release asset name and its sha256
// from the release's sha256sums.txt. Platforms not listed here (e.g.
// Windows) fail fast with an actionable message rather than running an
// unverified binary.
var cliSHA256 = map[string]struct {
	asset  string
	sha256 string
}{
	"linux/amd64":  {"tailwindcss-linux-x64", "5036c4fb4328e0bcdbb6065c70d8ac9452e0d4c947113a788a8f94fd390425c1"},
	"linux/arm64":  {"tailwindcss-linux-arm64", "394ddccc2402cfa3abd97dfba56f3587781a3d6e6ce66e65ceada14beb7664b8"},
	"darwin/amd64": {"tailwindcss-macos-x64", "cef8f110471e889c3c4409055cf8aff33076f58a081867b0dfc6534b290bfbb0"},
	"darwin/arm64": {"tailwindcss-macos-arm64", "b800b0659dc64b9f03ede5660244d9415d777d5739ae2889280877ca37be742a"},
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

	cmd := exec.Command(bin, "--input", *in, "--output", *out, "--minify")
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

	if err := os.MkdirAll(dir, 0o755); err != nil {
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
	if err := os.Chmod(tmp, 0o755); err != nil {
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
		f.Close()
		os.Remove(f.Name())
		return "", fmt.Errorf("write %s: %w", f.Name(), err)
	}
	if err := f.Close(); err != nil {
		os.Remove(f.Name())
		return "", fmt.Errorf("close %s: %w", f.Name(), err)
	}
	return f.Name(), nil
}

// verifyFile reports whether path exists and hashes to wantHex.
func verifyFile(path, wantHex string) (bool, error) {
	f, err := os.Open(path)
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
