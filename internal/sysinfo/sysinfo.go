// Package sysinfo collects server data for the fleet inventory in the dashboard:
// OS/arch, distribution and its version, kernel, CPUs, RAM and IPs. Cheap
// (stdlib only + /proc and os-release on Linux).
package sysinfo

import (
	"net"
	"os"
	"runtime"
	"strconv"
	"strings"
)

// Info is the inventory reported in the heartbeat.
type Info struct {
	OS   string `json:"os"`
	Arch string `json:"arch"`
	// Distribution, from os-release: "CloudLinux", "9.6", "CloudLinux 9.6
	// (Sapphire Caracal)". Empty when unknown — never the image's own OS when
	// running in a container (see InContainer).
	OSName        string   `json:"osName,omitempty"`
	OSVersion     string   `json:"osVersion,omitempty"`
	OSPrettyName  string   `json:"osPrettyName,omitempty"`
	Kernel        string   `json:"kernel"`
	CPUs          int      `json:"cpus"`
	MemoryBytes   int64    `json:"memoryBytes"`
	IPs           []string `json:"ips"`
	DockerVersion string   `json:"dockerVersion,omitempty"`
}

// Collect builds the current inventory (dockerVersion is filled separately by the agent).
// Inside a container the distribution is left empty: /etc/os-release there is
// the image's (Alpine), not the host's — the agent asks the host's Docker.
func Collect() Info {
	i := Info{
		OS:          runtime.GOOS,
		Arch:        runtime.GOARCH,
		Kernel:      kernel(),
		CPUs:        runtime.NumCPU(),
		MemoryBytes: memTotalBytes(),
		IPs:         localIPs(),
	}
	if !InContainer() {
		i.OSName, i.OSVersion, i.OSPrettyName = hostOSRelease()
	}
	return i
}

// InContainer reports whether the agent runs inside a Docker or Podman
// container (the official image), where os-release describes the image.
func InContainer() bool {
	for _, f := range []string{"/.dockerenv", "/run/.containerenv"} {
		if _, err := os.Stat(f); err == nil {
			return true
		}
	}
	return false
}

func hostOSRelease() (name, version, pretty string) {
	// /etc/os-release is the standard location; /usr/lib/os-release the
	// fallback the spec defines for systems without the first.
	for _, f := range []string{"/etc/os-release", "/usr/lib/os-release"} {
		if b, err := os.ReadFile(f); err == nil {
			return parseOSRelease(string(b))
		}
	}
	return "", "", ""
}

// parseOSRelease extracts NAME, VERSION_ID and PRETTY_NAME from an os-release
// file. Values may be bare or quoted. Pure function.
func parseOSRelease(content string) (name, version, pretty string) {
	for _, line := range strings.Split(content, "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok || strings.HasPrefix(key, "#") {
			continue
		}
		if len(value) >= 2 && (value[0] == '"' || value[0] == '\'') && value[len(value)-1] == value[0] {
			value = value[1 : len(value)-1]
		}
		switch key {
		case "NAME":
			name = value
		case "VERSION_ID":
			version = value
		case "PRETTY_NAME":
			pretty = value
		}
	}
	return name, version, pretty
}

func kernel() string {
	b, err := os.ReadFile("/proc/sys/kernel/osrelease")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func memTotalBytes() int64 {
	b, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0
	}
	return parseMemTotal(string(b))
}

// parseMemTotal extracts "MemTotal: N kB" from /proc/meminfo → bytes. Pure function.
func parseMemTotal(meminfo string) int64 {
	for _, line := range strings.Split(meminfo, "\n") {
		if !strings.HasPrefix(line, "MemTotal:") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			if kb, err := strconv.ParseInt(fields[1], 10, 64); err == nil {
				return kb * 1024
			}
		}
	}
	return 0
}

// localIPs returns the non-loopback IPv4 addresses of the interfaces that are up.
func localIPs() []string {
	ips := []string{}
	ifaces, err := net.Interfaces()
	if err != nil {
		return ips
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			if ipnet, ok := addr.(*net.IPNet); ok {
				if v4 := ipnet.IP.To4(); v4 != nil {
					ips = append(ips, v4.String())
				}
			}
		}
	}
	return ips
}
