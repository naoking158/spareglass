---
name: verify-spareglass
description: Launch an isolated spareglass host (Go; macOS natively, Linux in Docker) on loopback in tmux, drive its embedded browser client with headless Chrome via playwright-cli, approve pairing in the host terminal, and prove that pairing, WebRTC streaming, virtual displays and the devices CLI work, with JSON client state, screenshots and the host log as evidence. Use after changing server, pairing, vdisplay, media or web/ code, before opening a PR, or when asked to verify, reproduce or 動作確認 spareglass behavior.
---

# verify-spareglass

spareglass is a host process: it serves a web client at `/N`, pairs browsers by a y/N prompt in its own terminal, creates a macOS virtual display at the browser's screen size and streams it over WebRTC. This skill runs the real binary, drives the real client page, and answers the real terminal prompt. Everything goes through one helper:

```sh
SG="$(git rev-parse --show-toplevel)/.claude/skills/verify-spareglass/scripts/sg"
$SG help
```

Run `sg` from inside the spareglass checkout you want to verify (it builds that checkout). The `SG=` line above resolves the helper of that same checkout.

## Requirements

- macOS 14+ with `mise`, `tmux` and `python3`.
- `mise install` in the checkout. The repo's `mise.toml` provides go, ffmpeg, node and `playwright-cli`, and `sg` runs playwright-cli through it (`mise -C <checkout> exec`), so no global install is needed.
- `--real` mode only: Screen Recording permission for the terminal app that runs Claude Code.

## Launch

```sh
$SG up                 # test-pattern mode (default)
$SG up --real          # real capture of the virtual display
```

`up` prints `RUN_ID=<id>` first, then the URL, mode, git rev, checkout fingerprint and the evidence directory. Copy the RUN_ID literally into every later command. It:

- builds `./cmd/spareglass` into `~/.cache/spareglass-verify/<id>/spareglass`,
- starts `serve -v --addr 127.0.0.1:<free port>` in tmux session `sg-<id>` with `HOME=<run>/home`, so `config.json` and `devices.json` are private to this run (the user's paired devices are never touched),
- waits until `/api/healthz` answers. If it does not within 10s, it tears the run down and exits non-zero.

Ready means: `sg doctor` passes and `sg open` shows the page title `spareglass · display 1`.

Pick the mode by what changed:

| Changed code | Mode | What it proves |
|---|---|---|
| `web/`, `internal/server`, `internal/pairing` | `up` (test pattern) | pairing, signaling, WebRTC, client UI. A virtual display is still created, but ffmpeg streams a test pattern instead of capturing it. |
| `internal/vdisplay`, `internal/media` | `up --real` | the virtual display is captured and its desktop arrives in the browser. |

Never report a test-pattern run as proof of capture/encode.

### Linux target

```sh
$SG up --linux
```

This cross-builds `GOOS=linux` for the Docker daemon's arch, runs the host in container `sg-<id>` (`debian:bookworm-slim`, stdin attached through tmux so `sg answer` works), and runs Debian `chromium` in container `sg-<id>-browser`, which shares the host container's network namespace. playwright-cli on the Mac drives that browser over CDP, and the page URL is `http://127.0.0.1:7878/N` as seen from inside. The browser image `sg-linux-chromium` is built from `linux/Dockerfile.browser` on first use. Every other `sg` command works unchanged. `devices` runs `docker exec`, and `displays` is macOS only.

It needs Docker (colima or Docker Desktop) with `$HOME` mounted, because the run directory under `~/.cache` is bind-mounted as `/app`.

What a Linux run can prove today: the Linux build starts, the config path is `/root/.config/spareglass/`, pairing works, WebRTC reaches `connected`, the client ends at the `no-display` message (`The host cannot create a display: virtual displays are not implemented on this platform yet…`) with Connect enabled again, and the devices CLI works. Streaming is unreachable on Linux until `internal/vdisplay` and `internal/media` are implemented, so report it as unreachable. A plain container has no compositor, so this target cannot verify future portal/Mutter virtual-display work either. That needs a GNOME/KDE session with PipeWire.

The browser must be able to receive H.264. Check with `$SG pw $ID --raw eval '() => RTCRtpReceiver.getCapabilities("video").codecs.some(c => /h264/i.test(c.mimeType))'`. Debian `chromium` returns `true`. `chromedp/headless-shell` returns `false`, and the host then fails with `unable to start track, codec is not supported by remote` (HTTP 500).

There is no Windows target. Docker on macOS cannot run Windows, and verifying Windows needs a Windows VM or a Windows CI runner.

## Doctor

```sh
$SG doctor <RUN_ID>
```

Read-only. Checks: tmux session alive; the port's listener is this run's binary; `/api/healthz` ok; `/1` serves the client; host config lives under this run's `home/`; the checkout fingerprint (HEAD + tracked diff + untracked files) still equals the one built. A fingerprint mismatch means you edited code after `up`: `down` and `up` again. Run doctor first whenever anything looks off.

## Drive

| Command | Use |
|---|---|
| `$SG open <RUN_ID> [N]` | open `/N` (default 1) in this run's headless Chrome session |
| `$SG pw <RUN_ID> click 'getByRole("button", { name: "Connect" })'` | the only button on the page |
| `$SG state <RUN_ID> [NAME]` | client state JSON (below); `NAME` also saves `evidence/NAME.json` |
| `$SG wait <RUN_ID> '<JS predicate over s>' [SEC]` | poll state until true; exits 1 with the last state on timeout |
| `$SG answer <RUN_ID> <CODE> y\|n` | answer the host prompt; refuses unless the waiting prompt shows `CODE` |
| `$SG devices <RUN_ID> [revoke ID\|clear]` | the real `spareglass devices` CLI against this run's store |
| `$SG displays` | spareglass virtual displays macOS has right now (`system_profiler`) |
| `$SG log <RUN_ID>` | host log (stdout + stderr, `-v` debug HTTP lines) |
| `$SG pw <RUN_ID> <playwright-cli args>` | anything else: `screenshot --filename=x.png`, `snapshot`, `reload`, `resize W H`, `tab-new`, `tab-select I`, `goto URL`, `console` |

`sg pw` runs with cwd = the evidence directory, so `--filename` paths land there, and it exits non-zero when playwright-cli reports an error.

State fields (`s` in predicates): `url`, `live` (body has class `live` = host said streaming), `videoWidth`, `videoHeight`, `t` (video currentTime), `status` (text of `#status`), `statusError`, `code` (pairing code or null), `connectEnabled`, `screen` ([w, h]), `paired` (credential in localStorage).

Standard pairing + stream journey:

```sh
$SG open $ID 1
$SG pw $ID click 'getByRole("button", { name: "Connect" })'
$SG wait $ID 's.code' 10
CODE=$($SG state $ID | python3 -c 'import json,sys;print(json.load(sys.stdin)["code"])')
$SG answer $ID $CODE y
$SG wait $ID 's.paired && s.connectEnabled' 5
$SG pw $ID click 'getByRole("button", { name: "Connect" })'
$SG wait $ID 's.live && s.videoWidth > 0 && s.t > 1' 20
$SG state $ID live
$SG pw $ID screenshot --filename=live.png
$SG displays
```

Feature recipes: [features/README.md](features/README.md).

## Evidence

Everything lives in `~/.cache/spareglass-verify/<RUN_ID>/evidence/` and survives `down`:

- `host.log`: full host output, including the pairing prompt, `session created`, `client screen`, `virtual display created ... size=WxH`, `first frame sent`, `shutting down`.
- `NAME.json` from `sg state ID NAME`: client state at each step. Capture one per action you claim.
- `*.png` from `sg pw ID screenshot --filename=...`. Read the PNG to confirm content (test pattern bars vs. a macOS desktop).
- `devices.txt` (every `sg devices` output), `devices-final.txt` (store at teardown), `displays-after-down.txt` (proves the virtual displays were removed).
- `.playwright-cli/`: snapshots and console logs from playwright-cli.

Streaming is proven by `live: true`, `videoWidth > 0` and `t` increasing between two `state` calls, plus `first frame sent` in the log. The `#status` text is not proof: it stays `Connecting…` while live.

## Cleanup

```sh
$SG down <RUN_ID>
```

Closes this run's browser session, sends Ctrl-C to this run's host (which removes its virtual displays and logs `shutting down`), kills only tmux session `sg-<RUN_ID>`, deletes `home/` and the binary, keeps `evidence/` and `run.env`, and prints `displays-after-down.txt`. Run it after every attempt, failed or not. Never `pkill spareglass`: the user may run their own host.

If `up` printed a RUN_ID but failed later, still run `sg down <RUN_ID>`.

## Gotchas

- Virtual displays are machine-wide and named `spareglass N` by slot. A display in `sg displays` may belong to a concurrent run or to the user's own spareglass. That is not a leak, and it is not yours to kill.
- The startup banner prints LAN URLs (`http://192.168.x.x:PORT/1`) that do not work, because the run binds to 127.0.0.1. Use the URL `sg up` prints.
- Coverage limits: headless Chrome on loopback. Not covered: LAN reachability, port 80, Safari/iPad, real full screen, touch.
- `--real` screenshots contain the user's real desktop (the virtual display shows their wallpaper and menu bar). Keep them local.
- In test-pattern mode `videoWidth` is always 1280×720 whatever the display size. Check the display size in `host.log` (`virtual display created ... size=`) or `sg displays`.
- The console always shows one error: `/favicon.ico` 404. Ignore it, but read any other console errors.
- All tabs of one `sg` session share localStorage, so pairing in one tab pairs them all.
- A browser without H.264 sees only `could not start session` and gets HTTP 500 from `/api/session`. The real cause (`codec is not supported by remote`) appears only in `host.log`.
- Linux: the headless Chromium screen is 800×600, not 1280×720.

## When you cannot verify

Report which of these stopped you:

1. **Cannot start:** `up` or `doctor` fails (build error, no tmux, port). Include the output and `host.log`.
2. **Cannot drive:** playwright-cli missing (run `mise install` in the checkout), or `--real` without Screen Recording permission (log shows `capture pipeline stopped`).
3. **Missing precondition:** e.g. macOS refuses another display (status `no-display`) because the Mac's external-display limit is reached.
