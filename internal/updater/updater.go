// Package updater baixa e instala a última release do Bridge, verificando o
// sha256 publicado antes de trocar o binário. Update fácil na mão
// (`backify-bridge update`); auto-update no heartbeat fica pra depois.
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

// assetName é o nome do binário publicado para este OS/arch.
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
		return "", fmt.Errorf("GitHub API %d ao buscar a última release", resp.StatusCode)
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

// Run instala a última versão se for diferente da atual. Devolve a tag e se
// houve atualização.
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
		return "", false, fmt.Errorf("checksum não confere (esperado %s, veio %s)", want, got)
	}

	// Substitui o próprio binário (no Linux dá pra renomear por cima do que roda).
	self, err := os.Executable()
	if err != nil {
		return "", false, err
	}
	tmp := filepath.Join(filepath.Dir(self), ".backify-bridge.new")
	if err := os.WriteFile(tmp, bin, 0o755); err != nil {
		return "", false, fmt.Errorf("sem permissão pra escrever em %s: %w", filepath.Dir(self), err)
	}
	if err := os.Rename(tmp, self); err != nil {
		_ = os.Remove(tmp)
		return "", false, err
	}
	return tag, true, nil
}
