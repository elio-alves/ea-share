//go:build windows

package main

import (
	"fmt"
	"log"
	"strings"
	"sync"

	"fyne.io/systray"
)

// connState is how far along a running target/controller's connection
// currently is - used to show "listening" vs. "connected (TCP)" vs.
// "connected (TCP+UDP)" in the tray instead of just a bare running/not
// running checkmark. Derived by pattern-matching the process's own log
// lines (see startTarget/startController's onLine) since neither binary
// exposes this over any other channel today.
type connState int

const (
	connNone    connState = iota // process not running at all
	connWaiting                  // running, no peer connected/authenticated yet
	connTCP                      // connected, TCP only
	connUDP                      // connected, TCP + the UDP mouse channel
)

// app holds all tray state: the loaded profiles and whichever target/
// controller process is currently running (at most one of each at a time).
type app struct {
	mu      sync.Mutex
	cfg     Config
	cfgPath string

	runningTarget     *runningProc
	runningTargetName string
	runningTargetConn connState
	runningCtrl       *runningProc
	runningCtrlName   string
	runningCtrlConn   connState

	editorOpen bool // guards against opening a second profile editor window
}

// targetConnLabel/ctrlConnLabel turn a connState into the short status
// text shown next to a running profile's name.
func targetConnLabel(s connState) string {
	switch s {
	case connWaiting:
		return tr("status.target_listening")
	case connTCP:
		return tr("status.connected_tcp")
	case connUDP:
		return tr("status.connected_udp")
	default:
		return ""
	}
}

func ctrlConnLabel(s connState) string {
	switch s {
	case connWaiting:
		return tr("status.ctrl_connecting")
	case connTCP:
		return tr("status.connected_tcp")
	case connUDP:
		return tr("status.connected_udp")
	default:
		return ""
	}
}

// setTargetConn/upgradeTargetConnAtLeastTCP update the target's
// connection state and redraw the menu. The "upgrade" variant is for the
// "authenticated" log line, which always fires - but for a UDP-mouse
// connection it fires *after* the "UDP mouse channel enabled" line (see
// cmd/target/main.go:handleConn), so it must never downgrade an
// already-detected connUDP back down to connTCP.
func (a *app) setTargetConn(s connState) {
	a.mu.Lock()
	a.runningTargetConn = s
	a.mu.Unlock()
	a.rebuild()
}

func (a *app) upgradeTargetConnAtLeastTCP() {
	a.mu.Lock()
	if a.runningTargetConn == connWaiting || a.runningTargetConn == connNone {
		a.runningTargetConn = connTCP
	}
	a.mu.Unlock()
	a.rebuild()
}

func (a *app) setCtrlConn(s connState) {
	a.mu.Lock()
	a.runningCtrlConn = s
	a.mu.Unlock()
	a.rebuild()
}

func (a *app) upgradeCtrlConnAtLeastTCP() {
	a.mu.Lock()
	if a.runningCtrlConn == connWaiting || a.runningCtrlConn == connNone {
		a.runningCtrlConn = connTCP
	}
	a.mu.Unlock()
	a.rebuild()
}

func (a *app) onReady() {
	systray.SetIcon(generateIcon())
	systray.SetTooltip("kbs")
	a.rebuild()
}

func (a *app) onExit() {
	a.mu.Lock()
	t, c := a.runningTarget, a.runningCtrl
	a.mu.Unlock()
	if t != nil {
		t.Stop()
	}
	if c != nil {
		c.Stop()
	}
}

// rebuild tears down and redraws the whole tray menu from current state.
// Called after every state change (profiles reloaded, process started or
// stopped) instead of trying to patch individual items in place.
func (a *app) rebuild() {
	a.mu.Lock()
	cfg := a.cfg
	runningTargetName := a.runningTargetName
	runningTargetConn := a.runningTargetConn
	runningCtrlName := a.runningCtrlName
	runningCtrlConn := a.runningCtrlConn
	a.mu.Unlock()

	systray.ResetMenu()

	var statusParts []string
	if runningTargetName != "" {
		statusParts = append(statusParts, tr("status.listening_as", fmt.Sprintf("%s — %s", runningTargetName, targetConnLabel(runningTargetConn))))
	}
	if runningCtrlName != "" {
		statusParts = append(statusParts, tr("status.controlling", fmt.Sprintf("%s — %s", runningCtrlName, ctrlConnLabel(runningCtrlConn))))
	}
	status := tr("status.stopped")
	if len(statusParts) > 0 {
		status = strings.Join(statusParts, " · ")
	}
	statusItem := systray.AddMenuItem("kbs — "+status, "")
	statusItem.Disable()
	systray.SetTooltip("kbs — " + status)

	systray.AddSeparator()

	targetMenu := systray.AddMenuItem(tr("menu.listen_as_target"), "")
	if len(cfg.Targets) == 0 {
		targetMenu.Disable()
	}
	for _, tp := range cfg.Targets {
		running := runningTargetName == tp.Name
		label := fmt.Sprintf("%s  (%s)", tp.Name, tp.Listen)
		if running {
			label = fmt.Sprintf("%s  — %s", tp.Name, targetConnLabel(runningTargetConn))
		}
		item := targetMenu.AddSubMenuItem(label, "")
		if running {
			item.Check()
		}
		go func() {
			for range item.ClickedCh {
				// Toggle: clicking the profile that's currently running
				// stops it instead of restarting it - same switch-like
				// behavior as "Stop target", just reachable from the same
				// item you started it from.
				a.mu.Lock()
				isRunning := a.runningTargetName == tp.Name
				a.mu.Unlock()
				if isRunning {
					a.stopTarget()
				} else {
					a.startTarget(tp)
				}
			}
		}()
	}

	ctrlMenu := systray.AddMenuItem(tr("menu.connect_to"), "")
	if len(cfg.Controllers) == 0 {
		ctrlMenu.Disable()
	}
	for _, cp := range cfg.Controllers {
		running := runningCtrlName == cp.Name
		edgeLabel := cp.Edge
		if edgeLabel == "" {
			edgeLabel = tr("edge.always_share")
		}
		label := fmt.Sprintf("%s  (%s)", cp.Name, edgeLabel)
		if running {
			label = fmt.Sprintf("%s  — %s", cp.Name, ctrlConnLabel(runningCtrlConn))
		}
		item := ctrlMenu.AddSubMenuItem(label, "")
		if running {
			item.Check()
		}
		go func() {
			for range item.ClickedCh {
				a.mu.Lock()
				isRunning := a.runningCtrlName == cp.Name
				a.mu.Unlock()
				if isRunning {
					a.stopController()
				} else {
					a.startController(cp)
				}
			}
		}()
	}

	systray.AddSeparator()

	stopTarget := systray.AddMenuItem(tr("menu.stop_target"), "")
	if runningTargetName == "" {
		stopTarget.Disable()
	}
	go func() {
		for range stopTarget.ClickedCh {
			a.stopTarget()
		}
	}()

	stopCtrl := systray.AddMenuItem(tr("menu.stop_controller"), "")
	if runningCtrlName == "" {
		stopCtrl.Disable()
	}
	go func() {
		for range stopCtrl.ClickedCh {
			a.stopController()
		}
	}()

	systray.AddSeparator()

	copyToken := systray.AddMenuItem(tr("menu.copy_token"), "")
	if len(cfg.Targets) == 0 {
		copyToken.Disable()
	}
	go func() {
		for range copyToken.ClickedCh {
			a.mu.Lock()
			targets := a.cfg.Targets
			name := a.runningTargetName
			a.mu.Unlock()
			token := ""
			for _, tp := range targets {
				if tp.Name == name || (name == "" && token == "") {
					token = tp.Token
				}
			}
			if token != "" {
				if err := setClipboardText(token); err != nil {
					log.Printf("copy token: %v", err)
				}
			}
		}
	}()

	editCfg := systray.AddMenuItem(tr("menu.edit_profiles"), "")
	go func() {
		for range editCfg.ClickedCh {
			a.openProfileEditor()
		}
	}()

	reload := systray.AddMenuItem(tr("menu.reload_profiles"), "")
	go func() {
		for range reload.ClickedCh {
			a.reloadConfig()
		}
	}()

	// Language names are shown in their own language regardless of the
	// current UI language (standard convention for a language picker) -
	// someone who can't read the current language can still find their
	// own by name.
	langMenu := systray.AddMenuItem(tr("menu.language"), "")
	langEnItem := langMenu.AddSubMenuItem("English", "")
	langPtItem := langMenu.AddSubMenuItem("Português", "")
	if currentLang == langEN {
		langEnItem.Check()
	} else {
		langPtItem.Check()
	}
	go func() {
		for range langEnItem.ClickedCh {
			a.setLang(langEN)
		}
	}()
	go func() {
		for range langPtItem.ClickedCh {
			a.setLang(langPT)
		}
	}()

	systray.AddSeparator()
	quit := systray.AddMenuItem(tr("menu.quit"), "")
	go func() {
		for range quit.ClickedCh {
			systray.Quit()
		}
	}()
}

// setLang switches the UI language, persists the choice to the profile
// file, and redraws the menu with the new strings.
func (a *app) setLang(l lang) {
	setLang(l)
	a.mu.Lock()
	a.cfg.Lang = string(l)
	cfg := a.cfg
	path := a.cfgPath
	a.mu.Unlock()
	if err := saveConfig(path, cfg); err != nil {
		log.Printf("saving language preference: %v", err)
	}
	a.rebuild()
}

func (a *app) startTarget(tp TargetProfile) {
	a.mu.Lock()
	prev := a.runningTarget
	a.runningTarget = nil
	a.runningTargetName = ""
	a.mu.Unlock()
	if prev != nil {
		prev.Stop()
	}

	args := []string{"-listen", tp.Listen}
	if tp.Token != "" {
		args = append(args, "-token", tp.Token)
	}

	awaitingGeneratedToken := tp.Token == ""
	onLine := func(line string) {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			return
		}
		if awaitingGeneratedToken {
			if strings.Contains(trimmed, "generated one for this session") {
				return // token itself is the next non-empty line
			}
			if isLikelyToken(trimmed) {
				awaitingGeneratedToken = false
				a.saveGeneratedTargetToken(tp.Name, trimmed)
			}
		}
		switch {
		case strings.Contains(trimmed, "UDP mouse channel enabled"):
			a.setTargetConn(connUDP)
		case strings.Contains(trimmed, "authenticated, now controlling this machine"):
			a.upgradeTargetConnAtLeastTCP()
		case strings.Contains(trimmed, "disconnected"),
			strings.Contains(trimmed, "auth failed"),
			strings.Contains(trimmed, "awaiting auth"):
			a.setTargetConn(connWaiting)
		}
		log.Printf("[target %s] %s", tp.Name, trimmed)
		systray.SetTooltip(fmt.Sprintf("kbs — target %s: %s", tp.Name, trimmed))
	}
	onExit := func(err error) {
		if err != nil {
			log.Printf("[target %s] exited: %v", tp.Name, err)
		}
		a.mu.Lock()
		if a.runningTargetName == tp.Name {
			a.runningTarget = nil
			a.runningTargetName = ""
			a.runningTargetConn = connNone
		}
		a.mu.Unlock()
		a.rebuild()
	}

	proc, err := startProcess(siblingBinaryName("ea-share-target"), args, onLine, onExit)
	if err != nil {
		log.Printf("start target %s: %v", tp.Name, err)
		return
	}

	a.mu.Lock()
	a.runningTarget = proc
	a.runningTargetName = tp.Name
	a.runningTargetConn = connWaiting
	a.mu.Unlock()
	a.rebuild()
}

func (a *app) stopTarget() {
	a.mu.Lock()
	proc := a.runningTarget
	a.runningTarget = nil
	a.runningTargetName = ""
	a.runningTargetConn = connNone
	a.mu.Unlock()
	if proc != nil {
		proc.Stop()
	}
	a.rebuild()
}

func (a *app) startController(cp ControllerProfile) {
	a.mu.Lock()
	prev := a.runningCtrl
	a.runningCtrl = nil
	a.runningCtrlName = ""
	a.mu.Unlock()
	if prev != nil {
		prev.Stop()
	}

	// -yes: a tray subprocess has no console to prompt on for the
	// trust-on-first-use confirmation, so unknown targets are trusted
	// automatically. The fingerprint is still pinned afterwards, same as
	// the interactive flow, so a later mismatch (real MITM, or the target
	// reinstalled) still hard-fails the connection.
	args := []string{"-connect", cp.Connect, "-token", cp.Token, "-yes"}
	if cp.Edge != "" {
		args = append(args, "-edge", cp.Edge)
	}
	if cp.UDPMouse {
		args = append(args, "-udp-mouse")
	}

	onLine := func(line string) {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			return
		}
		switch {
		case strings.Contains(trimmed, "UDP mouse channel active"):
			a.setCtrlConn(connUDP)
		case strings.Contains(trimmed, "Connected and authenticated to"):
			a.upgradeCtrlConnAtLeastTCP()
		}
		log.Printf("[controller %s] %s", cp.Name, trimmed)
		systray.SetTooltip(fmt.Sprintf("kbs — %s: %s", cp.Name, trimmed))
	}
	onExit := func(err error) {
		if err != nil {
			log.Printf("[controller %s] exited: %v", cp.Name, err)
		}
		a.mu.Lock()
		if a.runningCtrlName == cp.Name {
			a.runningCtrl = nil
			a.runningCtrlName = ""
			a.runningCtrlConn = connNone
		}
		a.mu.Unlock()
		a.rebuild()
	}

	proc, err := startProcess(siblingBinaryName("ea-share-controller"), args, onLine, onExit)
	if err != nil {
		log.Printf("start controller %s: %v", cp.Name, err)
		return
	}

	a.mu.Lock()
	a.runningCtrl = proc
	a.runningCtrlName = cp.Name
	a.runningCtrlConn = connWaiting
	a.mu.Unlock()
	a.rebuild()
}

func (a *app) stopController() {
	a.mu.Lock()
	proc := a.runningCtrl
	a.runningCtrl = nil
	a.runningCtrlName = ""
	a.runningCtrlConn = connNone
	a.mu.Unlock()
	if proc != nil {
		proc.Stop()
	}
	a.rebuild()
}

func (a *app) reloadConfig() {
	cfg, path, err := loadConfig()
	if err != nil {
		log.Printf("reload config: %v", err)
		return
	}
	setLang(lang(cfg.Lang))
	a.mu.Lock()
	a.cfg = cfg
	a.cfgPath = path
	a.mu.Unlock()
	a.rebuild()
}

func (a *app) saveGeneratedTargetToken(name, token string) {
	a.mu.Lock()
	for i := range a.cfg.Targets {
		if a.cfg.Targets[i].Name == name {
			a.cfg.Targets[i].Token = token
		}
	}
	cfg := a.cfg
	path := a.cfgPath
	a.mu.Unlock()
	if err := saveConfig(path, cfg); err != nil {
		log.Printf("saving generated token: %v", err)
	}
}

// isLikelyToken reports whether s looks like the hex token ea-share-target
// prints on its own line right after announcing it generated one.
func isLikelyToken(s string) bool {
	if len(s) < 16 {
		return false
	}
	for _, r := range s {
		if !strings.ContainsRune("0123456789abcdefABCDEF", r) {
			return false
		}
	}
	return true
}
