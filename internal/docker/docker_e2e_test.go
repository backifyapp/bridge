//go:build e2e

// Teste de aceitação do Docker (roundtrip real de volume). Fora do `go test`
// normal — precisa de um host com docker + socket. Rode com:
//
//	go test -tags e2e ./internal/docker
package docker

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestVolumeRoundtripE2E(t *testing.T) {
	ctx := context.Background()
	if !Available(ctx) {
		t.Skip("docker indisponível")
	}

	const src = "backify-e2e-src"
	const dst = "backify-e2e-dst"
	t.Cleanup(func() {
		_, _ = run(ctx, "volume", "rm", "-f", src)
		_, _ = run(ctx, "volume", "rm", "-f", dst)
	})

	// Cria o volume de origem e grava um arquivo conhecido.
	if _, err := run(ctx, "volume", "create", src); err != nil {
		t.Fatal(err)
	}
	if _, err := run(ctx, "run", "--rm", "-v", src+":/data", helperImage, "sh", "-c", "echo hello-backify > /data/x.txt"); err != nil {
		t.Fatal(err)
	}

	// Export → tar em memória.
	var buf bytes.Buffer
	if err := ExportVolume(ctx, src, &buf); err != nil {
		t.Fatalf("export: %v", err)
	}
	if buf.Len() == 0 {
		t.Fatal("tar exportado vazio")
	}

	// Import num volume NOVO e confere o conteúdo.
	if err := ImportVolume(ctx, dst, &buf); err != nil {
		t.Fatalf("import: %v", err)
	}
	out, err := run(ctx, "run", "--rm", "-v", dst+":/data", helperImage, "cat", "/data/x.txt")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "hello-backify") {
		t.Fatalf("conteúdo restaurado não bateu: %q", out)
	}
}
