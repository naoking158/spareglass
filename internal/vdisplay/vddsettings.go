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

// vddKeep is how many resolutions added by spareglass stay listed. Each one
// is a mode the driver offers; older ones are dropped so the list does not
// grow with every browser size seen.
const vddKeep = 8

var (
	vddResolutionRE = regexp.MustCompile(`(?s)<resolution>\s*<width>\s*(\d+)\s*</width>\s*<height>\s*(\d+)\s*</height>`)
	vddAddedRE      = regexp.MustCompile(`(?s)` + regexp.QuoteMeta(vddMarker) + `\s*<resolution>.*?</resolution>\s*`)
)

// withVDDResolution returns the Virtual Display Driver's vdd_settings.xml
// with width×height listed under <resolutions>, unchanged when it is already
// listed. Only the newest vddKeep resolutions added this way are kept; the
// rest of the file, comments included, is kept as is.
func withVDDResolution(xml string, width, height, hz int) (string, error) {
	for _, m := range vddResolutionRE.FindAllStringSubmatch(xml, -1) {
		if m[1] == fmt.Sprint(width) && m[2] == fmt.Sprint(height) {
			return xml, nil
		}
	}
	if !strings.Contains(xml, "</resolutions>") {
		return "", errors.New("vdd_settings.xml has no <resolutions> section")
	}
	if extra := len(vddAddedRE.FindAllStringIndex(xml, -1)) - (vddKeep - 1); extra > 0 {
		xml = vddAddedRE.ReplaceAllStringFunc(xml, func(s string) string {
			if extra > 0 {
				extra--
				return ""
			}
			return s
		})
	}
	if hz <= 0 {
		hz = 60
	}
	i := strings.Index(xml, "</resolutions>")
	entry := fmt.Sprintf("    %s\n        <resolution>\n            <width>%d</width>\n            <height>%d</height>\n            <refresh_rate>%d</refresh_rate>\n        </resolution>\n    ", vddMarker, width, height, hz)
	return xml[:i] + entry + xml[i:], nil
}
