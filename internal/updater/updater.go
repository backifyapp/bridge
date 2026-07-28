// Package updater downloads and installs the latest Bridge release, verifying
// the published sha256 before swapping the binary. Easy manual update
// (`backify-bridge update`); auto-update on heartbeat comes later.
package updater

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const repo = "backifyapp/bridge"

var httpClient = &http.Client{Timeout: 60 * time.Second}

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
	return io.ReadAll(resp.Body)
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

	base := "https://github.com/" + repo + "/releases/latest/download/" + assetName()
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
