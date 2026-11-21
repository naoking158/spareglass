package vdisplay

import (
	"encoding/xml"
	"fmt"
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

// vddDefault is the resolution part of the vdd_settings.xml that the
// Virtual Display Driver 25.5.2 installer writes: 35 modes.
const vddDefault = `<vdd_settings>
	<global>
		<g_refresh_rate>60</g_refresh_rate>
		<g_refresh_rate>90</g_refresh_rate>
		<g_refresh_rate>120</g_refresh_rate>
		<g_refresh_rate>144</g_refresh_rate>
		<g_refresh_rate>165</g_refresh_rate>
		<g_refresh_rate>244</g_refresh_rate>
	</global>
    <resolutions>
        <resolution><width>800</width><height>600</height><refresh_rate>30</refresh_rate></resolution>
        <resolution><width>1366</width><height>768</height><refresh_rate>30</refresh_rate></resolution>
        <resolution><width>1920</width><height>1080</height><refresh_rate>30</refresh_rate></resolution>
        <resolution><width>2560</width><height>1440</height><refresh_rate>30</refresh_rate></resolution>
        <resolution><width>3840</width><height>2160</height><refresh_rate>30</refresh_rate></resolution>
    </resolutions>
</vdd_settings>
`

func sizesOf(t *testing.T, doc string) []string {
	t.Helper()
	var s vddSettings
	if err := xml.Unmarshal([]byte(doc), &s); err != nil {
		t.Fatalf("result is not valid XML: %v\n%s", err, doc)
	}
	var got []string
	for _, r := range s.Resolutions {
		got = append(got, fmt.Sprintf("%dx%d", r.Width, r.Height))
	}
	return got
}

func TestWithVDDResolutionStaysWithinTheModeBudget(t *testing.T) {
	if got := vddModes(vddDefault); got != 35 {
		t.Fatalf("modes in the installer's settings = %d, want 35", got)
	}
	doc := vddDefault
	for i := range 10 {
		var err error
		if doc, err = withVDDResolution(doc, 1000+i, 700, 60); err != nil {
			t.Fatal(err)
		}
		if n := vddModes(doc); n > vddModeBudget {
			t.Fatalf("after adding %d sizes the driver would offer %d modes, over %d", i+1, n, vddModeBudget)
		}
	}
	got := sizesOf(t, doc)
	want := []string{"800x600", "1366x768", "1920x1080", "2560x1440", "3840x2160", "1006x700", "1007x700", "1008x700", "1009x700"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("resolutions = %v, want %v", got, want)
	}
}

func TestWithVDDResolutionAddsOverTheBudget(t *testing.T) {
	doc := vddDefault
	for i := range 6 {
		doc = strings.Replace(doc, "</resolutions>", fmt.Sprintf("<resolution><width>%d</width><height>500</height><refresh_rate>30</refresh_rate></resolution></resolutions>", 900+i), 1)
	}
	doc, err := withVDDResolution(doc, 1180, 820, 60)
	if err != nil {
		t.Fatal(err)
	}
	if got := sizesOf(t, doc); got[len(got)-1] != "1180x820" {
		t.Fatalf("resolutions = %v, want 1180x820 added", got)
	}
}

func TestWithVDDResolutionKeepsAReusedSize(t *testing.T) {
	doc, err := withVDDResolution(vddDefault, 1180, 820, 60)
	if err != nil {
		t.Fatal(err)
	}
	for i := range 10 {
		if doc, err = withVDDResolution(doc, 1000+i, 700, 60); err != nil {
			t.Fatal(err)
		}
		if moved, ok := withVDDResolutionUsed(doc, 1180, 820); ok {
			doc = moved
		}
	}
	if got := sizesOf(t, doc); got[len(got)-1] != "1180x820" {
		t.Fatalf("resolutions = %v, want 1180x820 newest", got)
	}
	if _, ok := withVDDResolutionUsed(doc, 800, 600); ok {
		t.Fatal("moved a resolution spareglass did not add")
	}
}

func TestWithVDDResolutionNeedsSection(t *testing.T) {
	if _, err := withVDDResolution("<vdd_settings/>", 1180, 820, 60); err == nil {
		t.Fatal("want an error without <resolutions>")
	}
}
