//go:build linux

package agent

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	psutilNet "github.com/shirou/gopsutil/v4/net"
)

// collectHardwareDetails fills in the hardware inventory fields of Details on
// a best-effort basis. Every source is optional; failures leave fields empty.
func (a *Agent) collectHardwareDetails() {
	d := &a.systemDetails

	// BIOS version from DMI sysfs (no root needed on most kernels)
	d.BiosVersion = readSysfsTrimmed("/sys/class/dmi/id/bios_version")

	// network interfaces: reuse the agent's already-filtered set so virtual
	// interfaces (docker/veth/lo/...) are excluded from the count
	ifaces, err := psutilNet.Interfaces()
	if err == nil {
		var ips []string
		var maxSpeed uint64
		count := 0
		for _, iface := range ifaces {
			if _, ok := a.netInterfaces[iface.Name]; !ok {
				continue
			}
			count++
			for _, addr := range iface.Addrs {
				// keep only global addresses; strip CIDR suffix and skip
				// loopback / IPv6 link-local (fe80::) which add noise
				ip := addr.Addr
				if idx := strings.Index(ip, "/"); idx >= 0 {
					ip = ip[:idx]
				}
				if ip == "" || strings.HasPrefix(ip, "127.") || ip == "::1" || strings.HasPrefix(ip, "fe80:") {
					continue
				}
				ips = append(ips, ip)
			}
			if speed := nicSpeedMbps(iface.Name); speed > maxSpeed {
				maxSpeed = speed
			}
		}
		d.NicCount = count
		d.NicSpeedMbps = maxSpeed
		// cap the list so the details payload stays small
		if len(ips) > 4 {
			ips = ips[:4]
		}
		d.IpAddrs = strings.Join(ips, ", ")
	}

	// BMC version requires IPMI; best-effort via ipmitool if present
	d.BmcVersion = readBmcVersion()

	// recent IPMI System Event Log entries (hardware alerts)
	d.SelEntries = readSelEntries()
}

// readSelEntries returns the newest IPMI SEL entries, one per line, capped so
// the details payload stays small. Returns "" when ipmitool is unavailable or
// IPMI is not accessible.
func readSelEntries() string {
	out, err := runCommandOutput("ipmitool", "sel", "elist", "last", "10")
	if err != nil {
		return ""
	}
	lines := make([]string, 0, 10)
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, "\n")
}

func readSysfsTrimmed(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// nicSpeedMbps reads the nominal link speed of a NIC from sysfs.
func nicSpeedMbps(name string) uint64 {
	s := readSysfsTrimmed(filepath.Join("/sys/class/net", name, "speed"))
	v, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return 0
	}
	return v
}

// readBmcVersion tries `ipmitool mc info` for the BMC firmware version.
// Returns "" when ipmitool is missing or IPMI is not accessible.
func readBmcVersion() string {
	out, err := runCommandOutput("ipmitool", "mc", "info")
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "Firmware Revision") {
			if idx := strings.Index(line, ":"); idx >= 0 {
				return strings.TrimSpace(line[idx+1:])
			}
		}
	}
	return ""
}
