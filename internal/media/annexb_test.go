package media

import (
	"bytes"
	"testing"
)

func TestReadAccessUnits(t *testing.T) {
	nal := func(b ...byte) []byte { return append([]byte{0, 0, 0, 1}, b...) }
	var stream []byte
	// AU1: SPS, PPS, IDR slice (first_mb=0)
	stream = append(stream, nal(0x67, 0x42)...)
	stream = append(stream, nal(0x68, 0xce)...)
	stream = append(stream, nal(0x65, 0x88, 0x01)...)
	// AU2: P slice split in two slices (second has first_mb != 0)
	stream = append(stream, nal(0x41, 0x9a, 0x02)...)
	stream = append(stream, nal(0x41, 0x40, 0x03)...)
	// AU3: P slice
	stream = append(stream, nal(0x41, 0x9a, 0x04)...)

	var got []EncodedFrame
	err := ReadAccessUnits(bytes.NewReader(stream), 30, func(f EncodedFrame) error {
		got = append(got, f)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d access units, want 3", len(got))
	}
	if !got[0].Keyframe || got[1].Keyframe {
		t.Fatal("keyframe flags wrong")
	}
	if c := bytes.Count(got[0].Data, []byte{0, 0, 0, 1}); c != 3 {
		t.Fatalf("AU1 has %d NALs, want 3", c)
	}
	if c := bytes.Count(got[1].Data, []byte{0, 0, 0, 1}); c != 2 {
		t.Fatalf("AU2 has %d NALs, want 2 (two slices of one picture)", c)
	}
}
