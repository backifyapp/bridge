// Package sysinfo coleta dados do servidor para o inventário de frota no painel:
// OS/arch, kernel, CPUs, RAM e IPs. Barato (só stdlib + /proc no Linux).
package sysinfo

import (
	"net"
	"os"
	"runtime"
	"strconv"
	"strings"
)

// Info é o inventário reportado no heartbeat.
type Info struct {
	OS            string   `json:"os"`
	Arch          string   `json:"arch"`
	Kernel        string   `json:"kernel"`
	CPUs          int      `json:"cpus"`
	MemoryBytes   int64    `json:"memoryBytes"`
	IPs           []string `json:"ips"`
	DockerVersion string   `json:"dockerVersion,omitempty"`
}

// Collect monta o inventário atual (dockerVersion é preenchido à parte pelo agent).
func Collect() Info {
	return Info{
		OS:          runtime.GOOS,
		Arch:        runtime.GOARCH,
		Kernel:      kernel(),
		CPUs:        runtime.NumCPU(),
		MemoryBytes: memTotalBytes(),
		IPs:         localIPs(),
	}
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

// parseMemTotal extrai "MemTotal: N kB" do /proc/meminfo → bytes. Função pura.
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

// localIPs devolve os IPv4 não-loopback das interfaces up.
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
