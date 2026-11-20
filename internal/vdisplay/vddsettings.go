package vdisplay

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

var vddResolutionRE = regexp.MustCompile(`(?s)<resolution>\s*<width>\s*(\d+)\s*</width>\s*<height>\s*(\d+)\s*</height>`)

// withVDDResolution returns the Virtual Display Driver's vdd_settings.xml
// with width×height listed under <resolutions>, unchanged when it is already
// listed. The rest of the file, comments included, is kept as is.
func withVDDResolution(xml string, width, height, hz int) (string, error) {
	for _, m := range vddResolutionRE.FindAllStringSubmatch(xml, -1) {
		if m[1] == fmt.Sprint(width) && m[2] == fmt.Sprint(height) {
			return xml, nil
		}
	}
	i := strings.Index(xml, "</resolutions>")
	if i < 0 {
		return "", errors.New("vdd_settings.xml has no <resolutions> section")
	}
	if hz <= 0 {
		hz = 60
	}
	entry := fmt.Sprintf("    <resolution>\n            <width>%d</width>\n            <height>%d</height>\n            <refresh_rate>%d</refresh_rate>\n        </resolution>\n    ", width, height, hz)
	return xml[:i] + entry + xml[i:], nil
}
