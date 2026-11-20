# Pairing

A browser that the host has not seen shows a 4-digit code after Connect, and the host terminal asks `Allow? [y/N]` with the same code. Approval gives the browser a credential in localStorage, so later visits skip the prompt; denial leaves it unpaired.

## Sub-features

- `pair-request` Connect on an unpaired browser shows a code and the host prompts with the same code.
- `pair-approve` `y` pairs the browser; status `Approved. Tap Connect to start.`; one device in the store.
- `pair-deny` `n` shows `The host declined this browser.` as an error and stores nothing.
- `pair-remembered` after reload, a paired browser shows `This browser is paired. Tap Connect.` and connects without a prompt.

## How to get to it (user POV)

- Open `http://<host>/1` on the display device and tap Connect.
- Answer in the terminal that runs `spareglass`.

## Driving it with sg

Preconditions: fresh run, `doctor` shows `paired devices: 0`, page opened with `$SG open $ID 1`.

- **Request.** Tap Connect. `$SG pw $ID click 'getByRole("button", { name: "Connect" })'`, then `$SG wait $ID 's.code' 10`. Read the code: `CODE=$($SG state $ID | python3 -c 'import json,sys;print(json.load(sys.stdin)["code"])')`. `$SG log $ID | grep 'Code shown'` shows the same code.
- **Deny.** `$SG answer $ID $CODE n` prints `Denied Chrome on Mac.` Then `$SG wait $ID 's.statusError' 5` and `$SG state $ID pair-deny`: `status` is `The host declined this browser.`, `paired` false, `connectEnabled` true.
- **Approve.** Tap Connect again, wait for `s.code`, read the new code, `$SG answer $ID $CODE y` prints `Allowed Chrome on Mac.` Then `$SG wait $ID 's.paired && s.connectEnabled' 5` and `$SG state $ID pair-approve`: `status` is `Approved. Tap Connect to start.`
- **Stored.** `$SG devices $ID` lists one `Chrome on Mac` device.
- **Remembered.** `$SG pw $ID reload`, `$SG state $ID pair-remembered`: `status` is `This browser is paired. Tap Connect.` Connect again, and `host.log` gets no new `wants to connect` line.

## Gotchas

- `sg answer` refuses when the waiting prompt shows another code, or when no prompt is waiting. The host discards lines typed while no prompt is shown, so an answer sent too early is lost and the request keeps waiting.
- A request expires after 2 minutes without an answer. The browser then shows `Nobody answered on the host.`
- One waiting request per IP address: tapping Connect again from the same address returns the same code. At most 4 addresses can wait at once; the 5th gets HTTP 429.
- Approval needs a second tap on Connect, because the first tap can no longer open full screen.
