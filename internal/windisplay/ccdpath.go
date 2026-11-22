package windisplay

import (
	"fmt"
	"strings"
)

// Path is one row of QueryDisplayConfig(QDC_ALL_PATHS): a source of an
// adapter that could drive a target (a monitor connector).
type Path struct {
	SourceAdapter uint64
	SourceID      uint32
	Source        string // GDI name such as \\.\DISPLAY3, "" when Windows gives none
	TargetAdapter uint64
	TargetID      uint32
	Active        bool
	Available     bool   // a monitor is connected to the target
	TargetStatus  uint32 // DISPLAYCONFIG_PATH_TARGET_INFO.statusFlags
}

func (p Path) String() string {
	src := p.Source
	if src == "" {
		src = "?"
	}
	return fmt.Sprintf("%s(adapter %x source %d) -> adapter %x target %d active=%t available=%t status=%#x",
		src, p.SourceAdapter, p.SourceID, p.TargetAdapter, p.TargetID, p.Active, p.Available, p.TargetStatus)
}

type endpoint struct {
	adapter uint64
	id      uint32
}

// pickPath finds the path that shows the target behind the GDI source
// device. The target is any target on the adapter that owns device, so a
// target paired with another source of that adapter is found as well.
// When a target of the adapter is already on the desktop, pickPath returns
// that active path with active set. Otherwise it returns an inactive path,
// preferring an available target and then device's own source.
func pickPath(paths []Path, device string) (i int, active bool, err error) {
	adapters := map[uint64]bool{}
	for _, p := range paths {
		if p.Source == device {
			adapters[p.SourceAdapter] = true
		}
	}
	if len(adapters) == 0 {
		return -1, false, fmt.Errorf("no display path has source %s", device)
	}
	targets := map[endpoint]bool{}
	for _, p := range paths {
		if adapters[p.SourceAdapter] && adapters[p.TargetAdapter] {
			targets[endpoint{p.TargetAdapter, p.TargetID}] = true
		}
	}
	best, bestScore := -1, -1
	for j, p := range paths {
		if !adapters[p.SourceAdapter] || !targets[endpoint{p.TargetAdapter, p.TargetID}] {
			continue
		}
		if p.Active {
			return j, true, nil
		}
		score := 0
		if p.Available {
			score += 2
		}
		if p.Source == device {
			score++
		}
		if score > bestScore {
			best, bestScore = j, score
		}
	}
	if best < 0 {
		return -1, false, fmt.Errorf("the adapter of %s has no target", device)
	}
	return best, false, nil
}

// adapterPaths returns the rows that touch the adapter owning device, or
// every row when no row names device.
func adapterPaths(paths []Path, device string) []Path {
	adapters := map[uint64]bool{}
	for _, p := range paths {
		if p.Source == device {
			adapters[p.SourceAdapter] = true
		}
	}
	if len(adapters) == 0 {
		return paths
	}
	var out []Path
	for _, p := range paths {
		if adapters[p.SourceAdapter] || adapters[p.TargetAdapter] {
			out = append(out, p)
		}
	}
	return out
}

func formatPaths(paths []Path) string {
	rows := make([]string, len(paths))
	for i, p := range paths {
		rows[i] = p.String()
	}
	return "[" + strings.Join(rows, "; ") + "]"
}
