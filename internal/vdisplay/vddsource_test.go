package vdisplay

import (
	"errors"
	"testing"

	"github.com/naoking158/spareglass/internal/windisplay"
)

// noMonitorList and noMonitorPaths are a Windows machine where the driver's
// source exists and still lists modes, but has no monitor and no display path.
var noMonitorList = []windisplay.Monitor{
	{Device: `\\.\DISPLAY1`, Description: "AMD Radeon(TM) Graphics", Attached: true, Primary: true, Flags: 0x5, MonitorID: `MONITOR\GSM5B7F\{4d36e96e-e325-11ce-bfc1-08002be10318}\0000`, Width: 1920, Height: 1080},
	{Device: `\\.\DISPLAY2`, Description: "AMD Radeon(TM) Graphics"},
	{Device: `\\.\DISPLAY6`, Description: vddAdapter},
}

var noMonitorPaths = []windisplay.Path{
	{SourceAdapter: 0x1806b, Source: `\\.\DISPLAY1`, TargetAdapter: 0x1806b, TargetID: 256, Active: true, Available: true, TargetStatus: 1},
	{SourceAdapter: 0x1806b, Source: `\\.\DISPLAY1`, TargetAdapter: 0x1806b, TargetID: 50331904, Active: true, Available: true, TargetStatus: 1},
	{SourceAdapter: 0x1806b, SourceID: 1, Source: `\\.\DISPLAY2`, TargetAdapter: 0x1806b, TargetID: 257},
}

func TestVDDOpenPlanReloadsWhenTheDriverHasNoMonitor(t *testing.T) {
	src, ok := vddSource(noMonitorList)
	if !ok || src.Device != `\\.\DISPLAY6` {
		t.Fatalf("vddSource = %+v, %v", src, ok)
	}
	connected := vddConnected(src, noMonitorPaths)
	if connected {
		t.Fatal("vddConnected = true for a source with no monitor and no path")
	}
	modes := [][2]int{{800, 600}, {1920, 1080}, {832, 1112}}

	mode, reload, err := vddOpenPlan(ok, connected, modes, 832, 1112, 0)
	if err != nil || !reload {
		t.Fatalf("with reloads left: mode %v, reload %v, err %v; want a reload", mode, reload, err)
	}

	_, reload, err = vddOpenPlan(ok, connected, modes, 832, 1112, maxReloads)
	if reload || !errors.Is(err, ErrUnavailable) || !errors.Is(err, errNoMonitor) {
		t.Fatalf("with no reload left: reload %v, err %v; want ErrUnavailable and errNoMonitor", reload, err)
	}
}

func TestVDDConnectedByPathWithoutMonitorID(t *testing.T) {
	src := windisplay.Monitor{Device: `\\.\DISPLAY6`, Description: vddAdapter}
	paths := []windisplay.Path{{SourceAdapter: 0x2c2338, Source: `\\.\DISPLAY6`, TargetAdapter: 0x2c2338, TargetID: 256, Available: true}}
	if !vddConnected(src, paths) {
		t.Fatal("vddConnected = false for a source with a display path")
	}
}

func TestVDDOpenPlanUsesTheSourceWithAMonitor(t *testing.T) {
	list := []windisplay.Monitor{
		{Device: `\\.\DISPLAY4`, Description: vddAdapter, Attached: true},
		{Device: `\\.\DISPLAY6`, Description: vddAdapter, MonitorID: `MONITOR\MTT1337\{4d36e96e-e325-11ce-bfc1-08002be10318}\0001`},
	}
	src, ok := vddSource(list)
	if !ok || src.Device != `\\.\DISPLAY6` {
		t.Fatalf("vddSource = %+v, %v", src, ok)
	}
	mode, reload, err := vddOpenPlan(ok, vddConnected(src, nil), [][2]int{{800, 600}, {832, 1112}}, 832, 1112, 0)
	if err != nil || reload || mode != [2]int{832, 1112} {
		t.Fatalf("mode %v, reload %v, err %v; want 832x1112 without a reload", mode, reload, err)
	}
}
