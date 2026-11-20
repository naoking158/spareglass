package vdisplay

import (
	"encoding/xml"
	"strings"
	"testing"
)

const vddSample = `<?xml version='1.0' encoding='utf-8'?>
<vdd_settings>
    <monitors>
        <count>1</count>
    </monitors>
    <resolutions>
        <resolution>
            <width>800</width>
            <height>600</height>
            <refresh_rate>30</refresh_rate>
        </resolution>
    </resolutions>
    <!-- kept -->
</vdd_settings>
`

type vddSettings struct {
	Resolutions []struct {
		Width  int `xml:"width"`
		Height int `xml:"height"`
	} `xml:"resolutions>resolution"`
}

func TestWithVDDResolutionAddsOnce(t *testing.T) {
	once, err := withVDDResolution(vddSample, 1180, 820, 60)
	if err != nil {
		t.Fatal(err)
	}
	twice, err := withVDDResolution(once, 1180, 820, 60)
	if err != nil {
		t.Fatal(err)
	}
	if twice != once {
		t.Fatal("adding the same resolution again changed the file")
	}
	var s vddSettings
	if err := xml.Unmarshal([]byte(once), &s); err != nil {
		t.Fatalf("result is not valid XML: %v\n%s", err, once)
	}
	if len(s.Resolutions) != 2 || s.Resolutions[1].Width != 1180 || s.Resolutions[1].Height != 820 {
		t.Fatalf("resolutions = %+v", s.Resolutions)
	}
	if !strings.Contains(once, "<!-- kept -->") {
		t.Fatal("comment lost")
	}
}

func TestWithVDDResolutionNeedsSection(t *testing.T) {
	if _, err := withVDDResolution("<vdd_settings/>", 1180, 820, 60); err == nil {
		t.Fatal("want an error without <resolutions>")
	}
}
