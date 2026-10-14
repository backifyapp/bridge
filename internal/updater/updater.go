// Package updater downloads and installs the latest Bridge release, verifying
// the published sha256 before swapping the binary. Easy manual update
// (`backify-bridge update`); auto-update on heartbeat comes later.
//
// What the checksum does and does not prove: the binary and its .sha256 come
// from the same GitHub release, so the checksum only catches a corrupted
// download — whoever can publish a release can publish both. Authenticity
// needs a signature checked against a key embedded in this binary (minisign or
// cosign; see SECURITY.md → "Update authenticity"). Until then, the updater at
// least downloads from the EXACT tag it resolved (never "latest/download",
// which could move between the two requests), accepts only vX.Y.Z tags and
// refuses to go back to an older version.
package updater

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const repo = "backifyapp/bridge"

var httpClient = &http.Client{Timeout: 60 * time.Second}

// maxAssetBytes caps a download (the binary is ~15 MB).
const maxAssetBytes = 200 << 20

var tagRe = regexp.MustCompile(`^v(\d{1,4})\.(\d{1,4})\.(\d{1,4})$`)

// assetURL is the download URL of this platform's binary IN THE GIVEN TAG.
func assetURL(tag string) string {
	return "https://github.com/" + repo + "/releases/download/" + tag + "/" + assetName()
}

// semver parses "vX.Y.Z" (ok=false otherwise, e.g. "dev").
func semver(tag string) ([3]int, bool) {
	m := tagRe.FindStringSubmatch(tag)
	if m == nil {
		return [3]int{}, false
	}
	var v [3]int
	for i := range v {
		v[i], _ = strconv.Atoi(m[i+1])
	}
	return v, true
}

// older reports whether a < b (both valid semver tags).
func older(a, b [3]int) bool {
	for i := range a {
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return false
}

// assetName is the name of the binary published for this OS/arch.
func assetName() string {
	return fmt.Sprintf("backify-bridge_%s_%s", runtime.GOOS, runtime.GOARCH)
}

// parseSHA extrai o hash da 1ª coluna do arquivo .sha256 (`<hash>  <arquivo>`).
func parseSHA(raw string) string {
	fields := strings.Fields(strings.TrimSpace(raw))
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

func latestTag() (string, error) {
	req, _ := http.NewRequest("GET", "https://api.github.com/repos/"+repo+"/releases/latest", nil)
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("GitHub API %d while fetching the latest release", resp.StatusCode)
	}
	var r struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return "", err
	}
	if r.TagName == "" {
		return "", fmt.Errorf("nenhuma release encontrada")
	}
	if _, ok := semver(r.TagName); !ok {
		return "", fmt.Errorf("tag de release inesperada %q (esperado vX.Y.Z)", r.TagName)
	}
	return r.TagName, nil
}

func download(url string) ([]byte, error) {
	resp, err := httpClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("download %s: HTTP %d", url, resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxAssetBytes+1))
	if err != nil {
		return nil, err
	}
	if len(b) > maxAssetBytes {
		return nil, fmt.Errorf("download %s: maior que %d bytes", url, maxAssetBytes)
	}
	return b, nil
}

// Run installs the latest version if it differs from the current one. It returns
// the tag and whether an update happened.
func Run(currentVersion string) (tag string, updated bool, err error) {
	tag, err = latestTag()
	if err != nil {
		return "", false, err
	}
	if tag == currentVersion {
		return tag, false, nil
	}
	if cur, ok := semver(currentVersion); ok {
		if next, _ := semver(tag); older(next, cur) {
			return "", false, fmt.Errorf("a release mais recente (%s) é anterior à instalada (%s): recusando o downgrade", tag, currentVersion)
		}
	}

	// From the tag just resolved — "latest/download" could point to another
	// release by the time the second request is made.
	base := assetURL(tag)
	bin, err := download(base)
	if err != nil {
		return "", false, err
	}
	shaRaw, err := download(base + ".sha256")
	if err != nil {
		return "", false, err
	}
	want := parseSHA(string(shaRaw))
	got := fmt.Sprintf("%x", sha256.Sum256(bin))
	if want == "" || got != want {
		return "", false, fmt.Errorf("checksum mismatch (expected %s, got %s)", want, got)
	}

	// Replaces its own binary (on Linux you can rename over a running executable).
	self, err := os.Executable()
	if err != nil {
		return "", false, err
	}
	tmp := filepath.Join(filepath.Dir(self), ".backify-bridge.new")
	if err := os.WriteFile(tmp, bin, 0o755); err != nil {
		return "", false, fmt.Errorf("no permission to write to %s: %w", filepath.Dir(self), err)
	}
	if err := os.Rename(tmp, self); err != nil {
		_ = os.Remove(tmp)
		return "", false, err
	}
	return tag, true, nil
}
