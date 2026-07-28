package docker

import "testing"

func TestParseVolumeList(t *testing.T) {
	got := parseVolumeList("pgdata\n\n  redisdata \n")
	if len(got) != 2 || got[0].Name != "pgdata" || got[1].Name != "redisdata" {
		t.Fatalf("parse errado: %+v", got)
	}
	if len(parseVolumeList("  \n")) != 0 {
		t.Fatal("vazio deveria dar lista vazia")
	}
}

func TestParseContainerList(t *testing.T) {
	got := parseContainerList("abc123\tweb\tnginx:latest\ndef456\tdb\tpostgres:16\n")
	if len(got) != 2 {
		t.Fatalf("esperava 2, veio %d", len(got))
	}
	if got[0] != (Container{ID: "abc123", Name: "web", Image: "nginx:latest"}) {
		t.Fatalf("container 0 errado: %+v", got[0])
	}
}

func TestExportImportArgs(t *testing.T) {
	exp := exportVolumeArgs("pgdata")
	if exp[0] != "run" || exp[3] != "pgdata:/data:ro" {
		t.Fatalf("export args: %v", exp)
	}
	imp := importVolumeArgs("pgdata")
	if imp[4] != "pgdata:/data" {
		t.Fatalf("import args (deve ser rw): %v", imp)
	}
}

func TestRunContainerArgs(t *testing.T) {
	args := runContainerArgs(RunSpec{
		Name: "web", Image: "nginx:1", RestartPolicy: "unless-stopped",
		Env: []string{"A=1"}, Ports: []string{"8080:80"}, Volumes: []string{"v:/data"},
	})
	joined := ""
	for _, a := range args {
		joined += a + " "
	}
	for _, want := range []string{"run -d", "--name web", "--restart unless-stopped", "-e A=1", "-p 8080:80", "-v v:/data"} {
		if !contains(joined, want) {
			t.Fatalf("faltou %q em: %s", want, joined)
		}
	}
	if args[len(args)-1] != "nginx:1" {
		t.Fatalf("the image must be the last arg: %v", args)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (indexOf(s, sub) >= 0)
}
func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

// `du -sb` devolve "<bytes>\t<path>"; só a 1ª coluna interessa.
func TestParseDuBytes(t *testing.T) {
	cases := map[string]int64{
		"1234\t/data\n":     1234,
		"  99\t/data":       99,
		"0\t/data\n":        0,
		"":                  0,
		"nao-numero\t/data": 0,
	}
	for in, want := range cases {
		if got := parseDuBytes(in); got != want {
			t.Errorf("parseDuBytes(%q) = %d, want %d", in, got, want)
		}
	}
}
