package netcheck

import (
	"net"
	"os/exec"
)

const profilesScript = `[Console]::OutputEncoding = [Text.Encoding]::UTF8; Get-NetConnectionProfile | ForEach-Object { "{0}` + "`t" + `{1}` + "`t" + `{2}" -f $_.InterfaceIndex, $_.NetworkCategory, $_.Name }`

func check() []Problem {
	out, err := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", profilesScript).Output()
	if err != nil {
		return nil
	}
	return publicLANProblems(parseWindowsProfiles(string(out)), lanInterfaces())
}

// lanInterfaces returns the indexes of interfaces with a private IPv4
// address, the ones netutil.LANURLs lists.
func lanInterfaces() map[int]bool {
	ifs, err := net.Interfaces()
	if err != nil {
		return nil
	}
	lan := map[int]bool{}
	for _, ifc := range ifs {
		addrs, err := ifc.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			if ipnet, ok := a.(*net.IPNet); ok {
				if ip := ipnet.IP.To4(); ip != nil && !ip.IsLoopback() && ip.IsPrivate() {
					lan[ifc.Index] = true
				}
			}
		}
	}
	return lan
}
