# Device management

`spareglass devices` lists the browsers that were approved. `devices revoke ID` forgets one, and `devices clear` forgets all. A running host picks up the change from disk, so a revoked browser's next Connect is rejected, and the page asks it to pair again.

## Sub-features

- `devices-list` shows `ID  Name  paired YYYY-MM-DD HH:MM`, or `No paired browsers.`
- `devices-revoke` removes one device. The revoked browser's next Connect shows `The host no longer knows this browser. Tap Connect to pair again.` and drops its stored credential.
- `devices-clear` removes every device.

## How to get to it (user POV)

- Run `spareglass devices`, `spareglass devices revoke ID` or `spareglass devices clear` in any terminal while the host runs.

## Driving it with sg

Preconditions: one paired browser (see [Pairing](./pairing.md) `pair-approve`).

- **List.** `$SG devices $ID` prints one line. Keep its first column as `DEV`.
- **Revoke.** `$SG devices $ID revoke $DEV` prints `revoked <DEV>`. `$SG devices $ID` prints `No paired browsers.`
- **Rejected on next Connect.** `$SG pw $ID reload`, tap Connect, then `$SG wait $ID 's.statusError' 15`. `$SG state $ID devices-revoked` shows that status and `paired: false`. `host.log` shows `POST /api/session status=401`.
- **Re-pair.** Connect again. A new code appears and the pairing prompt returns.

## Gotchas

- Revoking does not end a session that is already live. The tab keeps streaming until it reconnects. Check rejection on a new Connect, not on the live tab.
- `sg devices` uses this run's private `HOME`. Running the bare `spareglass devices` would change the user's real store instead.
- `devices clear` succeeds with no output check beyond `All browsers forgotten.`; confirm with `$SG devices $ID`.
