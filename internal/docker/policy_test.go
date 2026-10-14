package docker

import (
	"encoding/json"
	"testing"
)

func TestNilPolicyAllowsEverything(t *testing.T) {
	var p *Policy
	if !p.AllowVolume("x") || !p.AllowContainer("x") || !p.AllowImage("evil.example.com/pwn:latest") || p.RestrictsImages() {
		t.Fatal("no policy file section must mean no extra restriction")
	}
	if err := p.Validate(); err != nil {
		t.Fatal(err)
	}
}

// A missing list does not restrict; an empty list allows nothing.
func TestPolicyListsAreEnforcedOnlyWhenPresent(t *testing.T) {
	var p Policy
	if err := json.Unmarshal([]byte(`{"volumes":["app-*","db"],"containers":[]}`), &p); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]bool{"app-data": true, "app-data-restored-20261014": true, "db": true, "dbx": false, "other": false} {
		if got := p.AllowVolume(name); got != want {
			t.Errorf("AllowVolume(%q)=%v want %v", name, got, want)
		}
	}
	if p.AllowContainer("web") {
		t.Error(`"containers": [] must allow nothing`)
	}
	if !p.AllowImage("anything") || p.RestrictsImages() {
		t.Error("no images list: images are not restricted by the allowlist")
	}
}

func TestPolicyContainerMatchesAnyName(t *testing.T) {
	p := &Policy{Containers: []string{"web"}}
	if !p.AllowContainer("3f9c2e1d0a", "web") {
		t.Fatal("an id whose resolved name is allowed must pass")
	}
	if p.AllowContainer("3f9c2e1d0a", "") {
		t.Fatal("an unresolved id must not pass")
	}
}

func TestPolicyImages(t *testing.T) {
	p := &Policy{Images: []string{"postgres:16-alpine", "ghcr.io/acme/*"}}
	if !p.RestrictsImages() || !p.AllowImage("postgres:16-alpine") || !p.AllowImage("ghcr.io/acme/web:1") {
		t.Fatal("listed images must pass")
	}
	if p.AllowImage("evil.example.com/pwn:latest") || p.AllowImage("postgres:latest") {
		t.Fatal("unlisted images must be refused")
	}
}

func TestPolicyValidateRefusesBadGlobs(t *testing.T) {
	if err := (&Policy{Volumes: []string{"app-["}}).Validate(); err == nil {
		t.Fatal("malformed glob must fail validation")
	}
	if err := (&Policy{Images: []string{""}}).Validate(); err == nil {
		t.Fatal("empty entry must fail validation")
	}
}
