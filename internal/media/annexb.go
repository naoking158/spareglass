package media

import (
	"bytes"
	"io"
	"time"

	"github.com/pion/webrtc/v4/pkg/media/h264reader"
)

var startCode = []byte{0, 0, 0, 1}

// ReadAccessUnits reads an H.264 Annex-B byte stream (e.g. ffmpeg -f h264)
// and calls out once per access unit (one picture plus its SPS/PPS/SEI),
// with the NAL units joined by start codes as Pion expects.
//
// An access unit is only known to be complete when the next one starts, so
// this adds up to one frame of latency; a native encoder callback avoids that.
func ReadAccessUnits(r io.Reader, fps int, out func(EncodedFrame) error) error {
	rd, err := h264reader.NewReader(r)
	if err != nil {
		return err
	}
	var (
		buf     bytes.Buffer
		hasVCL  bool
		isKey   bool
		last    time.Time
		nominal = time.Second / time.Duration(max(fps, 1))
	)
	emit := func() error {
		if buf.Len() == 0 {
			return nil
		}
		d := nominal
		now := time.Now()
		if !last.IsZero() {
			if since := now.Sub(last); since > 0 && since < time.Second {
				d = since
			}
		}
		last = now
		f := EncodedFrame{Data: bytes.Clone(buf.Bytes()), Duration: d, Keyframe: isKey}
		buf.Reset()
		hasVCL, isKey = false, false
		return out(f)
	}

	for {
		nal, err := rd.NextNAL()
		if err == io.EOF {
			return emit()
		}
		if err != nil {
			return err
		}
		if len(nal.Data) == 0 {
			continue
		}
		typ := nal.Data[0] & 0x1f
		vcl := typ == 1 || typ == 5
		if hasVCL && startsAccessUnit(typ, vcl, nal.Data) {
			if err := emit(); err != nil {
				return err
			}
		}
		buf.Write(startCode)
		buf.Write(nal.Data)
		if vcl {
			hasVCL = true
		}
		if typ == 5 {
			isKey = true
		}
	}
}

// startsAccessUnit reports whether a NAL that follows a picture begins the
// next access unit (H.264 spec 7.4.1.2.3, simplified).
func startsAccessUnit(typ byte, vcl bool, data []byte) bool {
	if !vcl {
		// AUD, SPS, PPS, SEI and reserved 14-18 only appear before a picture.
		return typ == 9 || typ == 7 || typ == 8 || typ == 6 || (typ >= 14 && typ <= 18)
	}
	// A slice with first_mb_in_slice == 0 starts a new picture. first_mb is
	// ue(v); its first bit is 1 exactly when the value is 0.
	return len(data) > 1 && data[1]&0x80 != 0
}
