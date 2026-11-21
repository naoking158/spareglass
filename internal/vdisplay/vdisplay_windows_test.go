//go:build windows

package vdisplay

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/naoking158/spareglass/internal/media"
	"github.com/naoking158/spareglass/internal/windisplay"
)

// TestVDDDisplayStreams needs the Virtual Display Driver and ffmpeg; CI runs
// it with SPAREGLASS_VDD_TEST=1 after installing the driver. With
// SPAREGLASS_VDD_FRAMES=DIR it also saves each stream there for inspection.
func TestVDDDisplayStreams(t *testing.T) {
	if os.Getenv("SPAREGLASS_VDD_TEST") == "" {
		t.Skip("set SPAREGLASS_VDD_TEST=1 on a machine with the Virtual Display Driver")
	}
	p := Default()
	if err := p.Available(); err != nil {
		t.Fatal(err)
	}
	defer p.CloseAll()

	for i, size := range [][2]int{{1180, 820}, {1366, 1024}, {1180, 820}} {
		d, err := p.Open(Spec{Name: "spareglass 1", Width: size[0], Height: size[1], RefreshHz: 60})
		if err != nil {
			t.Fatalf("Open %v: %v", size, err)
		}
		m, err := windisplay.Find(d.ID())
		if err != nil {
			t.Fatal(err)
		}
		if !m.Attached || m.Width != size[0] || m.Height != size[1] {
			t.Fatalf("monitor after Open %v: %+v", size, m)
		}
		t.Logf("opened %+v", m)
		streamFrames(t, d.ID(), fmt.Sprintf("%d-%dx%d.h264", i+1, size[0], size[1]))
		if err := d.Close(); err != nil {
			t.Fatal(err)
		}
		if m, err := windisplay.Find(d.ID()); err == nil && m.Attached {
			t.Fatalf("monitor still attached after Close: %+v", m)
		}
	}
}

// TestVDDRecoversFromTooManyModes lists more resolutions than the driver's
// monitor survives, as earlier spareglass versions did, and expects Open to
// prune them and bring the monitor back.
func TestVDDRecoversFromTooManyModes(t *testing.T) {
	if os.Getenv("SPAREGLASS_VDD_TEST") == "" {
		t.Skip("set SPAREGLASS_VDD_TEST=1 on a machine with the Virtual Display Driver")
	}
	path := vddSettingsPath()
	orig, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	crowded := string(orig)
	for i := range 8 {
		entry := fmt.Sprintf("%s<resolution><width>%d</width><height>500</height><refresh_rate>60</refresh_rate></resolution>\n</resolutions>", vddMarker, 900+i*10)
		crowded = strings.Replace(crowded, "</resolutions>", entry, 1)
	}
	if err := os.WriteFile(path, []byte(crowded), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := vddCommand("RELOAD_DRIVER"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * time.Second)
	_, err = vddDevice()
	t.Logf("monitor with %d modes listed: %v", vddModes(crowded), err)

	p := Default()
	defer p.CloseAll()
	d, err := p.Open(Spec{Name: "spareglass 1", Width: 1180, Height: 820, RefreshHz: 60})
	if err != nil {
		t.Fatalf("Open after too many modes: %v", err)
	}
	if m, err := windisplay.Find(d.ID()); err != nil || !m.Attached || m.Width != 1180 {
		t.Fatalf("monitor after Open: %+v, %v", m, err)
	}
	_ = d.Close()
}

var errEnough = errors.New("enough")

func streamFrames(t *testing.T, id, name string) {
	t.Helper()
	var out []byte
	pl, err := media.NewPipeline(id, 30, 4000)
	if err != nil {
		t.Fatal(err)
	}
	defer pl.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	n := 0
	err = pl.Run(ctx, func(f media.EncodedFrame) error {
		out = append(out, f.Data...)
		if n++; n >= 90 {
			return errEnough
		}
		return nil
	})
	if !errors.Is(err, errEnough) {
		t.Fatalf("stream from %s: %v after %d frames", id, err, n)
	}
	if dir := os.Getenv("SPAREGLASS_VDD_FRAMES"); dir != "" {
		if err := os.WriteFile(filepath.Join(dir, name), out, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}
