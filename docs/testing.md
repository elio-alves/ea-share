# Testing

## Running

```sh
go test ./...
```

Runs on any OS (Linux, Windows) — the packages with automated tests today
don't depend on any platform-specific API. A Linux CI can run the whole
suite normally, even though it can't *compile* the `_windows.go` files
(excluded from the Linux build by `//go:build windows` — see below).

## What has automated test coverage

OS-independent packages/files, covered with unit tests:

| Package/file | What it tests |
|---|---|
| `internal/protocol` | `WriteMessage`/`ReadMessage` round trip, size limit, truncated message, `Edge.Opposite()` |
| `internal/clipsync` | `WriteFrame`/`ReadFrame` round trip, size limit, `ClipAddr` (port+1 derivation) |
| `internal/auth` | `TokensEqual` (equal, different, different length, empty) |
| `internal/tlsutil` | certificate persistence, fingerprint format/determinism, `KnownHosts` (trust/lookup/disk persistence) |
| `internal/keys` | `NameToVK`↔`VKToName` round trip (Windows) / `NameToKeycode`↔`KeycodeToName` (Linux), no code duplication — only the half matching the current OS runs |
| `cmd/controller/edge_math.go` | all the pure geometry behind edge switching (`entryPosition`, `pushesPast`, `hasMovedAway`, `releaseRelPos`, `controllerWarpPosition`, plus an engage→disengage round-trip test) — extracted from `edge_windows.go` **on purpose** so it doesn't need Windows to test. It's the exact math behind the two mouse bugs described in [`docs/known-issues.md`](known-issues.md). |

## What doesn't, and why

The bulk of the logic that actually matters (Windows low-level hooks,
`SendInput`, Raw Input, native clipboard read/write) lives in
`_windows.go` files that call the Win32 API directly via `syscall`.
There's no real way to unit-test that without an actual Windows session
with a real keyboard/mouse/clipboard — there's no cheap way to simulate
`WH_MOUSE_LL`/`SendInput`/`OpenClipboard` in an automated test, and
mocking all of it would lose exactly the part that has broken the most so
far (see the bugs in [`docs/known-issues.md`](known-issues.md) — none of
them would have been caught by a mock).

Rule used in this repo: **any pure logic (math, parsing, framing,
decision-making) that can be separated from the syscall it's next to
should be extracted into a file with no build tag and tested** (that's
what was done for `edge_math.go`, and it's the model for future
extractions — e.g. if the `BI_BITFIELDS` parsing in `internal/clipboard`
grows, it'd be worth separating the pure bytes→`image.Image` parsing from
the `GetClipboardData` call).

### Manual test checklist (Windows-only, two machines)

No automated substitute for this today — run it after any change to
`internal/capture`, `internal/inject`, `internal/clipboard`, or
`cmd/*/edge_windows.go`/`cmd/*/clipboard_windows.go`:

1. **Legacy**: `controller` without `-edge` reflects keyboard/mouse
   continuously.
2. **Edge crossing**: pushing the cursor to the edge engages; pushing
   back through the same edge disengages — no spurious flip right after
   engaging (that was the `pt`-diffing bug).
3. **Clipboard text**, both directions (`Ctrl+Alt+V` engaged and
   disengaged).
4. **Clipboard image** (`PrintScreen`), both directions — covers the
   `BI_BITFIELDS` parsing.
5. **Holding the hotkey longer** (V's auto-repeat) shouldn't leave any
   modifier stuck afterward.
6. If touching `internal/keys` or the key mappings: test at least one key
   from each category (letter, number, function, modifier, punctuation)
   arriving correctly on the other end.

## `scripts/build.sh`

Not a test, but it's what guarantees the four build targets (target and
controller for Windows and Linux, tray Windows-only) keep compiling — run
it before opening a PR:

```sh
./scripts/build.sh
```
