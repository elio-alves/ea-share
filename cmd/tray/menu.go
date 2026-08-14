//go:build windows

package main

import (
	"fmt"
	"log"
	"os/exec"
	"strings"
	"sync"

	"fyne.io/systray"
)

// app holds all tray state: the loaded profiles and whichever target/
// controller process is currently running (at most one of each at a time).
type app struct {
	mu      sync.Mutex
	cfg     Config
	cfgPath string

	runningTarget     *runningProc
	runningTargetName string
	runningCtrl       *runningProc
	runningCtrlName   string
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
	runningCtrlName := a.runningCtrlName
	a.mu.Unlock()

	systray.ResetMenu()

	var statusParts []string
	if runningTargetName != "" {
		statusParts = append(statusParts, "ouvindo como "+runningTargetName)
	}
	if runningCtrlName != "" {
		statusParts = append(statusParts, "controlando "+runningCtrlName)
	}
	status := "parado"
	if len(statusParts) > 0 {
		status = strings.Join(statusParts, " · ")
	}
	statusItem := systray.AddMenuItem("kbs — "+status, "")
	statusItem.Disable()
	systray.SetTooltip("kbs — " + status)

	systray.AddSeparator()

	targetMenu := systray.AddMenuItem("Ouvir como target", "")
	if len(cfg.Targets) == 0 {
		targetMenu.Disable()
	}
	for _, tp := range cfg.Targets {
		item := targetMenu.AddSubMenuItem(fmt.Sprintf("%s  (%s)", tp.Name, tp.Listen), "")
		if runningTargetName == tp.Name {
			item.Check()
		}
		go func() {
			for range item.ClickedCh {
				a.startTarget(tp)
			}
		}()
	}

	ctrlMenu := systray.AddMenuItem("Conectar em", "")
	if len(cfg.Controllers) == 0 {
		ctrlMenu.Disable()
	}
	for _, cp := range cfg.Controllers {
		edgeLabel := cp.Edge
		if edgeLabel == "" {
			edgeLabel = "sempre compartilhar"
		}
		item := ctrlMenu.AddSubMenuItem(fmt.Sprintf("%s  (%s)", cp.Name, edgeLabel), "")
		if runningCtrlName == cp.Name {
			item.Check()
		}
		go func() {
			for range item.ClickedCh {
				a.startController(cp)
			}
		}()
	}

	systray.AddSeparator()

	stopTarget := systray.AddMenuItem("Parar target", "")
	if runningTargetName == "" {
		stopTarget.Disable()
	}
	go func() {
		for range stopTarget.ClickedCh {
			a.stopTarget()
		}
	}()

	stopCtrl := systray.AddMenuItem("Parar controller", "")
	if runningCtrlName == "" {
		stopCtrl.Disable()
	}
	go func() {
		for range stopCtrl.ClickedCh {
			a.stopController()
		}
	}()

	systray.AddSeparator()

	copyToken := systray.AddMenuItem("Copiar token do target", "")
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

	editCfg := systray.AddMenuItem("Editar perfis (notepad)", "")
	go func() {
		for range editCfg.ClickedCh {
			a.mu.Lock()
			path := a.cfgPath
			a.mu.Unlock()
			if err := exec.Command("notepad.exe", path).Start(); err != nil {
				log.Printf("open notepad: %v", err)
			}
		}
	}()

	reload := systray.AddMenuItem("Recarregar perfis", "")
	go func() {
		for range reload.ClickedCh {
			a.reloadConfig()
		}
	}()

	systray.AddSeparator()
	quit := systray.AddMenuItem("Sair", "")
	go func() {
		for range quit.ClickedCh {
			systray.Quit()
		}
	}()
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
		}
		a.mu.Unlock()
		a.rebuild()
	}

	proc, err := startProcess("target2.exe", args, onLine, onExit)
	if err != nil {
		log.Printf("start target %s: %v", tp.Name, err)
		return
	}

	a.mu.Lock()
	a.runningTarget = proc
	a.runningTargetName = tp.Name
	a.mu.Unlock()
	a.rebuild()
}

func (a *app) stopTarget() {
	a.mu.Lock()
	proc := a.runningTarget
	a.runningTarget = nil
	a.runningTargetName = ""
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

	onLine := func(line string) {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			return
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
		}
		a.mu.Unlock()
		a.rebuild()
	}

	proc, err := startProcess("controller2.exe", args, onLine, onExit)
	if err != nil {
		log.Printf("start controller %s: %v", cp.Name, err)
		return
	}

	a.mu.Lock()
	a.runningCtrl = proc
	a.runningCtrlName = cp.Name
	a.mu.Unlock()
	a.rebuild()
}

func (a *app) stopController() {
	a.mu.Lock()
	proc := a.runningCtrl
	a.runningCtrl = nil
	a.runningCtrlName = ""
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

// isLikelyToken reports whether s looks like the hex token target.exe
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
