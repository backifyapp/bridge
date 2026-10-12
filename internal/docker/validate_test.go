package docker

import "testing"

func TestValidName(t *testing.T) {
	for _, ok := range []string{"pgdata", "my_vol.1", "a-b", "3f9c2e1d0a"} {
		if !ValidName(ok) {
			t.Errorf("%q devia ser válido", ok)
		}
	}
	for _, bad := range []string{"", "/etc", "../x", "-f={{json .}}", "a/b", ".hidden", "a b", "x\x00"} {
		if ValidName(bad) {
			t.Errorf("%q devia ser recusado", bad)
		}
	}
}

func TestValidateRunSpec(t *testing.T) {
	good := RunSpec{
		Name: "app-restored", Image: "postgres:16-alpine", Env: []string{"POSTGRES_PASSWORD=x=y"},
		Ports:         []string{"5432:5432", "127.0.0.1:8080:80/tcp"},
		Volumes:       []string{"pgdata:/var/lib/postgresql/data", "cfg:/etc/app:ro"},
		RestartPolicy: "unless-stopped",
	}
	if err := ValidateRunSpec(good); err != nil {
		t.Fatalf("spec legítima recusada: %v", err)
	}
	bad := map[string]RunSpec{
		"bind mount do / do host": {Image: "alpine", Volumes: []string{"/:/host"}},
		"bind mount relativo":     {Image: "alpine", Volumes: []string{"./x:/x"}},
		"imagem vira flag":        {Image: "--privileged"},
		"nome vira flag":          {Name: "--privileged", Image: "alpine"},
		"env vira flag":           {Image: "alpine", Env: []string{"--privileged"}},
		"porta vira flag":         {Image: "alpine", Ports: []string{"--network=host"}},
		"restart desconhecido":    {Image: "alpine", RestartPolicy: "--privileged"},
		"modo de volume estranho": {Image: "alpine", Volumes: []string{"v:/x:z"}},
	}
	for name, s := range bad {
		if ValidateRunSpec(s) == nil {
			t.Errorf("%s: devia ser recusado", name)
		}
	}
}
