package windisplay

import "testing"

const (
	gpu = 0x1111
	vdd = 0x2222
)

func TestPickPath(t *testing.T) {
	gpuPaths := []Path{
		{SourceAdapter: gpu, SourceID: 0, Source: `\\.\DISPLAY1`, TargetAdapter: gpu, TargetID: 10, Active: true, Available: true},
		{SourceAdapter: gpu, SourceID: 1, Source: `\\.\DISPLAY2`, TargetAdapter: gpu, TargetID: 11, Available: true},
	}
	tests := []struct {
		name       string
		paths      []Path
		wantSource string
		wantActive bool
		wantErr    bool
	}{
		{
			name: "own source",
			paths: append(gpuPaths[:2:2],
				Path{SourceAdapter: vdd, SourceID: 0, Source: `\\.\DISPLAY6`, TargetAdapter: vdd, TargetID: 20, Available: true}),
			wantSource: `\\.\DISPLAY6`,
		},
		{
			name: "target only reachable through another source of the adapter",
			paths: append(gpuPaths[:2:2],
				Path{SourceAdapter: vdd, SourceID: 0, Source: `\\.\DISPLAY6`, TargetAdapter: vdd, TargetID: 20},
				Path{SourceAdapter: vdd, SourceID: 1, Source: `\\.\DISPLAY7`, TargetAdapter: vdd, TargetID: 20, Available: true}),
			wantSource: `\\.\DISPLAY7`,
		},
		{
			name: "target already on the desktop",
			paths: append(gpuPaths[:2:2],
				Path{SourceAdapter: vdd, SourceID: 0, Source: `\\.\DISPLAY6`, TargetAdapter: vdd, TargetID: 20, Available: true},
				Path{SourceAdapter: vdd, SourceID: 1, Source: `\\.\DISPLAY7`, TargetAdapter: vdd, TargetID: 20, Active: true, Available: true}),
			wantSource: `\\.\DISPLAY7`,
			wantActive: true,
		},
		{
			name: "unavailable target is still tried",
			paths: append(gpuPaths[:2:2],
				Path{SourceAdapter: vdd, SourceID: 0, Source: `\\.\DISPLAY6`, TargetAdapter: vdd, TargetID: 20}),
			wantSource: `\\.\DISPLAY6`,
		},
		{
			name:    "no path names the device",
			paths:   append(gpuPaths[:2:2], Path{SourceAdapter: vdd, TargetAdapter: vdd, TargetID: 20, Available: true}),
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			i, active, err := pickPath(tt.paths, `\\.\DISPLAY6`)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("picked %v", tt.paths[i])
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got := tt.paths[i]; got.Source != tt.wantSource || active != tt.wantActive || got.TargetAdapter != vdd {
				t.Fatalf("picked %v active=%t, want source %s active=%t", got, active, tt.wantSource, tt.wantActive)
			}
		})
	}
}
