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
