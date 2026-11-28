package netcheck

import (
	"os/exec"
	"slices"
	"testing"
)

func TestWindowsProfilesParse(t *testing.T) {
	out, err := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", profilesScript).Output()
	if err != nil {
		t.Fatalf("powershell: %v", err)
	}
	ps := parseWindowsProfiles(string(out))
	if len(ps) == 0 {
		t.Fatalf("no profiles parsed from %q", out)
	}
	for _, p := range ps {
		if !slices.Contains([]string{"Public", "Private", "DomainAuthenticated"}, p.Category) {
			t.Errorf("profile %+v: unexpected category in %q", p, out)
		}
	}
	t.Logf("profiles: %+v, LAN interfaces: %v, problems: %+v", ps, lanInterfaces(), check())
}
