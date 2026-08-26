//go:build windows

// The declarative sub-package of walk is dot-imported here on purpose:
// its nested-struct syntax (MainWindow{Children: []Widget{...}}) is how
// every walk app builds a widget tree, and spelling out
// declarative.MainWindow/declarative.TabPage/... on every line would add
// noise without adding clarity. Kept confined to this one file.
package main

import (
	"errors"
	"log"
	"runtime"
	"strings"

	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
)

// edgeValues are the Edge values a controller profile can hold, in the
// same order as the ComboBox options built from them; index 0 (empty
// string) means legacy always-share mode.
var edgeValues = []string{"", "left", "right", "top", "bottom"}

func edgeLabels() []string {
	return []string{tr("edge.always_share"), "left", "right", "top", "bottom"}
}

func indexOfEdge(v string) int {
	for i, ev := range edgeValues {
		if ev == v {
			return i
		}
	}
	return 0
}

// openProfileEditor opens the visual profile editor window, replacing
// the old "open tray_profiles.json in notepad" flow. It runs on its own
// locked OS thread (Win32 windows are thread-affine) and doesn't
// interfere with systray's own message loop running on the main
// goroutine (see main.go, systray.Run). editorOpen (guarded by a.mu)
// stops a second window from being opened while one is already up.
func (a *app) openProfileEditor() {
	a.mu.Lock()
	if a.editorOpen {
		a.mu.Unlock()
		return
	}
	a.editorOpen = true
	cfg := cloneConfig(a.cfg)
	a.mu.Unlock()

	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()

		e := &profileEditor{app: a, workingCfg: cfg}
		e.run()

		a.mu.Lock()
		a.editorOpen = false
		a.mu.Unlock()
	}()
}

// cloneConfig deep-copies cfg's slices so editing the copy never mutates
// the running app's own a.cfg before Save is actually clicked.
func cloneConfig(cfg Config) Config {
	clone := cfg
	clone.Targets = append([]TargetProfile(nil), cfg.Targets...)
	clone.Controllers = append([]ControllerProfile(nil), cfg.Controllers...)
	return clone
}

// profileEditor is a master-detail window: each tab (Targets,
// Controllers) has a ListBox of profile names on the left and a form
// with that profile's fields on the right. It edits workingCfg, a
// private copy of Config (see cloneConfig) - nothing reaches disk or the
// running tray's a.cfg until Save/Save & restart running is clicked.
type profileEditor struct {
	app        *app
	workingCfg Config
	mw         *walk.MainWindow

	targetList     *walk.ListBox
	targetSelected int
	targetName     *walk.LineEdit
	targetListen   *walk.LineEdit
	targetToken    *walk.LineEdit

	ctrlList     *walk.ListBox
	ctrlSelected int
	ctrlName     *walk.LineEdit
	ctrlConnect  *walk.LineEdit
	ctrlToken    *walk.LineEdit
	ctrlEdge     *walk.ComboBox
	ctrlUDP      *walk.CheckBox
}

func (e *profileEditor) run() {
	e.targetSelected = -1
	e.ctrlSelected = -1

	mww := MainWindow{
		AssignTo: &e.mw,
		Title:    tr("editor.title"),
		MinSize:  Size{Width: 520, Height: 360},
		Size:     Size{Width: 640, Height: 440},
		Layout:   VBox{},
		Children: []Widget{
			TabWidget{
				Pages: []TabPage{
					e.targetsPage(),
					e.controllersPage(),
				},
			},
			Composite{
				Layout: HBox{},
				Children: []Widget{
					HSpacer{},
					PushButton{
						Text:      tr("editor.close"),
						OnClicked: func() { e.mw.Close() },
					},
					PushButton{
						Text:      tr("editor.save"),
						OnClicked: func() { e.doSave() },
					},
					PushButton{
						Text:      tr("editor.save_restart"),
						OnClicked: func() { e.doSaveAndRestart() },
					},
				},
			},
		},
	}

	if err := mww.Create(); err != nil {
		log.Printf("profile editor: creating window: %v", err)
		return
	}

	if len(e.workingCfg.Targets) > 0 {
		e.refreshTargetList(0)
	}
	if len(e.workingCfg.Controllers) > 0 {
		e.refreshCtrlList(0)
	}

	e.mw.Run()
}

func (e *profileEditor) targetsPage() TabPage {
	return TabPage{
		Title:  tr("editor.tab_targets"),
		Layout: HBox{},
		Children: []Widget{
			Composite{
				Layout:        VBox{},
				StretchFactor: 1,
				Children: []Widget{
					ListBox{
						AssignTo: &e.targetList,
						MinSize:  Size{Width: 140, Height: 100},
						OnCurrentIndexChanged: func() {
							e.commitTarget()
							e.loadTarget(e.targetList.CurrentIndex())
						},
					},
					Composite{
						Layout: HBox{},
						Children: []Widget{
							PushButton{
								Text: tr("editor.add"),
								OnClicked: func() {
									e.commitTarget()
									e.workingCfg.Targets = append(e.workingCfg.Targets, TargetProfile{
										Name:   tr("editor.new_target"),
										Listen: ":7777",
									})
									e.refreshTargetList(len(e.workingCfg.Targets) - 1)
								},
							},
							PushButton{
								Text:      tr("editor.remove"),
								OnClicked: func() { e.removeTarget() },
							},
						},
					},
				},
			},
			Composite{
				Layout:        Grid{Columns: 2},
				StretchFactor: 2,
				Children: []Widget{
					Label{Text: tr("editor.field_name")},
					LineEdit{AssignTo: &e.targetName},

					Label{Text: tr("editor.field_listen")},
					LineEdit{AssignTo: &e.targetListen},

					Label{Text: tr("editor.field_token")},
					LineEdit{AssignTo: &e.targetToken},
				},
			},
		},
	}
}

func (e *profileEditor) controllersPage() TabPage {
	return TabPage{
		Title:  tr("editor.tab_controllers"),
		Layout: HBox{},
		Children: []Widget{
			Composite{
				Layout:        VBox{},
				StretchFactor: 1,
				Children: []Widget{
					ListBox{
						AssignTo: &e.ctrlList,
						MinSize:  Size{Width: 140, Height: 100},
						OnCurrentIndexChanged: func() {
							e.commitCtrl()
							e.loadCtrl(e.ctrlList.CurrentIndex())
						},
					},
					Composite{
						Layout: HBox{},
						Children: []Widget{
							PushButton{
								Text: tr("editor.add"),
								OnClicked: func() {
									e.commitCtrl()
									e.workingCfg.Controllers = append(e.workingCfg.Controllers, ControllerProfile{
										Name: tr("editor.new_controller"),
									})
									e.refreshCtrlList(len(e.workingCfg.Controllers) - 1)
								},
							},
							PushButton{
								Text:      tr("editor.remove"),
								OnClicked: func() { e.removeCtrl() },
							},
						},
					},
				},
			},
			Composite{
				Layout:        Grid{Columns: 2},
				StretchFactor: 2,
				Children: []Widget{
					Label{Text: tr("editor.field_name")},
					LineEdit{AssignTo: &e.ctrlName},

					Label{Text: tr("editor.field_connect")},
					LineEdit{AssignTo: &e.ctrlConnect},

					Label{Text: tr("editor.field_token")},
					LineEdit{AssignTo: &e.ctrlToken},

					Label{Text: tr("editor.field_edge")},
					ComboBox{
						AssignTo: &e.ctrlEdge,
						Model:    edgeLabels(),
					},

					Label{Text: tr("editor.field_udp_mouse")},
					CheckBox{AssignTo: &e.ctrlUDP},
				},
			},
		},
	}
}

// commitTarget/commitCtrl write the currently visible fields back into
// the working copy, at the index that was selected *before* whatever
// just happened (add/remove/switch selection) - called first, in that
// order, by every action that might change which profile is selected,
// so in-progress edits are never silently lost.

func (e *profileEditor) commitTarget() {
	if e.targetSelected < 0 || e.targetSelected >= len(e.workingCfg.Targets) {
		return
	}
	tp := &e.workingCfg.Targets[e.targetSelected]
	tp.Name = e.targetName.Text()
	tp.Listen = e.targetListen.Text()
	tp.Token = e.targetToken.Text()
}

func (e *profileEditor) loadTarget(i int) {
	e.targetSelected = i
	if i < 0 || i >= len(e.workingCfg.Targets) {
		e.targetName.SetText("")
		e.targetListen.SetText("")
		e.targetToken.SetText("")
		return
	}
	tp := e.workingCfg.Targets[i]
	e.targetName.SetText(tp.Name)
	e.targetListen.SetText(tp.Listen)
	e.targetToken.SetText(tp.Token)
}

func (e *profileEditor) refreshTargetList(selectIndex int) {
	names := make([]string, len(e.workingCfg.Targets))
	for i, tp := range e.workingCfg.Targets {
		names[i] = tp.Name
	}
	e.targetList.SetModel(names)
	if selectIndex >= 0 && selectIndex < len(names) {
		e.targetList.SetCurrentIndex(selectIndex)
	}
	e.loadTarget(selectIndex)
}

func (e *profileEditor) removeTarget() {
	i := e.targetList.CurrentIndex()
	if i < 0 || i >= len(e.workingCfg.Targets) {
		return
	}
	name := e.workingCfg.Targets[i].Name
	if walk.MsgBox(e.mw, tr("editor.confirm_remove_title"), tr("editor.confirm_remove_body", name), walk.MsgBoxYesNo|walk.MsgBoxIconWarning) != walk.DlgCmdYes {
		return
	}
	e.workingCfg.Targets = append(e.workingCfg.Targets[:i], e.workingCfg.Targets[i+1:]...)
	e.targetSelected = -1
	newSel := i
	if newSel >= len(e.workingCfg.Targets) {
		newSel = len(e.workingCfg.Targets) - 1
	}
	e.refreshTargetList(newSel)
}

func (e *profileEditor) commitCtrl() {
	if e.ctrlSelected < 0 || e.ctrlSelected >= len(e.workingCfg.Controllers) {
		return
	}
	cp := &e.workingCfg.Controllers[e.ctrlSelected]
	cp.Name = e.ctrlName.Text()
	cp.Connect = e.ctrlConnect.Text()
	cp.Token = e.ctrlToken.Text()
	if idx := e.ctrlEdge.CurrentIndex(); idx >= 0 && idx < len(edgeValues) {
		cp.Edge = edgeValues[idx]
	}
	cp.UDPMouse = e.ctrlUDP.Checked()
}

func (e *profileEditor) loadCtrl(i int) {
	e.ctrlSelected = i
	if i < 0 || i >= len(e.workingCfg.Controllers) {
		e.ctrlName.SetText("")
		e.ctrlConnect.SetText("")
		e.ctrlToken.SetText("")
		e.ctrlEdge.SetCurrentIndex(0)
		e.ctrlUDP.SetChecked(false)
		return
	}
	cp := e.workingCfg.Controllers[i]
	e.ctrlName.SetText(cp.Name)
	e.ctrlConnect.SetText(cp.Connect)
	e.ctrlToken.SetText(cp.Token)
	e.ctrlEdge.SetCurrentIndex(indexOfEdge(cp.Edge))
	e.ctrlUDP.SetChecked(cp.UDPMouse)
}

func (e *profileEditor) refreshCtrlList(selectIndex int) {
	names := make([]string, len(e.workingCfg.Controllers))
	for i, cp := range e.workingCfg.Controllers {
		names[i] = cp.Name
	}
	e.ctrlList.SetModel(names)
	if selectIndex >= 0 && selectIndex < len(names) {
		e.ctrlList.SetCurrentIndex(selectIndex)
	}
	e.loadCtrl(selectIndex)
}

func (e *profileEditor) removeCtrl() {
	i := e.ctrlList.CurrentIndex()
	if i < 0 || i >= len(e.workingCfg.Controllers) {
		return
	}
	name := e.workingCfg.Controllers[i].Name
	if walk.MsgBox(e.mw, tr("editor.confirm_remove_title"), tr("editor.confirm_remove_body", name), walk.MsgBoxYesNo|walk.MsgBoxIconWarning) != walk.DlgCmdYes {
		return
	}
	e.workingCfg.Controllers = append(e.workingCfg.Controllers[:i], e.workingCfg.Controllers[i+1:]...)
	e.ctrlSelected = -1
	newSel := i
	if newSel >= len(e.workingCfg.Controllers) {
		newSel = len(e.workingCfg.Controllers) - 1
	}
	e.refreshCtrlList(newSel)
}

// validateAndCollect commits whatever's currently visible in both tabs'
// fields, then checks the whole working config against the same rules
// the CLI binaries themselves enforce (non-empty token on controller,
// -udp-mouse requires -edge, see cmd/controller/main.go) so a bad
// profile is caught here instead of failing silently when the tray
// later tries to launch it.
func (e *profileEditor) validateAndCollect() (Config, error) {
	e.commitTarget()
	e.commitCtrl()
	cfg := e.workingCfg

	seen := map[string]bool{}
	for _, tp := range cfg.Targets {
		name := strings.TrimSpace(tp.Name)
		if name == "" {
			return Config{}, errors.New(tr("editor.error_name_required"))
		}
		if seen[name] {
			return Config{}, errors.New(tr("editor.error_name_duplicate", name))
		}
		seen[name] = true
		if strings.TrimSpace(tp.Listen) == "" {
			return Config{}, errors.New(tr("editor.error_listen_required", name))
		}
	}

	seen = map[string]bool{}
	for _, cp := range cfg.Controllers {
		name := strings.TrimSpace(cp.Name)
		if name == "" {
			return Config{}, errors.New(tr("editor.error_name_required"))
		}
		if seen[name] {
			return Config{}, errors.New(tr("editor.error_name_duplicate", name))
		}
		seen[name] = true
		if strings.TrimSpace(cp.Connect) == "" {
			return Config{}, errors.New(tr("editor.error_connect_required", name))
		}
		if strings.TrimSpace(cp.Token) == "" {
			return Config{}, errors.New(tr("editor.error_token_required", name))
		}
		if cp.UDPMouse && cp.Edge == "" {
			return Config{}, errors.New(tr("editor.error_udp_requires_edge", name))
		}
	}

	return cfg, nil
}

// doSave validates, writes to disk (config.saveConfig), and updates the
// running app's own a.cfg + menu - same effect setLang already has on
// a.cfg today (see menu.go). Returns whether it actually saved, so
// doSaveAndRestart can bail out on a validation/write failure instead of
// restarting anything against a config that was never persisted.
func (e *profileEditor) doSave() bool {
	cfg, err := e.validateAndCollect()
	if err != nil {
		walk.MsgBox(e.mw, tr("editor.error_title"), err.Error(), walk.MsgBoxIconError)
		return false
	}

	e.app.mu.Lock()
	path := e.app.cfgPath
	e.app.mu.Unlock()

	if err := saveConfig(path, cfg); err != nil {
		walk.MsgBox(e.mw, tr("editor.error_title"), err.Error(), walk.MsgBoxIconError)
		return false
	}

	e.workingCfg = cfg
	e.app.mu.Lock()
	e.app.cfg = cfg
	e.app.mu.Unlock()
	e.app.rebuild()
	return true
}

// doSaveAndRestart saves, then restarts whichever of target/controller
// is currently running using the just-saved profile with the same name
// - this is the direct fix for having to stop and start by hand from the
// tray menu to pick up an edit. a.startTarget/a.startController already
// stop whatever was running before starting the new one (see menu.go),
// so there's no separate stop call needed here.
func (e *profileEditor) doSaveAndRestart() {
	if !e.doSave() {
		return
	}

	e.app.mu.Lock()
	targetName := e.app.runningTargetName
	ctrlName := e.app.runningCtrlName
	cfg := e.app.cfg
	e.app.mu.Unlock()

	if targetName != "" {
		if tp, ok := findTargetProfile(cfg, targetName); ok {
			e.app.startTarget(tp)
		} else {
			log.Printf("profile editor: running target %q no longer exists after save, not restarting", targetName)
		}
	}
	if ctrlName != "" {
		if cp, ok := findControllerProfile(cfg, ctrlName); ok {
			e.app.startController(cp)
		} else {
			log.Printf("profile editor: running controller %q no longer exists after save, not restarting", ctrlName)
		}
	}
}

func findTargetProfile(cfg Config, name string) (TargetProfile, bool) {
	for _, tp := range cfg.Targets {
		if tp.Name == name {
			return tp, true
		}
	}
	return TargetProfile{}, false
}

func findControllerProfile(cfg Config, name string) (ControllerProfile, bool) {
	for _, cp := range cfg.Controllers {
		if cp.Name == name {
			return cp, true
		}
	}
	return ControllerProfile{}, false
}
