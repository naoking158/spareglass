//go:build darwin

package media

import (
	"fmt"
	"strconv"
	"sync"

	"github.com/ebitengine/purego"
)

// NewPipeline captures the display with ffmpeg's avfoundation input and
// encodes it with VideoToolbox (hardware H.264).
//
// avfoundation names screens "Capture screen N", where N is the display's
// position in CGGetActiveDisplayList, so the CGDirectDisplayID is mapped to
// that position first.
//
// TODO(darwin): replace with ScreenCaptureKit → VideoToolbox in-process
// (cgo) for lower latency, real keyframe requests and no ffmpeg dependency.
func NewPipeline(displayID string, fps, bitrateKbps int) (Pipeline, error) {
	if _, err := FFmpegPath(); err != nil {
		return nil, err
	}
	id, err := strconv.ParseUint(displayID, 10, 32)
	if err != nil {
		return nil, fmt.Errorf("bad display id %q", displayID)
	}
	index, err := screenIndex(uint32(id))
	if err != nil {
		return nil, err
	}

	encoder := []string{"-c:v", "h264_videotoolbox", "-realtime", "1", "-allow_sw", "1", "-profile:v", "baseline"}
	if !FFmpegHasEncoder("h264_videotoolbox") {
		encoder = []string{"-c:v", "libx264", "-preset", "ultrafast", "-tune", "zerolatency", "-profile:v", "baseline"}
	}
	kbps := strconv.Itoa(bitrateKbps) + "k"
	args := []string{
		"-hide_banner", "-loglevel", "error",
		"-fflags", "nobuffer",
		"-f", "avfoundation", "-capture_cursor", "1", "-framerate", strconv.Itoa(fps), "-pixel_format", "nv12",
		"-i", fmt.Sprintf("Capture screen %d:none", index),
	}
	args = append(args, encoder...)
	args = append(args,
		"-pix_fmt", "nv12",
		"-b:v", kbps, "-maxrate", kbps, "-bufsize", kbps,
		"-g", strconv.Itoa(fps), "-bf", "0",
		"-bsf:v", "dump_extra=freq=keyframe", // SPS/PPS before every IDR
		"-f", "h264", "pipe:1",
	)
	return NewFFmpegPipeline(args, fps)
}

var (
	cgOnce                 sync.Once
	cgErr                  error
	cgGetActiveDisplayList func(max uint32, displays *uint32, count *uint32) int32
)

func screenIndex(id uint32) (int, error) {
	cgOnce.Do(func() {
		lib, err := purego.Dlopen("/System/Library/Frameworks/CoreGraphics.framework/CoreGraphics", purego.RTLD_LAZY|purego.RTLD_GLOBAL)
		if err != nil {
			cgErr = err
			return
		}
		purego.RegisterLibFunc(&cgGetActiveDisplayList, lib, "CGGetActiveDisplayList")
	})
	if cgErr != nil {
		return 0, cgErr
	}
	var ids [32]uint32
	var n uint32
	if e := cgGetActiveDisplayList(uint32(len(ids)), &ids[0], &n); e != 0 {
		return 0, fmt.Errorf("CGGetActiveDisplayList: CGError %d", e)
	}
	for i := 0; i < int(n); i++ {
		if ids[i] == id {
			return i, nil
		}
	}
	return 0, fmt.Errorf("display %d is not active", id)
}
