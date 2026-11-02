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

func TestParseOSRelease(t *testing.T) {
	cases := []struct {
		in                    string
		name, version, pretty string
	}{
		{
			"NAME=\"CloudLinux\"\nVERSION=\"9.6 (Sapphire Caracal)\"\nID=\"cloudlinux\"\nVERSION_ID=\"9.6\"\nPRETTY_NAME=\"CloudLinux 9.6 (Sapphire Caracal)\"\n",
			"CloudLinux", "9.6", "CloudLinux 9.6 (Sapphire Caracal)",
		},
		{
			"PRETTY_NAME=\"Ubuntu 24.04.1 LTS\"\nNAME=\"Ubuntu\"\nVERSION_ID=\"24.04\"\n",
			"Ubuntu", "24.04", "Ubuntu 24.04.1 LTS",
		},
		// Bare and single-quoted values; comments and blank lines ignored.
		{"# comment\n\nNAME=Alpine Linux\nVERSION_ID=3.22.2\nPRETTY_NAME='Alpine Linux v3.22'\n", "Alpine Linux", "3.22.2", "Alpine Linux v3.22"},
		// Debian testing has no VERSION_ID.
		{"PRETTY_NAME=\"Debian GNU/Linux forky/sid\"\nNAME=\"Debian GNU/Linux\"\n", "Debian GNU/Linux", "", "Debian GNU/Linux forky/sid"},
		{"", "", "", ""},
	}
	for _, c := range cases {
		n, v, p := parseOSRelease(c.in)
		if n != c.name || v != c.version || p != c.pretty {
			t.Errorf("parseOSRelease(%q) = %q, %q, %q; want %q, %q, %q", c.in, n, v, p, c.name, c.version, c.pretty)
		}
	}
}
