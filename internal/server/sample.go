package server

import (
	"github.com/pion/webrtc/v4/pkg/media"

	sm "github.com/naoking158/spareglass/internal/media"
)

func sampleOf(f sm.EncodedFrame) media.Sample {
	return media.Sample{Data: f.Data, Duration: f.Duration}
}
