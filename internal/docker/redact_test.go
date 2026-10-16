package docker

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// O erro de um `docker run` que falha volta ao worker na resposta HTTP e de lá
// ao log do restore que o cliente vê. Até a 0.5.0 ele levava o comando inteiro,
// com `-e MYSQL_ROOT_PASSWORD=<valor>`. O nome fica (diz qual variável era); o
// valor, nunca.
func TestDisplayArgsRedactsEnvValues(t *testing.T) {
	args := runContainerArgs(RunSpec{
		Name:    "db",
		Image:   "mysql:8",
		Env:     []string{"MYSQL_ROOT_PASSWORD=s3cr3t-valor", "TZ=UTC", "SO_O_NOME"},
		Volumes: []string{"dados:/var/lib/mysql"},
	}, false)
	got := displayArgs(append(args, "--env=API_KEY=abc123", "-eTOKEN=xyz789"))
	for _, leak := range []string{"s3cr3t-valor", "UTC", "abc123", "xyz789"} {
		if strings.Contains(got, leak) {
			t.Fatalf("valor %q vazou: %s", leak, got)
		}
	}
	for _, want := range []string{
		"-e MYSQL_ROOT_PASSWORD=***", "-e TZ=***", "-e SO_O_NOME",
		"--env=API_KEY=***", "-eTOKEN=***", "-v dados:/var/lib/mysql", "mysql:8",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("faltou %q em %s", want, got)
		}
	}
}

// Com um `docker` falso que falha e ainda repete o valor na saída (o pior
// caso), o erro de RunContainer não traz o valor por nenhum dos dois caminhos.
func TestRunContainerErrorCarriesNoEnvValue(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("docker falso é um script sh")
	}
	dir := t.TempDir()
	fake := "#!/bin/sh\necho \"docker: Error response from daemon: algo com $*\" 1>&2\nexit 125\n"
	if err := os.WriteFile(filepath.Join(dir, "docker"), []byte(fake), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	_, err := RunContainer(context.Background(), RunSpec{
		Name:  "db",
		Image: "mysql:8",
		Env:   []string{"MYSQL_ROOT_PASSWORD=s3cr3t-valor"},
	}, false)
	if err == nil {
		t.Fatal("esperava erro do docker falso")
	}
	msg := err.Error()
	if strings.Contains(msg, "s3cr3t-valor") {
		t.Fatalf("valor vazou no erro: %s", msg)
	}
	if !strings.Contains(msg, "MYSQL_ROOT_PASSWORD=***") || !strings.Contains(msg, "exit status 125") {
		t.Fatalf("o erro perdeu o diagnóstico: %s", msg)
	}
}
