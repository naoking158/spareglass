// Package netcheck finds settings on this computer that stop display devices
// from connecting to it.
package netcheck

import (
	"bufio"
	"strconv"
	"strings"
)

// Problem codes. The host page turns each into instructions.
const (
	// MacFirewallBlockAll: the macOS firewall blocks all incoming connections.
	MacFirewallBlockAll = "mac_firewall_block_all"
	// MacFirewallAppBlocked: the macOS firewall blocks spareglass.
	MacFirewallAppBlocked = "mac_firewall_app_blocked"
	// WindowsNetworkPublic: a network that display devices use is Public.
	WindowsNetworkPublic = "windows_network_public"
)

// Problem is a setting that stops display devices from connecting.
type Problem struct {
	Code string `json:"code"`
	// Network is the name of the network the problem is about, if any.
	Network string `json:"network,omitempty"`
}

// Check returns the problems found now. It runs OS commands, so call it off
// the request path.
func Check() []Problem {
	return check()
}

func macFirewallOn(globalState string) bool {
	return strings.Contains(globalState, "State = 1") || strings.Contains(globalState, "State = 2")
}

func macBlockAll(globalState, blockAll string) bool {
	return strings.Contains(globalState, "State = 2") || strings.Contains(blockAll, "set to enabled")
}

func macAppBlocked(appBlocked string) bool {
	return strings.Contains(appBlocked, "is blocked")
}

type windowsProfile struct {
	Index    int
	Category string
	Name     string
}

// parseWindowsProfiles reads "index<TAB>category<TAB>name" lines.
func parseWindowsProfiles(out string) []windowsProfile {
	var ps []windowsProfile
	sc := bufio.NewScanner(strings.NewReader(out))
	for sc.Scan() {
		f := strings.SplitN(strings.TrimRight(sc.Text(), "\r"), "\t", 3)
		if len(f) != 3 {
			continue
		}
		i, err := strconv.Atoi(f[0])
		if err != nil {
			continue
		}
		ps = append(ps, windowsProfile{Index: i, Category: f[1], Name: f[2]})
	}
	return ps
}

// publicLANProblems reports each Public profile on an interface that serves
// a LAN address.
func publicLANProblems(ps []windowsProfile, lan map[int]bool) []Problem {
	var out []Problem
	for _, p := range ps {
		if p.Category == "Public" && lan[p.Index] {
			out = append(out, Problem{Code: WindowsNetworkPublic, Network: p.Name})
		}
	}
	return out
}
