# Local patches

Copy of github.com/go-macos/virtualdisplay v0.5.0 (BSD-3-Clause, see LICENSE),
used through a `replace` in spareglass's go.mod.

- `Open` accepts a HiDPI display that comes up in its Retina mode (half the
  requested size in points, the requested size in pixels). Upstream compares
  pixels only, and the creating process usually cannot read the mode, so it
  sees points and rejects the display with ErrWrongMode. See `cameUp`.
