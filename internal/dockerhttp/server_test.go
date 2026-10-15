package dockerhttp

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/backifyapp/bridge/internal/docker"
)

// Names outside Docker's grammar never reach the docker CLI: "%2Fetc" decodes to
// "/etc", which `-v /etc:/data` would mount from the HOST.
func TestRoutesRefuseNamesOutsideDockerGrammar(t *testing.T) {
	h := Handler(secret, nil)
	for _, c := range []struct{ method, target, body string }{
		{"POST", "/docker/volume/%2Fetc/import", "x"},
		{"GET", "/docker/volume/%2Froot/export", ""},
		{"GET", "/docker/volume/%2Fvar%2Flib/size", ""},
		{"GET", "/docker/container/-f=x/inspect", ""},
		{"POST", "/docker/container/--help/pause", ""},
	} {
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, signedReq(c.method, c.target, c.body, now(), freshNonce()))
		if rr.Code != 400 {
			t.Errorf("%s %s: want 400, got %d (%s)", c.method, c.target, rr.Code, rr.Body.String())
		}
	}
}

func TestRunRefusesHostBindMount(t *testing.T) {
	h := Handler(secret, nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, signedReq("POST", "/docker/container/run", `{"name":"x","image":"alpine","volumes":["/:/host"]}`, now(), freshNonce()))
	if rr.Code != 400 {
		t.Fatalf("run spec with a host bind mount: want 400, got %d (%s)", rr.Code, rr.Body.String())
	}
}

// ── With a fake `docker` on PATH ───────────────────────────────────────────
//
// The fake answers like the CLI: `volume inspect` / `image inspect` succeed only
// for FAKE_VOLUMES / FAKE_IMAGES, the export (`run … tar -czf -`) prints
// FAKE_EXPORT_FILE and exits FAKE_EXPORT_EXIT, and `run -d` logs its argv.

const fakeDocker = `#!/bin/sh
echo "$*" >> "$FAKE_LOG"
case "$1 $2" in
"volume inspect") for v in $FAKE_VOLUMES; do [ "$v" = "$5" ] && { echo "$5"; exit 0; }; done
  echo "Error: No such volume: $5" >&2; exit 1 ;;
"image inspect") for i in $FAKE_IMAGES; do [ "$i" = "$5" ] && { echo sha256:feed; exit 0; }; done
  echo "Error: No such image: $5" >&2; exit 1 ;;
"volume create") echo "$3"; exit 0 ;;
"inspect --format") echo "/$4"; exit 0 ;;
esac
case "$*" in
*"-czf -"*) cat "$FAKE_EXPORT_FILE"; echo "tar: ./big.bin: Read error: Input/output error" >&2; exit "$FAKE_EXPORT_EXIT" ;;
"run -d"*) echo 0123456789ab; exit 0 ;;
esac
exit 0
`

type fakeEnv struct {
	log string
	dir string
}

func withFakeDocker(t *testing.T, volumes, images string) *fakeEnv {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "docker"), []byte(fakeDocker), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("FAKE_VOLUMES", volumes)
	t.Setenv("FAKE_IMAGES", images)
	t.Setenv("FAKE_LOG", filepath.Join(dir, "calls.log"))
	return &fakeEnv{log: filepath.Join(dir, "calls.log"), dir: dir}
}

func (f *fakeEnv) calls(t *testing.T) string {
	t.Helper()
	b, _ := os.ReadFile(f.log)
	return string(b)
}

// exportArchive writes the first n bytes of a real tar.gz-like gzip stream
// (n<0 = all of it) as the fake export output.
func (f *fakeEnv) exportArchive(t *testing.T, n int, exit string) {
	t.Helper()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	_, _ = zw.Write(bytes.Repeat([]byte("volume-data;"), 20_000))
	_ = zw.Close()
	data := buf.Bytes()
	if n >= 0 {
		data = data[:n]
	}
	p := filepath.Join(f.dir, "export.bin")
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FAKE_EXPORT_FILE", p)
	t.Setenv("FAKE_EXPORT_EXIT", exit)
}

// The defect: when `docker run … tar` died after the 200, the helper wrote the
// error text into the gzip body and the stream ended cleanly — the worker saw a
// complete export. Now the trailer carries the exit status, over real HTTP.
func TestExportFailureAfterStreamingIsInTheTrailer(t *testing.T) {
	f := withFakeDocker(t, "pgdata", "")
	f.exportArchive(t, 200, "2") // 200 bytes of gzip, then tar fails

	srv := httptest.NewServer(Handler(secret, nil))
	defer srv.Close()
	res := doSigned(t, srv, "GET", "/docker/volume/pgdata/export", "")
	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("status=%d (the 200 was already sent when tar failed)", res.StatusCode)
	}
	if got := res.Trailer.Get(TrailerExitCode); got != "2" {
		t.Fatalf("trailer %s=%q want 2", TrailerExitCode, got)
	}
	if !strings.Contains(res.Trailer.Get(TrailerExitError), "Input/output error") {
		t.Fatalf("trailer %s=%q", TrailerExitError, res.Trailer.Get(TrailerExitError))
	}
	if len(body) != 200 || bytes.Contains(body, []byte("export volume")) {
		t.Fatalf("the error text must not be appended to the archive (len=%d)", len(body))
	}
}

func TestExportSuccessTrailerIsZero(t *testing.T) {
	f := withFakeDocker(t, "pgdata", "")
	f.exportArchive(t, -1, "0")

	srv := httptest.NewServer(Handler(secret, nil))
	defer srv.Close()
	res := doSigned(t, srv, "GET", "/docker/volume/pgdata/export", "")
	zr, err := gzip.NewReader(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(io.Discard, zr); err != nil {
		t.Fatalf("complete export must be a complete gzip: %v", err)
	}
	_, _ = io.Copy(io.Discard, res.Body)
	res.Body.Close()
	if got := res.Trailer.Get(TrailerExitCode); got != "0" {
		t.Fatalf("trailer %s=%q want 0", TrailerExitCode, got)
	}
}

// Export failing before any byte: a plain 500 (and the trailer says so too).
func TestExportFailureBeforeStreamingIs500(t *testing.T) {
	f := withFakeDocker(t, "pgdata", "")
	f.exportArchive(t, 0, "125")
	srv := httptest.NewServer(Handler(secret, nil))
	defer srv.Close()
	res := doSigned(t, srv, "GET", "/docker/volume/pgdata/export", "")
	_, _ = io.Copy(io.Discard, res.Body)
	res.Body.Close()
	if res.StatusCode != 500 {
		t.Fatalf("status=%d want 500", res.StatusCode)
	}
}

// `docker run -v <name>:/data` creates a missing volume: a deleted or mistyped
// volume used to be backed up as an empty archive, reported as a success.
func TestExportAndSizeOfMissingVolumeAre404(t *testing.T) {
	f := withFakeDocker(t, "pgdata", "")
	h := Handler(secret, nil)
	for _, target := range []string{"/docker/volume/sumiu/export", "/docker/volume/sumiu/size"} {
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, signedReq("GET", target, "", now(), freshNonce()))
		if rr.Code != 404 {
			t.Errorf("%s: want 404, got %d", target, rr.Code)
		}
	}
	if strings.Contains(f.calls(t), "run") {
		t.Fatalf("docker run must not be called for a missing volume:\n%s", f.calls(t))
	}
}

// A restore only creates volumes: importing over an existing one (the live
// database's, say) is refused before tar runs.
func TestImportIntoExistingVolumeRefused(t *testing.T) {
	f := withFakeDocker(t, "pgdata", "")
	h := Handler(secret, nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, signedReq("POST", "/docker/volume/pgdata/import", "tar", now(), freshNonce()))
	if rr.Code != 409 {
		t.Fatalf("import into an existing volume: want 409, got %d (%s)", rr.Code, rr.Body.String())
	}
	if strings.Contains(f.calls(t), "-xzf") || strings.Contains(f.calls(t), "volume create") {
		t.Fatalf("nothing may be written:\n%s", f.calls(t))
	}
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, signedReq("POST", "/docker/volume/pgdata-restored-1/import", "tar", now(), freshNonce()))
	if rr.Code != 200 {
		t.Fatalf("import into a new volume: want 200, got %d (%s)", rr.Code, rr.Body.String())
	}
}

// The audit's three examples, each refused by default with no customer config.
func TestRunRefusesWhatASignerShouldNotDo(t *testing.T) {
	f := withFakeDocker(t, "", "postgres:16-alpine")
	h := Handler(secret, nil)
	for name, c := range map[string]struct {
		spec string
		code int
	}{
		"imagem que não está no host": {`{"name":"x","image":"evil.example.com/pwn:latest"}`, 409},
		"porta publicada":             {`{"name":"x","image":"postgres:16-alpine","ports":["0.0.0.0:2222:22"]}`, 400},
		"restart always":              {`{"name":"x","image":"postgres:16-alpine","restartPolicy":"always"}`, 400},
		"rede do host":                {`{"name":"x","image":"postgres:16-alpine","network":"host"}`, 400},
		"privileged":                  {`{"name":"x","image":"postgres:16-alpine","privileged":true}`, 400},
		"capAdd":                      {`{"name":"x","image":"postgres:16-alpine","capAdd":["SYS_ADMIN"]}`, 400},
		"devices":                     {`{"name":"x","image":"postgres:16-alpine","devices":["/dev/sda"]}`, 400},
	} {
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, signedReq("POST", "/docker/container/run", c.spec, now(), freshNonce()))
		if rr.Code != c.code {
			t.Errorf("%s: want %d, got %d (%s)", name, c.code, rr.Code, rr.Body.String())
		}
	}
	if strings.Contains(f.calls(t), "run -d") {
		t.Fatalf("no container may have been started:\n%s", f.calls(t))
	}

	// What a restore actually sends still works — and never pulls.
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, signedReq("POST", "/docker/container/run",
		`{"name":"db-restored-1","image":"postgres:16-alpine","env":["A=1"],"ports":[],"volumes":["pg-restored-1:/var/lib/postgresql/data"],"restartPolicy":""}`,
		now(), freshNonce()))
	if rr.Code != 200 {
		t.Fatalf("legit restore spec: want 200, got %d (%s)", rr.Code, rr.Body.String())
	}
	if !strings.Contains(f.calls(t), "run -d --pull=never --name db-restored-1") {
		t.Fatalf("restore must run with --pull=never:\n%s", f.calls(t))
	}
}

func TestLocalAllowlistIsEnforced(t *testing.T) {
	f := withFakeDocker(t, "app-data other", "ghcr.io/acme/web:1")
	pol := &docker.Policy{
		Volumes:    []string{"app-*"},
		Containers: []string{"web"},
		Images:     []string{"ghcr.io/acme/*"},
	}
	h := Handler(secret, pol)
	for name, c := range map[string]struct {
		method, target, body string
		code                 int
	}{
		"export fora da lista":      {"GET", "/docker/volume/other/export", "", 403},
		"import fora da lista":      {"POST", "/docker/volume/other-restored-1/import", "tar", 403},
		"inspect fora da lista":     {"GET", "/docker/container/db/inspect", "", 403},
		"pause fora da lista":       {"POST", "/docker/container/db/pause", "", 403},
		"imagem fora da lista":      {"POST", "/docker/container/run", `{"name":"x","image":"postgres:16-alpine"}`, 403},
		"monta volume fora":         {"POST", "/docker/container/run", `{"name":"x","image":"ghcr.io/acme/web:1","volumes":["other:/data"]}`, 403},
		"import dentro da lista":    {"POST", "/docker/volume/app-data-restored-1/import", "tar", 200},
		"inspect dentro da lista":   {"GET", "/docker/container/web/inspect", "", 200},
		"imagem listada (com pull)": {"POST", "/docker/container/run", `{"name":"x","image":"ghcr.io/acme/web:1","volumes":["app-data:/data"]}`, 200},
	} {
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, signedReq(c.method, c.target, c.body, now(), freshNonce()))
		if rr.Code != c.code {
			t.Errorf("%s: want %d, got %d (%s)", name, c.code, rr.Code, rr.Body.String())
		}
	}
	if !strings.Contains(f.calls(t), "run -d --pull=missing") {
		t.Fatalf("an allowlisted image may be pulled:\n%s", f.calls(t))
	}
}

func doSigned(t *testing.T, srv *httptest.Server, method, target, body string) *http.Response {
	t.Helper()
	req := signedReq(method, srv.URL+target, body, now(), freshNonce())
	req.RequestURI = ""
	res, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return res
}
