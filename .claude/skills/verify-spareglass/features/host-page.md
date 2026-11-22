# Host page

`spareglass serve` opens a page on the host computer (`http://127.0.0.1:<port>/#<token>`, printed as `Host page (approve devices, quit):`). It shows the address and QR code for the display device, asks to allow new browsers with their 4-digit code, lists connected displays and allowed devices (with Remove), and has a Quit button. The terminal `y/N` prompt still works; whichever answers first wins. `sg up` passes `--no-open`, so the page never opens in the user's own browser.

## Sub-features

- `host-open` the page loads with the token and shows the connect URL and a QR image; without the token it says to open it from the address spareglass shows.
- `host-approve` a pairing request appears as a card with the same code the display device shows; Allow pairs the browser, and the terminal prompt prints `(answered in the spareglass window)`.
- `host-deny` Don't allow makes the display device show `The host declined this browser.`
- `host-displays` a streaming browser is listed under Connected displays with its size.
- `host-forget` Remove (after a confirm dialog) drops the device; its stream closes at once and the browser has to pair again.
- `host-quit` Quit (after a confirm dialog) stops the host: `shutting down` in `host.log`, the page says spareglass has quit, and the virtual displays are gone.

## How to get to it (user POV)

- Start spareglass; the default browser opens the page. Answer requests there instead of in the terminal.

## Driving it with sg

Preconditions: fresh run (`sg up`), display tab opened with `$SG open $ID 1` (tab 0).

- **Open.** `HOST=$($SG host $ID)`, `$SG pw $ID tab-new "$HOST"`, `$SG pw $ID tab-select 1`, `$SG pw $ID snapshot`: the heading `spareglass`, a URL ending in `/1`, and the QR `img`. Language follows the browser (headless Chrome is English; open the tab from a playwright-cli config with `"contextOptions": {"locale": "ja-JP"}` to see Japanese).
- **Approve.** `$SG pw $ID tab-select 0`, click Connect, `$SG wait $ID 's.code' 10`, read `CODE`. `$SG pw $ID tab-select 1`, `$SG pw $ID snapshot` shows `CODE` under "A device wants to connect"; click `getByRole("button", { name: "Allow", exact: true })` (without `exact` it also matches "Don't allow" and playwright refuses). Back on tab 0, `$SG wait $ID 's.paired && s.connectEnabled' 5`. `host.log` has `pairing answered on the host page` and `(answered in the spareglass window)`.
- **Displays.** Connect on tab 0, wait `s.live`, snapshot tab 1: `Display 1: Chrome on Mac (1280×720)`.
- **Forget.** On tab 1, click `getByRole("button", { name: "Remove" })`; the click returns with a pending confirm dialog, then `$SG pw $ID dialog-accept`. `host.log` has `session closed` and `device forgotten on the host page`. Tab 0: `$SG wait $ID '!s.live && s.connectEnabled' 5`, status `The host closed this connection. Tap Connect to retry.` The next Connect says `The host no longer knows this browser. Tap Connect to pair again.`, and the one after that shows a new code.
- **Quit.** Click `getByRole("button", { name: "Quit spareglass" })`, `$SG pw $ID dialog-accept`. Tab 1 says `spareglass has quit`, `host.log` ends with `shutting down`, `$SG displays` shows none. Run `$SG down $ID` afterwards anyway.

## Gotchas

- The page lives on its own `127.0.0.1` listener, not on the display port; the display URL's port never serves it.
- The page polls every second; wait a moment or snapshot again after an action.
- Linux target: the page listens inside the container and `sg host` refuses.
