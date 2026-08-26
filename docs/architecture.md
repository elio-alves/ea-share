# Architecture

## Overview

```
controller (captures local input)  --TLS-->  target (injects received input)
      |                                            |
      capture.Source                          inject.Injector
      (WH_*_LL hooks + Raw Input, Windows)     (SendInput, Windows)
```

Two binaries (`cmd/controller`, `cmd/target`), one profile/GUI package
(`cmd/tray`), connected by two independent network protocols:

- **`internal/protocol`** — mouse/keyboard/handshake, where latency
  matters.
- **`internal/clipsync`** — clipboard (text/image), which can be large
  and slow without affecting the other.

Each `cmd/*` has `_windows.go` / `_other.go` (or `_linux.go`) files to
isolate what only exists on one platform — each `main.go` is OS-agnostic
and only calls functions with the same signature on both sides (e.g.
`runEdgeAware`, `startClipboardListener`).

## The `controller`'s two modes

- **Legacy** (no `-edge`): `capture.New()` — always forwards everything,
  no local suppression. `cmd/controller/main.go:runLegacy`.
- **Edge-aware** (`-edge left|right|top|bottom`): `capture.NewEdgeAware`
  — only forwards/suppresses after the cursor crosses the configured
  edge. `cmd/controller/edge_windows.go:runEdgeAware`.

## Engage/disengage state machine

Lives in `runEdgeAware` (`cmd/controller/edge_windows.go`), with the pure
geometry extracted into `cmd/controller/edge_math.go` (no build tag,
testable on any OS — see [`docs/testing.md`](testing.md)).

1. Cursor crosses the configured edge → `capture.EdgeCrossedEvent` →
   `engaged = true`, sends `MsgEngage` (with the relative position along
   the edge), starts simulating the cursor position on the `target`
   (`vx, vy`) locally, so it doesn't need a network round trip to know
   when to release.
2. While engaged, every `MouseMoveEvent` updates `vx, vy` and checks
   whether it has already moved away from the entry edge
   (`hasMovedAway`) and, later, whether it's pushed back out through it
   (`pushesPast`) — it only counts as "release" if both happened in that
   order (`movedAway && pushingOutEntry`), so it doesn't release itself
   from noise right after the crossing.
3. On release: computes where the *local* cursor should reappear
   (`controllerWarpPosition`, one pixel inside the edge that triggered
   the crossing) and calls `src.Disengage(...)`.

The `target` only participates passively: on receiving `MsgEngage`, it
computes the entry position (same logic, `entryPosition`) and does a
single `SetCursorPos`. From then on it only receives `MsgMouseMove`
(deltas) until the next `MsgEngage`.

## Clipboard (`Ctrl+Alt+V`)

- **Hotkey detection**: `internal/capture/capture_windows.go`,
  `keyboardProc` tracks Ctrl/Alt held state *outside* the normal
  forward/suppress gate (works engaged or disengaged). On seeing `V` go
  down with both held, it emits `HotkeyPasteEvent` and suppresses the
  key — it's never forwarded as a literal `V`, locally or to the target.
- **Dedicated connection**: `internal/clipsync` — simple binary framing
  (kind + length + payload, no JSON/base64) over a second TLS connection,
  main port + 1 (`ClipAddr`). This exists so a large payload
  (screenshot) never competes in the queue with mouse packets on the main
  connection.
- **Direction**: decided in `cmd/controller/edge_windows.go` at the
  moment of `HotkeyPasteEvent`, by reading the current `engaged` state —
  engaged pushes the local clipboard to the target; disengaged requests
  the target's and pastes locally (which is why the `controller` has its
  own `inject.New()`, which it didn't need before this feature).
- **Reading/writing the Windows clipboard**: `internal/clipboard` — text
  (`CF_UNICODETEXT`) and image (`CF_DIB` ↔ PNG, manual
  `BITMAPINFOHEADER` parsing, see [`docs/known-issues.md`](known-issues.md)
  on `BI_BITFIELDS`) only.

## Performance mode: UDP mouse channel (`-udp-mouse`)

- **Handshake**: piggybacks on the main connection. `MsgAuth.UDPMouse`
  (controller→target) requests the channel; if set, the target generates
  a random per-session key right after sending `MsgScreenInfo` and
  returns it as `MsgUDPKey` (`cmd/target/main.go:enableUDPMouse`). No
  separate trust decision — the key only ever travels over the already-
  authenticated TLS connection, same principle as the clipboard channel
  reusing the main connection's pinned fingerprint.
- **Wire format**: `internal/mousesync` — a fixed 48-byte binary packet
  (`seq uint64 | x int32 | y int32 | HMAC-SHA256 tag`), on `MouseAddr`
  (main port + 2; clipboard already took +1). No JSON, no TLS: the
  payload is a screen coordinate, not sensitive enough to encrypt, and
  the HMAC (keyed with the per-session key) is enough to make a packet
  unforgeable/unreplayable without a live connection to that target.
- **Absolute position, not deltas — on purpose.** `runEdgeAware`
  (`cmd/controller/edge_windows.go`) already tracks the simulated
  absolute cursor position (`vx, vy`) internally, regardless of
  transport. Performance mode sends *that*, not `e.DX/e.DY`, precisely
  so packet loss on UDP is harmless: the target (`udpMouseSession.apply`
  in `cmd/target/main.go`) only ever applies a packet whose sequence
  number is higher than the last one it saw, i.e. "newest position
  wins" — the same principle online games use to sync entity state over
  UDP. Sending relative deltas instead would have the opposite problem:
  a lost delta is motion that never happened, and the cursor drifts out
  of sync with no way to self-correct short of the next `MsgEngage`
  warp. This is the same family of gotcha as the `pt`-diffing bug in
  [`docs/known-issues.md`](known-issues.md) — anything that depends on
  every incremental update arriving is fragile under loss/suppression;
  absolute state isn't.
- **Scope**: mouse-move only, and only while engaged. Keyboard, buttons,
  wheel, `MsgEngage`/disengage, and the clipboard hotkey all keep using
  the main TCP/TLS connection unconditionally — `-udp-mouse` never
  touches those paths. Requires `-edge` (only mode with `vx, vy`
  tracking) and is Windows-only, like `-edge` itself. Any failure
  setting it up (port unreachable, key never arrives) degrades to the
  existing TCP path rather than aborting the connection.

## `tray`

`cmd/tray` has no capture/injection logic of its own — it's just a
process shell: reads/writes `%AppData%\kbs\tray_profiles.json`, draws the
menu (`fyne.io/systray`), and spawns sibling CLI binaries as hidden
subprocesses (`CREATE_NO_WINDOW`), capturing their output to the log.
The sibling binaries' names are derived from the tray's own executable
name (`process.go:siblingBinaryName`) so a suffixed build (`tray2.exe`,
see the parallel-build convention below) spawns the matching
`target2.exe`/`controller2.exe`, not the unrelated v1 pair sitting next
to it. See [`docs/known-issues.md`](known-issues.md) about the gap in
`tray.exe`'s own crash logging.

**Connection status / start-stop toggle** (`menu.go`): each profile's
menu item doubles as a switch - clicking it starts that profile if
nothing's running under its name, or stops it if it's the one currently
active (`connState`: `connWaiting` → `connTCP`/`connUDP`). The state
isn't reported by target/controller over any structured channel; it's
inferred by pattern-matching specific log lines the child process
already prints to its captured stdout/stderr (e.g. `"UDP mouse channel
enabled"`, `"authenticated, now controlling this machine"`) in
`startTarget`/`startController`'s `onLine` callback - so a wording
change to those log lines in `cmd/target`/`cmd/controller` will silently
stop being picked up here and needs a matching update in `menu.go`.

**Profile editor** (`profile_editor_windows.go`): a native `lxn/walk`
window (no CGO — pure `syscall` bindings, same style as the rest of the
project) replacing the old "edit the JSON in notepad" flow — a
master-detail form (profile list + fields) per tab (Targets,
Controllers), working on a private copy of `Config` until Save. "Save &
restart running" re-launches whichever of target/controller is currently
active with its freshly saved profile, reusing `startTarget`/
`startController` (which already stop whatever was running first) — the
direct fix for having to stop/start by hand from the tray menu to pick
up an edited profile. Requires `cmd/tray/rsrc_windows_amd64.syso` (an
embedded Windows manifest) to be present — see the `lxn/walk` entry in
[`docs/known-issues.md`](known-issues.md) for why.

## Development-parallel-build convention

When a change might break a session already in use on both machines,
build under a different name (`target2.exe`, `controller2.exe`,
`tray2.exe`, via `scripts/build.sh --suffix 2`) instead of overwriting
what's already running — that way it can be tested without dropping
whoever's already connected. `cmd/tray` doesn't hardcode that suffix;
it's a parallel-deployment practice, not a feature of the software.
