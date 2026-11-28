package netcheck

import (
	"os"
	"os/exec"
	"path/filepath"
)

const socketfilterfw = "/usr/libexec/ApplicationFirewall/socketfilterfw"

func check() []Problem {
	global, err := exec.Command(socketfilterfw, "--getglobalstate").Output()
	if err != nil || !macFirewallOn(string(global)) {
		return nil
	}
	blockAll, _ := exec.Command(socketfilterfw, "--getblockall").Output()
	if macBlockAll(string(global), string(blockAll)) {
		return []Problem{{Code: MacFirewallBlockAll}}
	}
	exe, err := os.Executable()
	if err != nil {
		return nil
	}
	if p, err := filepath.EvalSymlinks(exe); err == nil {
		exe = p
	}
	app, _ := exec.Command(socketfilterfw, "--getappblocked", exe).Output()
	if macAppBlocked(string(app)) {
		return []Problem{{Code: MacFirewallAppBlocked}}
	}
	return nil
}
