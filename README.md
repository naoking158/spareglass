# spareglass

Use any device with a web browser — a tablet, an old laptop, a phone — as an
**extended display** for your Mac, Windows or Linux computer. No app to
install on the display device: open a link, tap Connect.

> **Status: early prototype.** Pairing, the browser client and WebRTC
> negotiation work. Virtual displays work on macOS 14+. Screen capture and
> video streaming are not implemented yet. See [docs/design.md](docs/design.md).

| | macOS | Windows | Linux |
|---|---|---|---|
| Virtual display | ✅ CGVirtualDisplay | 🚧 planned (IddCx) | 🚧 planned (portal / Mutter) |
| Capture + encode | 🚧 | 🚧 | 🚧 |
| Browser client | ✅ | ✅ | ✅ |

## How it works

```
host (this computer)                           display device
┌──────────────────────────────┐              ┌──────────────┐
│ virtual display ─▶ capture   │   WebRTC     │ browser      │
│       ─▶ H.264 encode ───────┼─────────────▶│ <video>      │
│ HTTP server + pairing token  │◀─────────────┤ screen size  │
└──────────────────────────────┘   LAN only   └──────────────┘
```

1. `spareglass` starts a small web server and prints a URL and QR code.
2. The display device opens it and reports its screen size.
3. The host creates a virtual monitor of that size — the OS treats it as a real
   one, so you can drag windows onto it.
4. The monitor is captured, encoded and streamed to the browser over WebRTC.

## Install

Download the archive for your OS from
[Releases](https://github.com/naoking158/spareglass/releases), extract it and
run the binary.

macOS: binaries are not notarized yet. If macOS blocks it, run
`xattr -d com.apple.quarantine ./spareglass` once.

## Usage

```sh
spareglass            # start the host (same as "spareglass serve")
spareglass doctor     # show what works on this machine
spareglass config init   # write a config file with the defaults
spareglass config path   # where that file lives
```

Then open the printed URL on the other device (or scan the QR code) and tap
**Connect**. Both devices must be on the same network.

### Settings

Flags override the config file (`spareglass config path`).

| Flag | Config key | Default | Meaning |
|---|---|---|---|
| `--addr` | `addr` | `:7878` | Listen address |
| `--fps` | `fps` | `60` | Target frame rate |
| `--bitrate` | `bitrate_kbps` | `12000` | Video bitrate per display (kbps) |
| `--max-clients` | `max_clients` | `2` | Displays connected at once |
| `--render-scale` | `render_scale` | `1` | Display size vs. the client's CSS pixels (`1` legible, `2` sharp) |
| `--token` | `token` | random | Fixed pairing token, for a bookmarkable URL |

## Security

Anyone with the pairing link can see the virtual display, so treat it like a
password. The token is kept in the URL fragment and never sent in request
lines; media is encrypted with DTLS-SRTP. Run spareglass only on networks you
trust.

## Development

Requires Go 1.27+.

```sh
make run      # start the host with verbose logs
make test     # unit tests, including a full WebRTC negotiation
make doctor
```

Releases: push a `v*` tag; GitHub Actions runs GoReleaser and attaches the
binaries to the release.

## License

MIT
