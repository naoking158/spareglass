// Package doctor reports what this machine supports.
package doctor

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/naoking158/spareglass/internal/config"
	"github.com/naoking158/spareglass/internal/media"
	"github.com/naoking158/spareglass/internal/vdisplay"
)

// Run prints a short diagnostic report.
func Run(w io.Writer, version string, p vdisplay.Provider) error {
	fmt.Fprintf(w, "spareglass %s\n", version)
	fmt.Fprintf(w, "OS/arch:          %s/%s\n", runtime.GOOS, runtime.GOARCH)
	if v := osVersion(); v != "" {
		fmt.Fprintf(w, "OS version:       %s\n", v)
	}
	if runtime.GOOS == "linux" {
		fmt.Fprintf(w, "Session type:     %s\n", os.Getenv("XDG_SESSION_TYPE"))
		fmt.Fprintf(w, "Desktop:          %s\n", os.Getenv("XDG_CURRENT_DESKTOP"))
	}

	fmt.Fprintf(w, "Virtual displays: %s\n", p.Name())
	if err := p.Available(); err != nil {
		fmt.Fprintf(w, "                  NOT AVAILABLE: %v\n", err)
	} else {
		fmt.Fprintln(w, "                  available")
	}
	if d, ok := p.(interface{ Diagnose(io.Writer) }); ok {
		d.Diagnose(w)
	}

	switch ff, err := media.FFmpegPath(); {
	case media.InProcess:
		fmt.Fprintln(w, "Capture/encode:   ScreenCaptureKit + VideoToolbox (in-process)")
		fmt.Fprintln(w, "                  needs Screen Recording permission for this terminal app")
	case err != nil:
		fmt.Fprintf(w, "Capture/encode:   NOT AVAILABLE: %v\n", err)
	case runtime.GOOS == "darwin":
		enc := "libx264 (software; h264_videotoolbox missing)"
		if media.FFmpegHasEncoder("h264_videotoolbox") {
			enc = "h264_videotoolbox (hardware)"
		}
		fmt.Fprintf(w, "Capture/encode:   ffmpeg %s, encoder %s\n", ff, enc)
		fmt.Fprintln(w, "                  needs Screen Recording permission for this terminal app")
	case runtime.GOOS == "windows":
		if media.FFmpegHasEncoder("h264_mf") {
			fmt.Fprintf(w, "Capture/encode:   ffmpeg %s, gdigrab + h264_mf (Media Foundation)\n", ff)
		} else {
			fmt.Fprintf(w, "Capture/encode:   NOT AVAILABLE: ffmpeg %s has no h264_mf encoder\n", ff)
		}
	default:
		fmt.Fprintf(w, "Capture/encode:   NOT AVAILABLE on %s yet (ffmpeg found at %s)\n", runtime.GOOS, ff)
	}

	if p, err := config.Path(); err == nil {
		fmt.Fprintf(w, "Config file:      %s\n", p)
	}
	return nil
}

func osVersion() string {
	var out []byte
	var err error
	switch runtime.GOOS {
	case "darwin":
		out, err = exec.Command("sw_vers", "-productVersion").Output()
	case "linux":
		b, e := os.ReadFile("/etc/os-release")
		if e != nil {
			return ""
		}
		for _, line := range strings.Split(string(b), "\n") {
			if v, ok := strings.CutPrefix(line, "PRETTY_NAME="); ok {
				return strings.Trim(v, `"`)
			}
		}
		return ""
	default:
		return ""
	}
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
