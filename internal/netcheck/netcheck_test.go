package netcheck

import (
	"reflect"
	"testing"
)

func TestMacFirewall(t *testing.T) {
	cases := []struct {
		name                      string
		global, blockAll, app     string
		on, wantBlockAll, wantApp bool
	}{
		{"off", "Firewall is disabled. (State = 0)\n", "", "", false, false, false},
		{"on, allows", "Firewall is enabled. (State = 1)\n", "Firewall has block all state set to disabled.\n", "Incoming connection to /x/spareglass is permitted.\n", true, false, false},
		{"block all", "Firewall is enabled. (State = 1)\n", "Firewall has block all state set to enabled.\n", "", true, true, false},
		{"block all, older state", "Firewall is blocking all non-essential incoming connections. (State = 2)\n", "", "", true, true, false},
		{"app blocked", "Firewall is enabled. (State = 1)\n", "Firewall has block all state set to disabled.\n", "Incoming connection to /x/spareglass is blocked.\n", true, false, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := macFirewallOn(c.global); got != c.on {
				t.Errorf("macFirewallOn = %v, want %v", got, c.on)
			}
			if got := macBlockAll(c.global, c.blockAll); got != c.wantBlockAll {
				t.Errorf("macBlockAll = %v, want %v", got, c.wantBlockAll)
			}
			if got := macAppBlocked(c.app); got != c.wantApp {
				t.Errorf("macAppBlocked = %v, want %v", got, c.wantApp)
			}
		})
	}
}

func TestPublicLANProblems(t *testing.T) {
	out := "12\tPublic\tPixel 9\r\n7\tPrivate\tHome Wi-Fi\r\n30\tPublic\tVPN\r\n9\tDomainAuthenticated\tcorp.example\r\n"
	got := publicLANProblems(parseWindowsProfiles(out), map[int]bool{12: true, 7: true, 9: true})
	want := []Problem{{Code: WindowsNetworkPublic, Network: "Pixel 9"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}
