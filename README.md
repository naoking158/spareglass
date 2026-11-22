# spareglass

Use any device with a web browser — a tablet, an old laptop, a phone — as an
**extended display** for your Mac, Windows or Linux computer. No app to
install on the display device: open a link, tap Connect.

> **Status: early prototype.** Works end to end on macOS 14+ and Windows 10 /
> 11 (x64): virtual display → capture and H.264 encode → WebRTC → browser. Linux is not done yet. See [docs/design.md](docs/design.md).

| | macOS | Windows | Linux |
|---|---|---|---|
| Virtual display | ✅ CGVirtualDisplay | ✅ [Virtual Display Driver](https://github.com/VirtualDrivers/Virtual-Display-Driver) (one display at a time) | 🚧 planned (portal / Mutter) |
| Capture + encode | ✅ ScreenCaptureKit + VideoToolbox (in-process) | ✅ ffmpeg (gdigrab + Media Foundation) | 🚧 |
| Browser client | ✅ | ✅ | ✅ |

## How it works

```
host (this computer)                           display device
┌──────────────────────────────┐              ┌──────────────┐
│ virtual display ─▶ capture   │   WebRTC     │ browser      │
│       ─▶ H.264 encode ───────┼─────────────▶│ <video>      │
│ HTTP server + pairing (y/N)  │◀─────────────┤ screen size  │
└──────────────────────────────┘   LAN only   └──────────────┘
```

1. `spareglass` starts a small web server and prints a short URL such as
   `http://192.168.1.20/1` (and a QR code).
2. The display device opens it. The first time, it shows a 4-digit code and
   the host asks in its terminal whether to allow that browser. Once allowed,
   the browser is remembered and later visits connect right away.
3. The display device reports its screen size.
4. The host creates a virtual monitor of that size — the OS treats it as a real
   one, so you can drag windows onto it.
5. The monitor is captured, encoded and streamed to the browser over WebRTC.

## Install

Download the archive for your computer from
[Releases](https://github.com/naoking158/spareglass/releases), extract it and
follow `HOW-TO-USE.txt` inside the folder (Japanese).

### macOS

- macOS 14 or later. `spareglass_<version>_mac.zip` is a universal binary.
  It captures and encodes in-process; the bundled ffmpeg is used only by
  `serve --test-pattern`.
- Start it with `Start spareglass.command`. The binaries are not notarized
  yet: if macOS blocks them, allow them in System Settings → Privacy &
  Security, or run `xattr -d com.apple.quarantine ./spareglass` once.
- Give Screen Recording permission to the app you run spareglass from
  (System Settings → Privacy & Security → Screen & System Audio Recording).

### Windows

- Windows 10 / 11, 64-bit. `spareglass_<version>_windows_x64.zip` bundles
  ffmpeg and the Virtual Display Driver installer.
- Run `VirtualDisplayDriver-setup.exe` once (needs administrator approval),
  then start `spareglass.exe`. spareglass itself runs as a standard user.
- When the firewall asks, allow access on private networks; otherwise the
  display device cannot connect.

spareglass looks for ffmpeg (Windows, and `--test-pattern` on macOS) next to
its own binary first, then on `PATH`.

## Usage

```sh
spareglass            # start the host (same as "spareglass serve")
spareglass doctor     # show what works on this machine
spareglass serve --test-pattern   # stream a test pattern (checks the browser side)
spareglass config init   # write a config file with the defaults
spareglass config path   # where that file lives
spareglass config show   # print the effective config
spareglass devices        # browsers allowed to connect
spareglass devices revoke ID
spareglass devices clear
spareglass version
```

Then type the printed URL on the other device (e.g. `http://192.168.1.20/1`),
tap **Connect**, and answer `y` in the host terminal. `/1`, `/2`, … pick which
virtual display that browser shows. Both devices must be on the same network.

### Settings

Flags override the config file (`spareglass config path`).

| Flag | Config key | Default | Meaning |
|---|---|---|---|
| `--addr` | `addr` | port 80, else 7878 | Listen address |
| `--fps` | `fps` | `60` | Target frame rate |
| `--bitrate` | `bitrate_kbps` | `12000` | Video bitrate per display (kbps) |
| `--max-clients` | `max_clients` | `2` | Displays connected at once |
| `--render-scale` | `render_scale` | `1` | Display size vs. the client's CSS pixels (`1` legible, `2` sharp) |

## Security

Only browsers you approve in the host terminal can connect. Each approved
browser holds a random credential in localStorage; the host keeps only its
SHA-256 in `devices.json` next to the config file. Revoke one with
`spareglass devices revoke ID` (applies to a running host). Media is
encrypted with DTLS-SRTP. Run spareglass only on networks you trust.

## Development

Requires Go 1.27+. On macOS, building with cgo (the default when the Xcode
command line tools are installed) uses the in-process pipeline. A build with
`CGO_ENABLED=0` falls back to ffmpeg and needs it on `PATH`
(`brew install ffmpeg`); so does `--test-pattern`.

```sh
make run       # start the host with verbose logs
make test      # unit tests, including a full WebRTC negotiation
make doctor
make snapshot  # build the release archives into dist/ (needs goreleaser, macOS, mingw-w64, gh)
```

Releases: push a `v*` tag; GitHub Actions builds the bundled ffmpeg, runs
GoReleaser and attaches the archives to the release.

## License

MIT
