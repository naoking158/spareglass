//go:build windows

package vdisplay

import (
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/naoking158/spareglass/internal/windisplay"
)

func logMonitors(t *testing.T, label string) {
	list, err := windisplay.List()
	if err != nil {
		t.Logf("%s: list: %v", label, err)
		return
	}
	for _, m := range list {
		t.Logf("%s: %+v", label, m)
	}
}

func TestSpikeOpen(t *testing.T) {
	sizes := os.Getenv("SPIKE_SIZES")
	if sizes == "" {
		t.Skip()
	}
	p := Default()
	logMonitors(t, "before")
	for _, s := range strings.Split(sizes, ",") {
		wh := strings.Split(s, "x")
		w, _ := strconv.Atoi(wh[0])
		h, _ := strconv.Atoi(wh[1])
		d, err := p.Open(Spec{Name: "spareglass 1", Width: w, Height: h, RefreshHz: 60})
		if err != nil {
			t.Logf("RESULT %dx%d: FAIL %v", w, h, err)
			logMonitors(t, "after-fail")
			continue
		}
		t.Logf("RESULT %dx%d: OK %s", w, h, d.ID())
		logMonitors(t, "opened")
		_ = d.Close()
	}
}
