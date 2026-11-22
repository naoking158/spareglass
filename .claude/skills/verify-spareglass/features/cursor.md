# Pointer

On macOS the captured video has no pointer. The host reads the pointer about 120 times a second and sends its position in display points on the `cursor` data channel (unordered, no retransmits); the client draws an arrow at that point of the letterboxed video, scaled like the video. When the pointer is on another display the client hides the arrow. Windows and macOS builds without cgo keep the pointer in the video and send nothing.

## Sub-features

- `cursor-follow` the arrow's tip is where the pointer is, at 1x and HiDPI.
- `cursor-not-in-video` moving the pointer does not change the video.
- `cursor-hide` the arrow disappears when the pointer leaves the display.
- `cursor-rate` how often the client gets a new position while the pointer moves.

## How to get to it (user POV)

- Connect, then move the mouse onto the spareglass display (System Settings → Displays arranges it next to the others).

## Driving it with sg

Preconditions: streaming works ([Streaming](./streaming.md)). Use `--real` to see the video without a pointer; the arrow is drawn in test-pattern mode too. For HiDPI, open the page with a 2x device scale: write `{"browser":{"contextOptions":{"deviceScaleFactor":2,"viewport":{"width":1920,"height":1080}}}}` to a file and run `$SG pw $ID open --config=<file> <URL>/1` instead of `sg open`. macOS turns a 1280x720 HiDPI request into 1x, so use 1920x1080 (the log shows `pixels=3840x2160`).

- **Follow.** `$SG pointer $ID at 400 300`, then `$SG wait $ID 's.cursor && s.cursor.video[0] === 400' 5`. At HiDPI the `video` coordinates are doubled (`[800, 600]`), and `css` stays `[400, 300]` for a full-screen video.
- **Not in the video.** Take `$SG state $ID` after two different `pointer at` moves: `frame` is the same and `cursor` differs. To look at pixels, hide the arrow with `$SG pw $ID --raw eval '() => { document.getElementById("cursor").style.visibility = "hidden"; return 1; }'`, then take a screenshot.
- **Hide.** `$SG pointer $ID away`, then `$SG wait $ID 's.cursor === null' 5`.
- **Rate.** Record message times with `$SG pw $ID --raw eval '() => { window.__cur = []; cursorChannel.addEventListener("message", (e) => window.__cur.push([performance.now(), JSON.parse(e.data)])); return 1; }'`, run `$SG pointer $ID sweep 5 120`, then read `window.__cur` and take the intervals between messages whose position changed.

## Gotchas

- `pointer sweep` posts events every 8.3ms, but the window server can apply them in bursts. A listen-only event tap or `CGEventGetLocation` polled every millisecond shows the same bursts, so compare the client's intervals with one of those before blaming the host.
- A single `pointer at` can be lost when the window server is busy. Wait for `s.cursor` to match instead of sleeping.
- `pointer` moves the real pointer of the Mac running the run.
