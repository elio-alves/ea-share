# Known bugs and why

Each entry here was a real bug, diagnosed and fixed (or documented as an
accepted limitation). Goal: don't rediscover the same root cause from
scratch the next time a similar symptom shows up.

## Mouse deltas via hook `pt`-diffing break once suppressed

**Symptom**: right after "engaged", the log showed "disengaged" almost
instantly afterward — a spurious round trip.

**Cause**: `internal/capture/capture_windows.go` computed the mouse delta
by diffing the absolute position (`pt`) between consecutive `WH_MOUSE_LL`
hook calls. As soon as the `controller` starts suppressing local input
(engaged), Windows **doesn't commit** the new cursor position — so `pt`
stops accumulating reliably, and the difference between events turns into
noise. That noise sometimes happened to satisfy the "pushed back past the
entry edge" condition a moment after engaging.

**Fix**: delta capture moved to the Windows **Raw Input** API
(`RegisterRawInputDevices` + `WM_INPUT` on a hidden window, reading
`RAWMOUSE.lLastX/lLastY`) — gives the true relative HID delta,
independent of cursor suppression/clamping. `WH_MOUSE_LL` today only
handles edge detection (needs absolute position) and
suppressing/forwarding clicks/scroll/keyboard. Same technique
Synergy/Barrier use on Windows.

**Lesson**: anything that diffs the Windows cursor position across a span
where input is being suppressed or injected is unreliable — prefer Raw
Input deltas or an explicit re-centering warp.

## `BI_BITFIELDS` unsupported when reading a clipboard image

**Symptom**: pasting a screenshot (`PrintScreen`) failed silently —
nothing arrived on the other end, and the controller's log showed
`clipboard: unsupported DIB compression 3`.

**Cause**: `internal/clipboard.ReadImagePNG` only understood `BI_RGB`
(compression 0). But `BI_BITFIELDS` (compression 3) is the format Windows
**almost always** uses for a 32bpp screen-capture bitmap — the 3 color
channel masks (R/G/B) come as 3 extra DWORDs right after the
`BITMAPINFOHEADER`, instead of a fixed byte position.

**Fix**: accept `BI_BITFIELDS` for 32bpp, read the 3 masks, and extract
each channel via `bits.TrailingZeros32(mask)` as a shift, instead of
assuming a fixed byte offset.

**Lesson**: don't assume the simplest case (`BI_RGB`) when reading real
Windows bitmap data — `BI_BITFIELDS` is the common case for 32bpp, not
the rare one.

## `Ctrl+Alt+V` pasted the wrong combo into the focused app

**Symptom**: the clipboard arrived correctly on the other end (the log
showed "received text/image, pasting"), but nothing appeared in the app.

**Cause**: the `Ctrl+Alt+V` hotkey only suppresses the `V` key — `Ctrl`
and `Alt` keep being forwarded/injected as ordinary keys. At paste time,
the synthetic `Ctrl+V` was injected **on top of** an `Alt` the app still
saw as held — i.e. the app actually received `Ctrl+Alt+V`, which isn't a
paste shortcut anywhere.

**Fix**: `injectPaste` (in `cmd/target/clipboard_windows.go` and
`cmd/controller/clipboard_windows.go`) now explicitly releases
`ControlLeft/Right`, `AltLeft/Right`, and `ShiftLeft/Right` **before**
injecting the clean `Ctrl+V`.

## Modifier key "stuck" from `V`'s auto-repeat

**Symptom**: after using the clipboard feature, the controller's keyboard
"broke" — typing normally triggered Windows shortcuts (opening windows,
etc.), and even killing the app didn't fix it.

**Cause**: `V` (unlike `Ctrl`/`Alt`, which don't repeat) auto-repeats on
Windows while held. The hotkey handler fired on **every repeat**, spawning
overlapping `injectPaste` goroutines that raced each other releasing/
pressing `Ctrl` — leaving a modifier "stuck" at the OS level.

**Fix**: only fire once per physical press (guarded by `!s.hotkeyVDown` in
`internal/capture/capture_windows.go`) + a mutex serializing
`injectPaste` on both sides, as a backstop.

**Live recovery trick** (no Windows restart needed): a physical
`Ctrl+Alt+Del` resets it — it's handled by Windows' Secure Attention
Sequence, below any user-mode hook, and the desktop switch clears the
stuck key state as a side effect. Also worth trying: pressing and
releasing the stuck modifier alone (real hardware), the On-Screen
Keyboard, or signing out and back in.

## Elevated processes' tray icons don't respond to injected input

**Symptom**: hovering the (remotely controlled) cursor over certain
Windows tray icons (e.g. an antivirus) freezes control instantly; moving
the physical mouse on the target machine unfreezes it.

**Cause**: **UIPI** (User Interface Privilege Isolation), a Windows
protection — synthetic input (`SendInput`, how `target` injects the
cursor) from a lower-privilege process is blocked from affecting UI
belonging to a higher-privilege one. Confirmed by running
`target`/`tray` as Administrator: "normal" icons stopped freezing, but
the antivirus's kept doing it — its tray component likely runs at an even
higher integrity level (near SYSTEM), by design, as tamper-resistance.

**Not a bug in this code.** Confirmed that Synergy, Barrier, and
Microsoft's own Mouse Without Borders have exactly the same limitation
with elevated/UAC windows — it's a Windows security boundary, not an
implementation gap. Running as Administrator helps with common elevation
(UAC); there's no reasonable way (or need) to go as far as SYSTEM just
for this.

## `lxn/walk` windows fail silently without an embedded manifest

**Symptom**: clicking "Edit profiles" in the tray menu did nothing
visible - no window, no error dialog, nothing. `tray.log` showed a Go
panic-style stack trace ending in `TTM_ADDTOOL failed`, from deep inside
`walk.(*WidgetBase).init` → `walk.(*ToolTip).AddTool`.

**Cause**: every `lxn/walk` widget registers itself with an internal
shared tooltip control during construction (`WidgetBase.init`). That
registration (`TTM_ADDTOOL`) silently fails unless the process has an
embedded Windows application manifest declaring a dependency on Common
Controls v6 (`Microsoft.Windows.Common-Controls`, version 6.0.0.0) -
without it, Windows loads the ancient v5 common controls, and several
modern control APIs (this one included) just fail. `ea-share-tray.exe`
never needed a manifest before this: `fyne.io/systray`'s tray icon + native
menu don't touch themed/tooltip controls at all. The moment the profile
editor window (`cmd/tray/profile_editor_windows.go`) started using real
`walk` widgets (`ListBox`, `LineEdit`, `ComboBox`, ...), the missing
manifest became a hard blocker - and because window creation runs in its
own goroutine (see "Threading" in `docs/architecture.md`), the failure
never reached the console (there isn't one, `-H=windowsgui`) and only
showed up as a `log.Printf` line in `tray.log`, easy to miss.

**Fix**: `cmd/tray/rsrc_windows_amd64.syso`, a Windows resource file
generated with [`go-winres`](https://github.com/tc-hib/go-winres)
(`go-winres simply --arch amd64 --manifest gui --out
cmd/tray/rsrc`), declaring the Common Controls v6 dependency. Go's
linker embeds any `*.syso` file sitting in a package directory
automatically - no CGO, no C toolchain, nothing to change in
`scripts/build.sh`. Confirmed via a throwaway repro package building the
exact same widget shapes (`ListBox`+`Composite{Layout: Grid}`+
`LineEdit`) with and without the `.syso` present: `TTM_ADDTOOL failed`
without it, clean window creation with it.

**Lesson**: any future `cmd/tray` code that constructs `lxn/walk`
widgets depends on `rsrc_windows_amd64.syso` staying committed and
picked up by the build. If it's ever lost/regenerated, re-run the
`go-winres` command above from the repo root - a missing manifest
produces no compile error and no crash a user can see, just a silently
swallowed goroutine failure logged to a file most people never open.

## Numeric keypad (right-side numpad) keys weren't forwarded at all

**Symptom**: the target machine ignored the numeric keypad entirely -
neither the digits nor `+ - * /` on the right-side numpad did anything,
even though the same physical keys work fine typed on the target
directly. The number row above the letters worked normally.

**Cause**: the numpad digits/operators have their own Windows virtual-key
codes (`VK_NUMPAD0`-`VK_NUMPAD9` = `0x60`-`0x69`, `VK_MULTIPLY`,
`VK_ADD`, `VK_SUBTRACT`, `VK_DECIMAL`, `VK_DIVIDE`), completely separate
from the top-row digit keys (`0x30`-`0x39`) and from `Slash`/OEM
punctuation. `internal/keys` (the OS-independent name set shared by
`capture`/`inject`) never defined names or VK/keycode mappings for them,
so `keyboardProc` in `internal/capture/capture_windows.go` saw
`known == false` for every numpad key and silently dropped the event
instead of forwarding it (while still suppressing it locally once
engaged - so it didn't leak through to the controller machine either).

**Fix, attempt 1**: added `NumPad0`-`NumPad9`, `NumPadAdd`,
`NumPadSubtract`, `NumPadMultiply`, `NumPadDivide`, `NumPadDecimal` to
`internal/keys/keys.go`, with Windows VK mappings in
`internal/keys/keys_windows.go` and Linux evdev keycode mappings in
`internal/keys/keys_linux.go`. NumPad Enter was already fine - Windows
reports it with the same `VK_RETURN` as the main Enter key, so it was
already covered by the existing `Enter` mapping.

This forwarded the keys, but the digits/decimal still didn't show up as
characters on the target - see the next entry.

**Fix, attempt 2 (the one that stuck)**: the digit/decimal VKs
(`VK_NUMPAD0`-`9`, `VK_DECIMAL`) turned out to need special handling
beyond just forwarding them - see "Numpad digits forwarded correctly but
still didn't type anything" below. `NumPad0`-`9`/`NumPadDecimal` were
removed again; the digits/decimal are now folded into the ordinary
`N0`-`N9`/`Period` names at the capture layer instead (see that entry).
`NumPadAdd`/`Subtract`/`Multiply`/`Divide` stayed as-is from attempt 1 -
they have no such ambiguity and needed no further work.

**Lesson**: `known-issues.md` aside, `internal/keys/keys.go`'s constant
list is the actual source of truth for "which keys this tool can
forward at all" - a key missing from that file is silently dropped, not
a build error, so it's easy to miss until someone notices a whole class
of keys doing nothing.

## Numpad digits forwarded correctly but still didn't type anything

**Symptom**: after the fix above, the numpad's `+ - * /` worked
immediately, but the digits and decimal point still produced nothing on
the target - even though the wire event was confirmed arriving and
`SendInput` was being called with the right `VK_NUMPAD*`/`VK_DECIMAL`
code. Toggling NumLock on the *controller* correctly switched local
capture between digit VKs and navigation VKs (Home/End/arrows/etc, which
worked fine already), but with NumLock on and genuine digit VKs being
sent, the target still typed nothing.

**Cause**: on Windows, `VK_NUMPAD0`-`9`/`VK_DECIMAL` only translate to an
actual character when NumLock is toggled **on wherever that translation
happens** - i.e. on the target doing the injecting, regardless of what
the source's NumLock was. `SendInput` with an explicit `wVk` doesn't
route around this: it delivers the raw VK, and it's still up to the
target's own keyboard-layout translation (`ToUnicode`/`TranslateMessage`
in the receiving app) to turn `VK_NUMPAD7` into `'7'`, which it silently
declines to do while the target's NumLock is off. This is also why tools
like AutoHotkey explicitly toggle NumLock on before sending numpad
digits and restore it after.

**Fix considered but rejected**: have `target`'s injector check
`GetKeyState(VK_NUMLOCK)` and synthesize a NumLock keypress to turn it on
before injecting a digit/decimal. This does work, but leaves the
target's NumLock permanently toggled on as a global, persistent side
effect on that machine (its own keyboard/LED state changes even after
the ea-share session ends), and does nothing to let the target ever go
back to reflecting arrows if the controller's NumLock later toggles off
- the two machines' NumLock states have no ongoing relationship, just a
one-time forced flip.

**Fix (actual)**: skip the whole NumLock problem by never forwarding a
separate "NumPad digit" identity in the first place. `capture_windows.go`
already receives the correctly-resolved VK from Windows (`VK_NUMPAD7` if
the controller's NumLock is on, `VK_HOME` if it's off - Windows resolves
this once, based on the controller's own NumLock, before the hook ever
sees it). Since a numpad digit and the matching top-row digit are
functionally identical for every normal use of a keyboard, `keys_windows.go`'s
`VKToName` now maps `VK_NUMPAD0`-`9`/`VK_DECIMAL` directly to the same
names as the top row (`N0`-`N9`, `Period`) instead of introducing
distinct `NumPad*` names. Injecting `N7` never depends on NumLock at all,
so the target needs no NumLock awareness whatsoever. Applied the same
fold on the Linux side (`keys_linux.go`) for consistency, even though it
wasn't separately confirmed broken there.

**Lesson**: an injected VK on Windows isn't guaranteed to produce the
character you'd expect from that VK - toggle-key-dependent VKs
(NumLock/CapsLock/ScrollLock-sensitive ones) get reinterpreted using
*whichever machine's* toggle state is active at translation time, not
the state that produced the VK originally. When two VKs are functionally
interchangeable for your purposes, prefer forwarding the
toggle-independent one over trying to keep two machines' toggle-key
states in sync.

## `tray.log` doesn't capture `ea-share-tray.exe`'s own crash

**Status: identified, not yet fixed.**

`tray.log` only captures child-process (`target`/`controller`) output via
the `log` package's `log.SetOutput`. A panic/crash in `ea-share-tray.exe`
itself (which runs without a console, `-H=windowsgui`) goes nowhere — the last
line in the log before it disappears is always from a child process,
never from the tray itself. This happened for real during an overnight
sleep/hibernate: the child `target` process exited with an error, the
tray hit an internal systray tooltip error the same second, and then
nothing — the process was gone.

**Proposed mitigation** (not implemented): redirect the process's real
stdout/stderr handles (not just `log.SetOutput`, which only covers the
`log` package) to the log file right at the start of `main()`, plus a
size cap/rotation.
