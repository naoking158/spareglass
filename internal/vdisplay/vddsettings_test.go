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

func TestWithVDDResolutionKeepsTheNewestAdded(t *testing.T) {
	doc := vddSample
	for i := range vddKeep + 3 {
		var err error
		if doc, err = withVDDResolution(doc, 1000+i, 700, 60); err != nil {
			t.Fatal(err)
		}
	}
	var s vddSettings
	if err := xml.Unmarshal([]byte(doc), &s); err != nil {
		t.Fatalf("result is not valid XML: %v\n%s", err, doc)
	}
	var got []string
	for _, r := range s.Resolutions {
		got = append(got, fmt.Sprintf("%dx%d", r.Width, r.Height))
	}
	want := []string{"800x600"}
	for i := 3; i < vddKeep+3; i++ {
		want = append(want, fmt.Sprintf("%dx700", 1000+i))
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("resolutions = %v, want %v", got, want)
	}
	if !strings.Contains(doc, "<!-- kept -->") {
		t.Fatal("comment lost")
	}
}

func TestWithVDDResolutionKeepsAReusedSize(t *testing.T) {
	doc, err := withVDDResolution(vddSample, 1180, 820, 60)
	if err != nil {
		t.Fatal(err)
	}
	for i := range vddKeep + 3 {
		if doc, err = withVDDResolution(doc, 1000+i, 700, 60); err != nil {
			t.Fatal(err)
		}
		if moved, ok := withVDDResolutionUsed(doc, 1180, 820); ok {
			doc = moved
		}
	}
	var s vddSettings
	if err := xml.Unmarshal([]byte(doc), &s); err != nil {
		t.Fatalf("result is not valid XML: %v\n%s", err, doc)
	}
	last := s.Resolutions[len(s.Resolutions)-1]
	if last.Width != 1180 || last.Height != 820 || len(s.Resolutions) != vddKeep+1 {
		t.Fatalf("resolutions = %+v, want 800x600 plus %d with 1180x820 newest", s.Resolutions, vddKeep)
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
