//go:build e2e

// Docker acceptance test (real volume roundtrip). Outside the regular `go test`
// run — it needs a host with docker + socket. Run it with:
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
		t.Skip("docker unavailable")
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

	// Export → tar in memory.
	var buf bytes.Buffer
	if err := ExportVolume(ctx, src, &buf); err != nil {
		t.Fatalf("export: %v", err)
	}
	if buf.Len() == 0 {
		t.Fatal("tar exportado vazio")
	}

	// Import into a NEW volume and check the contents.
	if err := ImportVolume(ctx, dst, &buf); err != nil {
		t.Fatalf("import: %v", err)
	}
	out, err := run(ctx, "run", "--rm", "-v", dst+":/data", helperImage, "cat", "/data/x.txt")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "hello-backify") {
		t.Fatalf("restored contents did not match: %q", out)
	}
}
