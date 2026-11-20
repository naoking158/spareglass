# spareglass verification map

This directory is the maintained source for verifying the user-facing behavior of spareglass. Read this index, then use the matching feature file as the recipe. All commands use the helper `SG="$(git rev-parse --show-toplevel)/.claude/skills/verify-spareglass/scripts/sg"` and a `RUN_ID` printed by `$SG up`, written below as `$ID`.

## Baseline preconditions

- Started by this run: `$SG up` (or `$SG up --real`) from the checkout under test, and `$SG doctor $ID` passes.
- The run's device store is empty at start (`doctor` prints `paired devices: 0`).
- Never drive a host that `sg up` did not start in this run.

## Driving conventions

- Browser actions: `$SG pw $ID ...` (playwright-cli). The only control is `getByRole("button", { name: "Connect" })`.
- Read client state with `$SG state $ID [NAME]` and wait with `$SG wait $ID '<predicate over s>' SEC`. Do not use fixed sleeps.
- Host terminal actions: `$SG answer $ID <CODE> y|n`. Host CLI: `$SG devices $ID ...`.
- Machine state: `$SG displays`.

## Proof and skip reporting

- Save a `state` JSON for each claimed step (`$SG state $ID <step-name>`), plus the matching `host.log` lines.
- UI proof that pixels arrive: a screenshot (`$SG pw $ID screenshot --filename=<name>.png`) that you have opened and read.
- Say which mode was used (test-pattern or real). A test-pattern run does not prove capture.
- Report an unreachable step with the command, its output and the unmet precondition. Do not substitute another path.

## Feature entry contract

Each feature file has an H1, one paragraph, then `Sub-features`, `How to get to it (user POV)`, `Driving it with sg`, `Gotchas`.

## Features

- [Pairing](./pairing.md): first-visit approval in the host terminal, deny, remembered browser.
- [Streaming](./streaming.md): Connect → virtual display → live video, resize, reconnect.
- [Display slots](./display-slots.md): `/1`…`/N` choose the display; one browser per slot; out-of-range 404.
- [Device management](./device-management.md): `spareglass devices` list/revoke/clear and its effect on browsers.
