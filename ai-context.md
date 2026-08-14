# ai-context

Entry point for anyone (or any AI) working on `ea-share` day to day. This
file is deliberately kept **short** — it's an index, not an encyclopedia.
Only open the linked files below when the task at hand actually touches
that area; don't load everything every time.

## What this is

`ea-share` is a "reverse KVM": `controller` captures local keyboard/mouse
and sends it to `target`, which injects it. Two modes (always-share, or
Synergy-style edge switching via `-edge`), a shared clipboard via
`Ctrl+Alt+V`, and an optional system tray icon (`tray`) to run without a
terminal. Usage details: [`README.md`](README.md).

Go, no CGO, Windows is the primary platform (all the real input handling
goes through direct Win32 `syscall` calls); Linux only has the
always-share mode; macOS isn't implemented.

## Before touching something, read

| If the task involves... | Read |
|---|---|
| Understanding how the pieces connect, the protocol, engage/disengage states | [`docs/architecture.md`](docs/architecture.md) |
| A bug that *looks* familiar (mouse freezes, clipboard won't paste, stuck key) | [`docs/known-issues.md`](docs/known-issues.md) — **check here before investigating from scratch** |
| Running or writing a test, deciding whether something is testable | [`docs/testing.md`](docs/testing.md) |
| Building for Windows/Linux | [`scripts/build.sh`](scripts/build.sh) |
| Cutting a release (tag push -> GitHub Release, Windows only so far) | [`.github/workflows/release.yml`](.github/workflows/release.yml) |

## The 5 facts that are most expensive to rediscover

1. **Never diff the cursor's absolute position (the hook's `pt`) across a
   span where input is being suppressed/injected** — Windows doesn't
   commit the position in that case, and the diff turns into noise. Use
   Raw Input (`WM_INPUT`/`RAWMOUSE`) for a true relative delta. (real bug,
   see known-issues.md)
2. **`Ctrl+Alt+V` only suppresses `V`** — Ctrl/Alt keep being
   forwarded/injected as ordinary keys. Any synthetic key injection that
   depends on a "clean" modifier state needs to release Ctrl/Alt/Shift
   first.
3. **Windows clipboard images almost always arrive as `BI_BITFIELDS`**,
   not `BI_RGB` — don't assume the simplest format.
4. **Non-modifier keys auto-repeat on Windows**; a hotkey handler needs to
   dedupe by physical press, not by key event, or it fires multiple
   concurrent times while held.
5. **Elevated windows/processes (UAC, antivirus) don't respond to
   `SendInput` from a lower-privilege process** — that's Windows (UIPI),
   not a bug in this code; the same limitation exists in Synergy/Barrier.

## Pure logic vs. syscalls — where things live

Repo convention: OS-independent logic lives in a file **without** a build
tag (testable on any platform); anything calling an OS API is isolated in
`_windows.go`/`_linux.go`/`_other.go` with the same function signature on
each side. Canonical example: the edge-switching math lives in
`cmd/controller/edge_math.go` (no build tag, tested), separate from the
Windows hook in `edge_windows.go`. When adding new logic that involves a
syscall, consider whether the decision/parsing part can be extracted the
same way.

## Parallel-build convention

When testing a change that could break a deployment already in use (two
machines connected live), build under a suffix instead of overwriting the
binary in use: `./scripts/build.sh --suffix 2` produces
`target2.exe`/`controller2.exe`/`tray2.exe` alongside the originals. This
isn't a permanent project convention — it's just to avoid dropping
whoever's already connected while testing something new.
