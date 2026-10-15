package updater

import (
	"runtime"
	"strings"
	"testing"
)

func TestAssetNameMatchesReleaseNaming(t *testing.T) {
	got := assetName()
	want := "backify-bridge_" + runtime.GOOS + "_" + runtime.GOARCH
	if got != want {
		t.Fatalf("assetName=%q want=%q (tem que casar com o release.yml)", got, want)
	}
	if !strings.HasPrefix(got, "backify-bridge_") {
		t.Fatal("the asset prefix changed — release.yml and updater would be incompatible")
	}
}

func TestParseSHA(t *testing.T) {
	if got := parseSHA("abc123  backify-bridge_linux_amd64\n"); got != "abc123" {
		t.Fatalf("parseSHA=%q want abc123", got)
	}
	if parseSHA("   ") != "" {
		t.Fatal("linha vazia deveria dar string vazia")
	}
}

// The download is pinned to the tag that was resolved, never latest/download.
func TestAssetURLIsPinnedToTheTag(t *testing.T) {
	got := assetURL("v0.5.0")
	want := "https://github.com/backifyapp/bridge/releases/download/v0.5.0/" + assetName()
	if got != want {
		t.Fatalf("assetURL=%q want=%q", got, want)
	}
	if strings.Contains(got, "latest") {
		t.Fatal("must not use latest/download")
	}
}

func TestSemverAndOlder(t *testing.T) {
	for _, bad := range []string{"dev", "v1.2", "1.2.3", "v1.2.3-rc1", "v1.2.3/../../x", ""} {
		if _, ok := semver(bad); ok {
			t.Errorf("%q devia ser recusada", bad)
		}
	}
	a, _ := semver("v0.4.0")
	b, _ := semver("v0.10.0")
	if !older(a, b) || older(b, a) || older(a, a) {
		t.Fatal("comparação de versões errada (0.4.0 < 0.10.0)")
	}
}
