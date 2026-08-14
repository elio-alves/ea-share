//go:build windows

package main

import "fmt"

// lang is a supported tray UI language code.
type lang string

const (
	langEN lang = "en"
	langPT lang = "pt"
)

// currentLang is process-global: the tray only ever has one menu/UI at a
// time, so there's no need to thread this through every call. Set via
// setLang, read by tr.
var currentLang = langEN

func setLang(l lang) {
	if _, ok := messages[l]; ok {
		currentLang = l
	}
}

// tr looks up key in the current language, falling back to English if
// the current language is missing that key (shouldn't happen in
// practice, but keeps a typo from blanking out a menu item instead of
// just showing English).
func tr(key string, args ...any) string {
	tmpl, ok := messages[currentLang][key]
	if !ok {
		tmpl = messages[langEN][key]
	}
	if len(args) == 0 {
		return tmpl
	}
	return fmt.Sprintf(tmpl, args...)
}

// messages holds every user-facing tray string. Diagnostic log.Printf
// output (tray.log) deliberately stays English-only regardless of this
// setting - it's developer/debugging-facing, not UI, and consistent
// English keeps it easy to search/grep/paste into an issue.
var messages = map[lang]map[string]string{
	langEN: {
		"status.stopped":        "stopped",
		"status.listening_as":   "listening as %s",
		"status.controlling":    "controlling %s",
		"menu.listen_as_target": "Listen as target",
		"menu.connect_to":       "Connect to",
		"menu.stop_target":      "Stop target",
		"menu.stop_controller":  "Stop controller",
		"menu.copy_token":       "Copy target token",
		"menu.edit_profiles":    "Edit profiles (notepad)",
		"menu.reload_profiles":  "Reload profiles",
		"menu.language":         "Language",
		"menu.quit":             "Quit",
		"edge.always_share":     "always share",
	},
	langPT: {
		"status.stopped":        "parado",
		"status.listening_as":   "ouvindo como %s",
		"status.controlling":    "controlando %s",
		"menu.listen_as_target": "Ouvir como target",
		"menu.connect_to":       "Conectar em",
		"menu.stop_target":      "Parar target",
		"menu.stop_controller":  "Parar controller",
		"menu.copy_token":       "Copiar token do target",
		"menu.edit_profiles":    "Editar perfis (notepad)",
		"menu.reload_profiles":  "Recarregar perfis",
		"menu.language":         "Idioma",
		"menu.quit":             "Sair",
		"edge.always_share":     "sempre compartilhar",
	},
}
