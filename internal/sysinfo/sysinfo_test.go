package sysinfo

import "testing"

func TestParseMemTotal(t *testing.T) {
	meminfo := "MemTotal:       16333764 kB\nMemFree:  100 kB\n"
	got := parseMemTotal(meminfo)
	want := int64(16333764) * 1024
	if got != want {
		t.Fatalf("memTotal=%d want=%d", got, want)
	}
}

func TestParseMemTotalMissing(t *testing.T) {
	if parseMemTotal("MemFree: 100 kB\n") != 0 {
		t.Fatal("sem MemTotal deveria dar 0")
	}
}

func TestCollectPopulatesOSArch(t *testing.T) {
	i := Collect()
	if i.OS == "" || i.Arch == "" || i.CPUs < 1 {
		t.Fatalf("info incompleta: %+v", i)
	}
}
