# spareglass — notes for Claude Code

- Go 1.27+, module `github.com/naoking158/spareglass`. Run `make test` before committing.
- Per-OS code uses build tags: `*_darwin.go`, `*_windows.go`, `*_linux.go`, `*_other.go`.
  Keep `go vet` passing for every GOOS (`GOOS=darwin go vet ./...` etc.).
- `internal/vdisplay`: create/remove virtual monitors. Recreate instead of resizing
  (macOS: a mode change makes a CGVirtualDisplay unremovable). Use stable names
  ("spareglass N"); macOS stores an ICC profile per monitor identity.
- `internal/media`: capture → H.264 encode. Not implemented; will need cgo.
- `internal/pairing`: browser approval (terminal y/N) and remembered devices (devices.json).
- `internal/server`: routes /{n} (display n), /api/pair, /api/session (non-trickle WebRTC).
  `server_test.go` negotiates a real Pion peer; keep it green.
- `web/`: embedded browser client (plain HTML/JS, no build step).
- Design notes (Japanese): docs/design.md.
