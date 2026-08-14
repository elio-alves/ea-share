# ea-share — reverse KVM

Keyboard/mouse sharing "in reverse": instead of the machine hosting the
physical keyboard acting as the server (Synergy/Barrier), here it's
whichever machine **connects** that shares its keyboard/mouse.

- **`target`** — the machine being controlled. Listens, accepts an
  authenticated connection, and injects the events it receives locally.
- **`controller`** — the controlling machine. Connects to the `target`,
  captures the *local* keyboard/mouse, and sends the events over the
  network.
- **`tray`** — a Windows system-tray icon to start `target`/`controller`
  from saved profiles, no terminal needed (see
  [System tray icon (`tray`)](#system-tray-icon-tray)).

## Responsible use

This is a remote-access tool — use it only between machines you own (or
with the explicit authorization of the owner), the same way you'd use
SSH, TeamViewer, or Synergy. Whoever holds the token can type and click on
the `target` machine with no further confirmation; treat it like a
password. See [Security model](#security-model) before exposing this on a
network you don't control.

## Build

```sh
go build -o bin/target.exe ./cmd/target        # Windows
go build -o bin/controller.exe ./cmd/controller
go build -ldflags "-H=windowsgui" -o bin/tray.exe ./cmd/tray   # no console on launch

GOOS=linux GOARCH=amd64 go build -o bin/target ./cmd/target   # cross-compile for Linux
GOOS=linux GOARCH=amd64 go build -o bin/controller ./cmd/controller
```

Or use `scripts/build.sh` (works run from Windows via Git Bash or from
Linux/CI — see [`scripts/build.sh`](scripts/build.sh)):

```sh
./scripts/build.sh              # target/controller (windows+linux) + tray (windows)
./scripts/build.sh --suffix 2   # produces target2.exe/controller2.exe/tray2.exe, without overwriting a build already in use
```

No dependency uses CGO — no gcc/mingw toolchain needed to build for
Windows or Linux, nor to cross-compile from one OS to the other. macOS is
**not supported yet** (see [Known limitations](#known-limitations)).

## Usage

On the machine that will be controlled (`target`):

```sh
./target -listen :7777
```

On first run it generates a self-signed certificate and, if you don't
pass `-token`, a random token — both are printed to the terminal:

```
No -token given; generated one for this session:
  4fa94c92cf6e96a878652410cb58fdb2b769beb7b88ac859

Certificate fingerprint (verify this matches on the controller):
  D4:83:C3:D3:DB:17:4A:57:...
```

On the controlling machine (`controller`), copying the token printed
above:

```sh
./controller -connect 192.168.1.50:7777 -token 4fa94c92cf6e96a878652410cb58fdb2b769beb7b88ac859
```

On the first connection to an unknown `target`, the `controller` shows
the certificate fingerprint and asks for confirmation (like SSH's
`known_hosts`) before trusting it. After that, the fingerprint is saved
and checked automatically on every reconnection — if it changes, the
connection is refused (protection against man-in-the-middle attacks).

Without `-edge`, the `controller`'s keyboard and mouse are reflected
*continuously* on the `target` (legacy "always share" mode). `Ctrl+C`
shuts down both sides.

### Edge-triggered switching (`-edge`)

With `-edge left|right|top|bottom`, the `controller` behaves like
Synergy/Barrier: the local keyboard/mouse work normally until the cursor
touches the configured screen edge — from there it starts being captured
and forwarded to the `target`, with the cursor "entering" from the
opposite edge over there. Pushing back through the same edge returns
control locally.

```sh
./controller -connect 192.168.1.50:7777 -token ... -edge right   # target sits to the right, physically
```

`-edge` is the side of the screen **where the target sits**, from the
point of view of whoever's sitting at the `controller`.

### Shared clipboard (`Ctrl+Alt+V`)

With `-edge` active, `Ctrl+Alt+V` transfers the clipboard (text or image,
e.g. a screenshot) between the two machines — the direction is decided
automatically from the current state:

- **Controlling the target** (engaged) → `Ctrl+Alt+V` pushes the
  `controller`'s clipboard to the `target` and pastes it there.
- **Local control** (disengaged) → `Ctrl+Alt+V` fetches the `target`'s
  clipboard and pastes it here on the `controller`.

Regular `Ctrl+C`/`Ctrl+V` keep working normally on each machine (the key
is just forwarded like any other) — the special hotkey only swaps the
*clipboard content* between the two ends right before pasting. It uses a
TCP connection separate from the mouse/keyboard one (one port up), so a
large screenshot never delays mouse movement.

### Main flags

| Flag (target) | Description |
|---|---|
| `-listen` | address:port to listen on (default `:7777`) |
| `-token` | shared secret (or env `KBS_TOKEN`); generated if empty |
| `-data-dir` | where to store the TLS certificate |
| `-verbose` | periodically logs how many events of each kind were received |

| Flag (controller) | Description |
|---|---|
| `-connect` | `host:port` of the target (required) |
| `-token` | shared secret (or env `KBS_TOKEN`) |
| `-edge` | `left\|right\|top\|bottom`: enables edge switching + clipboard; without it, always shares (legacy mode) |
| `-fingerprint` | pins the expected target fingerprint, skipping the prompt |
| `-yes` | automatically trusts an unknown target, without asking |
| `-known-hosts` | path to the trusted-fingerprints file |

## Security model

- The connection is always TLS. Since there's no CA, trust is by
  **fingerprint pinning** (trust-on-first-use), like SSH — not by a
  validated certificate chain. The clipboard channel (when `-edge` is
  active) is a second TLS connection to the same destination, with the
  fingerprint already trusted from the first one — not a separate trust
  decision.
- After the TLS handshake, the `controller` must send the correct
  **token** before the `target` accepts any input event. The comparison
  is constant-time.
- The `target` only accepts **one controller at a time**; extra
  connections are refused while one is already active.
- **Treat the token like a root password**: whoever has it can type and
  click remotely on the `target` machine with no further confirmation.
  Don't hardcode the token in shared scripts; prefer passing it via
  `KBS_TOKEN` in the environment.
- On an untrusted network (open internet), run this behind a VPN
  (WireGuard/Tailscale) instead of exposing the port directly.

## Required permissions

- **Windows**: no special permission for capture/injection via
  `SetWindowsHookExW`/`SendInput` under normal use. See
  [Known limitations](#known-limitations) about elevated
  windows/processes (e.g. antivirus) — running as Administrator helps
  partially in that case.
- **Linux**:
  - `controller` (capture): needs to read `/dev/input/event*`, which
    normally requires root or being part of the `input` group
    (`sudo usermod -aG input $USER`, then re-open the session).
  - `target` (injection): needs access to `/dev/uinput`
    (`sudo modprobe uinput`; a `udev` rule or `uinput`/`input` group
    depending on the distro).
  - `-edge` and the shared clipboard aren't implemented on Linux yet
    (see [Known limitations](#known-limitations)).

## System tray icon (`tray`)

`tray.exe` is a graphical way to use `target`/`controller` without
opening a terminal: an icon sits in the Windows system tray (near the
clock) with a menu to start/stop each one from **saved profiles**.

- On first run, `tray.exe` creates `%AppData%\kbs\tray_profiles.json`
  with one example profile of each kind. Edit that file (**Edit profiles
  (notepad)** menu, then **Reload profiles**) to add your own machines:
  `name`, `listen`/`token` for targets; `name`, `connect`/`token`/`edge`
  for controllers (empty `edge` = legacy mode, always share, no edge
  switching).
- **Listen as target** / **Connect to** in the menu start the
  corresponding process (hidden, no console window) using `target.exe` /
  `controller.exe` — which need to be **in the same folder** as
  `tray.exe`.
- **Stop target** / **Stop controller** end the running process.
- **Copy target token** copies it to the clipboard (to paste when
  creating the controller's profile on the other machine).
- **Language** switches the menu between English and Português, saved to
  the same profile file (`"lang": "en"` or `"pt"`).
- Connections made by `tray` automatically trust an unknown target's
  certificate on first connect (equivalent to `-yes`, since there's no
  terminal to confirm the fingerprint) — after that the fingerprint is
  pinned normally, so a target reinstall or a real MITM still breaks the
  connection.
- Diagnostic log (the output of every `target`/`controller` started by
  the tray) lives at `%AppData%\kbs\tray.log`.

## Known limitations

- **macOS not implemented.** Capture/injection on macOS require CGO
  (Quartz `CGEventTap`/`CGEventPost`), out of scope for this first
  version, which builds without a C toolchain.
- **`-edge` and the shared clipboard are Windows-only.** On Linux,
  `controller`/`target` only work in legacy mode (always share).
- **One direction at a time.** The `target` doesn't send its own
  keyboard/mouse back to the `controller` either — bidirectional support
  is a possible future addition.
- **No local input blocking on the target.** While being controlled, the
  `target` still responds to its own physical keyboard/mouse too (they
  can collide).
- **Keyboard layout**: the mapped key set covers letters, numbers,
  function keys (F1-F12), navigation, modifiers, and common US-layout
  punctuation. Accent/language-specific keys (ç, á, dead keys, etc.)
  aren't mapped yet.
- No support for drag-and-drop file transfer or multiple
  monitors/displays — it's just keyboard, mouse (relative movement,
  buttons, and vertical scroll), and the clipboard (text/image) described
  above.
- **Elevated windows/processes on Windows (e.g. an antivirus's tray icon)
  may not respond to injected input.** This is a Windows protection
  (UIPI) against synthetic input from a lower-privilege process affecting
  a higher-privilege one — the same limitation exists in Synergy, Barrier,
  and Microsoft's Mouse Without Borders. Running `target`/`tray` as
  Administrator fixes it for common elevated windows (UAC); security
  software running at an even higher level (close to SYSTEM) may remain
  immune by design. See [`docs/known-issues.md`](docs/known-issues.md).

## Structure

```
cmd/target/            binary that accepts a connection and injects events
cmd/controller/        binary that connects and captures local events
cmd/tray/               Windows system tray icon (saved profiles, no terminal)
internal/protocol/     mouse/keyboard wire message format
internal/clipsync/      shared-clipboard wire format + dedicated connection
internal/keys/          OS-independent key names + mappings
internal/capture/       input capture (Windows/Linux/darwin-stub)
internal/inject/        input injection (Windows/Linux/darwin-stub)
internal/clipboard/      Windows clipboard read/write (text+image)
internal/tlsutil/        self-signed certificate + trust-on-first-use
internal/auth/           constant-time token check
docs/                    architecture, known-issue, and testing details
scripts/build.sh          cross-platform build script
```

## Development

- [`ai-context.md`](ai-context.md) — entry point for anyone (or any AI)
  working on the project day to day.
- [`docs/architecture.md`](docs/architecture.md) — how the pieces fit
  together.
- [`docs/known-issues.md`](docs/known-issues.md) — non-obvious bugs
  already found and why, so they don't get rediscovered from scratch.
- [`docs/testing.md`](docs/testing.md) — what has automated test
  coverage, what doesn't (and why), how to run it.

## License

[MIT](LICENSE)
