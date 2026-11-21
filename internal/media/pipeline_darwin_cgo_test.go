//go:build darwin && cgo

package media

import (
	"context"
	"testing"
	"time"
)

func nalTypes(au []byte) []byte {
	var types []byte
	for i := 0; i+4 < len(au); i++ {
		if au[i] == 0 && au[i+1] == 0 && au[i+2] == 0 && au[i+3] == 1 {
			types = append(types, au[i+4]&0x1f)
		}
	}
	return types
}

// TestEncoderEmitsEachFrameAtOnce guards the latency fix: every frame comes
// out before the next one goes in, the first is an IDR with SPS/PPS, and the
// rest are P frames.
func TestEncoderEmitsEachFrameAtOnce(t *testing.T) {
	frames, err := encodeGenerated(1280, 720, 60, 8000, 30, 20*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	first := nalTypes(frames[0].data)
	if !frames[0].key || len(first) < 3 || first[0] != 7 || first[1] != 8 || first[len(first)-1] != 5 {
		t.Fatalf("first frame: key=%v NAL types %v, want keyframe 7 8 ... 5", frames[0].key, first)
	}
	for i, f := range frames[1:] {
		if f.key {
			t.Fatalf("frame %d is a keyframe; want P frames until a keyframe is requested", i+1)
		}
		if types := nalTypes(f.data); len(types) == 0 || types[len(types)-1] != 1 {
			t.Fatalf("frame %d: NAL types %v, want a non-IDR slice", i+1, types)
		}
	}
}

// TestRunAfterCloseIsQuiet: a pipeline closed before Run (the display was
// recreated meanwhile) must not start capturing or report an error.
func TestRunAfterCloseIsQuiet(t *testing.T) {
	p, err := NewPipeline("4294967295", 60, 8000) // no such display
	if err != nil {
		t.Fatal(err)
	}
	_ = p.Close()
	if err := p.Run(context.Background(), func(EncodedFrame) error { return nil }); err != nil {
		t.Fatalf("Run after Close = %v, want nil", err)
	}
}
