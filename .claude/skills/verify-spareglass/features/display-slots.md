# Display slots

The URL path picks the display: `/1` … `/N`, where N is `max_clients` (default 2). Each slot has its own virtual display, named `spareglass N`, and is held by one browser at a time. `/` redirects to `/1`, and paths beyond N return 404.

## Sub-features

- `slot-title` `/2` shows `Use this screen as display 2` and the page title `spareglass · display 2`.
- `slot-second` a second Connect on `/2` creates `spareglass 2` next to `spareglass 1`.
- `slot-in-use` Connect on a slot another browser holds shows `that display is already in use by another browser` (HTTP 409).
- `slot-out-of-range` `/3` with the default config returns 404, and `/` redirects to `/1`.

## How to get to it (user POV)

- Type `http://<host>/2` on a second device.

## Driving it with sg

Preconditions: paired browser, with `/1` live in tab 0 (see [Streaming](./streaming.md) `stream-live`).

- **Second slot.** `$SG pw $ID tab-new`, `$SG pw $ID tab-select 1`, then `$SG pw $ID goto <URL>/2` (URL from `sg up`). `$SG state $ID slot-title` shows `url: "/2"` and `paired: true`. Connect, then `$SG wait $ID 's.live && s.t > 1' 20`. `$SG displays` lists both `spareglass 1` and `spareglass 2`.
- **Slot in use.** `$SG pw $ID tab-new`, `$SG pw $ID tab-select 2`, `$SG pw $ID goto <URL>/1`, Connect, then `$SG wait $ID 's.statusError' 15`. `$SG state $ID slot-in-use` shows `status: "that display is already in use by another browser"`. Tab 0 stays live (`tab-select 0`, then `state`).
- **Out of range.** `curl -s -o /dev/null -w '%{http_code}' <URL>/3` prints `404`. `curl -s -o /dev/null -w '%{http_code} %{redirect_url}' <URL>/` prints `302 <URL>/1`.

## Gotchas

- All tabs share localStorage, so pairing in one tab covers the others. To test an unpaired second browser, `$SG pw $ID localstorage-clear` in that tab (this clears the credential for every tab).
- `tab-new` does not switch tabs: `tab-select` first, then `goto`.
- The slot is freed only when the old session closes (data channel close or peer failure). A reload frees it within about a second.
