# Streaming

A paired browser taps Connect. The host negotiates WebRTC, creates a virtual display (`spareglass N`) the size of the browser's screen, and streams it as H.264 into the page's `<video>`. When the browser's screen size changes, the host recreates the display at the new size. Closing the page frees the slot so the browser can reconnect.

## Sub-features

- `stream-live` Connect → `session created` → `virtual display created size=WxH` → `first frame sent`; video plays.
- `stream-real` (`--real` only) the video shows the virtual display's macOS desktop, not the test pattern.
- `stream-resize` a screen size change sends `resize` and the display is recreated at the new size.
- `stream-reconnect` reload + Connect closes the old session and creates a new one on the same slot.
- `stream-teardown` stopping the host removes every virtual display it created.

## How to get to it (user POV)

- Open `http://<host>/1` on a paired browser and tap Connect. Rotate or resize the device to resize.

## Driving it with sg

Preconditions: the browser is paired (run [Pairing](./pairing.md) `pair-approve` first) and `$SG displays` shows no `spareglass 1` owned by someone else.

- **Connect.** `$SG pw $ID click 'getByRole("button", { name: "Connect" })'`, then `$SG wait $ID 's.live && s.videoWidth > 0 && s.t > 1' 20`. `$SG state $ID stream-live` shows `live: true`.
- **Playing, not frozen.** In `--real` mode run `$SG poke $ID` first. Then `$SG state $ID stream-live-2` a second or more later shows a different `frame` than `stream-live.json`. A larger `t` with the same `frame` means the stream is frozen.
- **Host side.** `$SG log $ID | grep -E 'virtual display created|first frame sent'` shows both lines, with `size=1280x720` for the default headless screen. `$SG displays` lists `spareglass 1: Resolution: 1280 x 720`.
- **Pixels.** `$SG pw $ID screenshot --filename=stream-live.png`, then read the PNG. Test-pattern mode: color bars with a timecode. `--real`: a macOS desktop (wallpaper + menu bar).
- **Large display (`--real`, media or vdisplay changes).** On a fresh page, `$SG pw $ID resize 1920 1080` before Connect, then repeat Connect, poke and the two `state` calls. The log shows `size=1920x1080`, and `frame` must change after `poke`.
- **Resize.** `$SG pw $ID resize 1024 768`, then `$SG wait $ID 's.screen[0] === 1024' 5`. The log gets `client screen ... type=resize css=1024x768` and `virtual display created ... size=1024x768`. `$SG displays` shows `1024 x 768`.
- **Reconnect.** `$SG pw $ID reload`, Connect, then `$SG wait $ID 's.live && s.t > 1' 20`. The log shows `session closed` for the old session id and `session created` for a new one, with no new pairing prompt.
- **Teardown.** `$SG down $ID`. `displays-after-down.txt` reads `no spareglass virtual displays` (unless another host owns one), and `host.log` ends with `shutting down`, followed by `session closed` for any session that was still live.

## Gotchas

- Linux target (`sg up --linux`): streaming is unreachable because virtual displays are not implemented. The journey stops after Connect with `statusError`, `connectEnabled: true` and the status `The host cannot create a display: …`, and the log shows `state=connected` followed by `virtual display refused`. Report this as unreachable, not verified.

- `#status` stays `Connecting…` while live. Assert `live`, `videoWidth` and a changing `frame`, not the status text.
- A frozen stream still has `live: true`, `videoWidth > 0` and an increasing `t`, and the host still logs `first frame sent`. Only a `frame` that does not change after `poke` reveals it.
- Test-pattern video is always 1280×720. Use the log or `sg displays` for the display size.
- If macOS refuses the size, the host retries smaller sizes (`virtual display refused` in the log, status `retrying`), and then reports `no-display`, which ends the session and re-enables Connect. Each refusal costs about 5s, so allow 20s or more in `wait`.
- `--real` needs Screen Recording permission for the terminal app. Without it, the log shows `capture pipeline stopped` and the client shows an error.
- The headless screen is the viewport (1280×720 by default). `resize` changes `screen.width/height`, which is what the client reports.
