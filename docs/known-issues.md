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

## `tray.log` doesn't capture `tray.exe`'s own crash

**Status: identified, not yet fixed.**

`tray.log` only captures child-process (`target`/`controller`) output via
the `log` package's `log.SetOutput`. A panic/crash in `tray.exe` itself
(which runs without a console, `-H=windowsgui`) goes nowhere — the last
line in the log before it disappears is always from a child process,
never from the tray itself. This happened for real during an overnight
sleep/hibernate: the child `target` process exited with an error, the
tray hit an internal systray tooltip error the same second, and then
nothing — the process was gone.

**Proposed mitigation** (not implemented): redirect the process's real
stdout/stderr handles (not just `log.SetOutput`, which only covers the
`log` package) to the log file right at the start of `main()`, plus a
size cap/rotation.
