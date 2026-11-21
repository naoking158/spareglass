//go:build !windows

package media

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEncoderListIsRetriedAfterAFailure(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "ran-once")
	ffmpeg := filepath.Join(dir, "ffmpeg")
	script := "#!/bin/sh\nif [ ! -e " + marker + " ]; then touch " + marker + "; exit 1; fi\necho ' V....D h264_fake  fake'\n"
	if err := os.WriteFile(ffmpeg, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	if ffmpegEncoders(ffmpeg) != nil {
		t.Fatal("first call should fail")
	}
	if got := string(ffmpegEncoders(ffmpeg)); got == "" {
		t.Fatal("a failed read was cached")
	}
}
