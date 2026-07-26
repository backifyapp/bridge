package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveLoadRoundtripAndPerms(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "sub", "bridge.json")
	in := &Config{APIURL: "https://api.backify.app", AgentID: "ag1", HMACSecret: "sek"}

	if err := Save(p, in); err != nil {
		t.Fatal(err)
	}

	fi, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if perm := fi.Mode().Perm(); perm != 0o600 {
		t.Fatalf("perm=%o want 600 (segredo em repouso)", perm)
	}

	out, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if *out != *in {
		t.Fatalf("roundtrip diff: %+v vs %+v", out, in)
	}
	if !out.Enrolled() {
		t.Fatal("deveria estar enrolled")
	}
}

func TestEnrolledFalseWhenIncomplete(t *testing.T) {
	if (&Config{AgentID: "x"}).Enrolled() {
		t.Fatal("sem segredo não deveria ser enrolled")
	}
}
