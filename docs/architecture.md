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

## `tray`

`cmd/tray` has no capture/injection logic of its own — it's just a
process shell: reads/writes `%AppData%\kbs\tray_profiles.json`, draws the
menu (`fyne.io/systray`), and spawns `target*.exe`/`controller*.exe` as
hidden subprocesses (`CREATE_NO_WINDOW`), capturing their output to the
log. See [`docs/known-issues.md`](known-issues.md) about the gap in
`tray.exe`'s own crash logging.

## Development-parallel-build convention

When a change might break a session already in use on both machines,
build under a different name (`target2.exe`, `controller2.exe`,
`tray2.exe`, via `scripts/build.sh --suffix 2`) instead of overwriting
what's already running — that way it can be tested without dropping
whoever's already connected. `cmd/tray` doesn't hardcode that suffix;
it's a parallel-deployment practice, not a feature of the software.
