package vdisplay

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// vddMarker precedes every resolution spareglass adds to vdd_settings.xml, so
// that its own entries can be told apart from the user's and pruned.
const vddMarker = "<!-- spareglass -->"

// vddModeBudget caps the modes the driver's monitor offers once spareglass
// has added its resolutions; with too many, the monitor does not come back
// after RELOAD_DRIVER.
const vddModeBudget = 64

var (
	vddResolutionRE = regexp.MustCompile(`(?s)<resolution>\s*<width>\s*(\d+)\s*</width>\s*<height>\s*(\d+)\s*</height>`)
	vddAddedRE      = regexp.MustCompile(`(?s)` + regexp.QuoteMeta(vddMarker) + `\s*<resolution>.*?</resolution>\s*`)
	vddRefreshRE    = regexp.MustCompile(`<refresh_rate>`)
	vddGlobalRE     = regexp.MustCompile(`<g_refresh_rate>\s*(\d+)\s*</g_refresh_rate>`)
)

// withVDDResolution returns the Virtual Display Driver's vdd_settings.xml
// with width×height listed under <resolutions>. A size already added by
// spareglass becomes the newest; one listed by anyone else is left alone.
// Older resolutions added this way are dropped until the driver would offer
// at most vddModeBudget modes; the rest of the file, comments included, is
// kept as is.
func withVDDResolution(xml string, width, height, hz int) (string, error) {
	if moved, ok := withVDDResolutionUsed(xml, width, height); ok {
		return moved, nil
	}
	for _, m := range vddResolutionRE.FindAllStringSubmatch(xml, -1) {
		if m[1] == fmt.Sprint(width) && m[2] == fmt.Sprint(height) {
			return xml, nil
		}
	}
	if !strings.Contains(xml, "</resolutions>") {
		return "", errors.New("vdd_settings.xml has no <resolutions> section")
	}
	if hz <= 0 {
		hz = 60
	}
	refresh := fmt.Sprintf("\n            <refresh_rate>%d</refresh_rate>", hz)
	for _, m := range vddGlobalRE.FindAllStringSubmatch(xml, -1) {
		if m[1] == fmt.Sprint(hz) {
			refresh = ""
		}
	}
	entry := fmt.Sprintf("    %s\n        <resolution>\n            <width>%d</width>\n            <height>%d</height>%s\n        </resolution>\n    ", vddMarker, width, height, refresh)
	i := strings.Index(xml, "</resolutions>")
	out := xml[:i] + entry + xml[i:]
	for vddModes(out) > vddModeBudget {
		added := vddAddedRE.FindAllStringIndex(out, -1)
		if len(added) < 2 {
			break
		}
		out = out[:added[0][0]] + out[added[0][1]:]
	}
	return out, nil
}

// vddModes counts the modes the driver builds from the settings: one per
// <refresh_rate>, plus one per distinct resolution for every global rate.
func vddModes(xml string) int {
	sizes := map[[2]string]bool{}
	for _, m := range vddResolutionRE.FindAllStringSubmatch(xml, -1) {
		sizes[[2]string{m[1], m[2]}] = true
	}
	return len(vddRefreshRE.FindAllStringIndex(xml, -1)) + len(sizes)*len(vddGlobalRE.FindAllStringIndex(xml, -1))
}

// withVDDResolutionUsed moves width×height to the end of the resolutions
// spareglass added, so that it is pruned last. It reports false when
// spareglass did not add that size.
func withVDDResolutionUsed(xml string, width, height int) (string, bool) {
	for _, loc := range vddAddedRE.FindAllStringIndex(xml, -1) {
		entry := xml[loc[0]:loc[1]]
		m := vddResolutionRE.FindStringSubmatch(entry)
		if m == nil || m[1] != fmt.Sprint(width) || m[2] != fmt.Sprint(height) {
			continue
		}
		if strings.HasPrefix(xml[loc[1]:], "</resolutions>") {
			return xml, true
		}
		rest := xml[:loc[0]] + xml[loc[1]:]
		i := strings.Index(rest, "</resolutions>")
		if i < 0 {
			return xml, false
		}
		return rest[:i] + "    " + strings.TrimSpace(entry) + "\n    " + rest[i:], true
	}
	return xml, false
}
