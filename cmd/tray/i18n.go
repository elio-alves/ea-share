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
		"status.stopped":          "stopped",
		"status.listening_as":     "listening as %s",
		"status.controlling":      "controlling %s",
		"status.target_listening": "listening",
		"status.ctrl_connecting":  "connecting…",
		"status.connected_tcp":    "connected (TCP)",
		"status.connected_udp":    "connected (TCP+UDP)",
		"menu.listen_as_target":   "Listen as target",
		"menu.connect_to":         "Connect to",
		"menu.stop_target":        "Stop target",
		"menu.stop_controller":    "Stop controller",
		"menu.copy_token":         "Copy target token",
		"menu.edit_profiles":      "Edit profiles",
		"menu.reload_profiles":    "Reload profiles",
		"menu.language":           "Language",
		"menu.quit":               "Quit",
		"edge.always_share":       "always share",

		"editor.title":                   "kbs — Profiles",
		"editor.tab_targets":             "Targets",
		"editor.tab_controllers":         "Controllers",
		"editor.field_name":              "Name",
		"editor.field_listen":            "Listen",
		"editor.field_token":             "Token",
		"editor.field_connect":           "Connect",
		"editor.field_edge":              "Edge",
		"editor.field_udp_mouse":         "Performance mode (UDP mouse, requires Edge)",
		"editor.add":                     "+ Add",
		"editor.remove":                  "- Remove",
		"editor.close":                   "Close",
		"editor.save":                    "Save",
		"editor.save_restart":            "Save && restart running",
		"editor.new_target":              "New target",
		"editor.new_controller":          "New controller",
		"editor.confirm_remove_title":    "Remove profile?",
		"editor.confirm_remove_body":     "Remove %q? This can't be undone.",
		"editor.error_title":             "Can't save",
		"editor.error_name_required":     "Every profile needs a name.",
		"editor.error_name_duplicate":    "Two profiles are named %q — names must be unique.",
		"editor.error_listen_required":   "%q: Listen address is required.",
		"editor.error_connect_required":  "%q: Connect address is required.",
		"editor.error_token_required":    "%q: Token is required.",
		"editor.error_udp_requires_edge": "%q: performance mode (UDP) requires an Edge side to be set.",
	},
	langPT: {
		"status.stopped":          "parado",
		"status.listening_as":     "ouvindo como %s",
		"status.controlling":      "controlando %s",
		"status.target_listening": "escutando",
		"status.ctrl_connecting":  "conectando…",
		"status.connected_tcp":    "conectado (TCP)",
		"status.connected_udp":    "conectado (TCP+UDP)",
		"menu.listen_as_target":   "Ouvir como target",
		"menu.connect_to":         "Conectar em",
		"menu.stop_target":        "Parar target",
		"menu.stop_controller":    "Parar controller",
		"menu.copy_token":         "Copiar token do target",
		"menu.edit_profiles":      "Editar perfis",
		"menu.reload_profiles":    "Recarregar perfis",
		"menu.language":           "Idioma",
		"menu.quit":               "Sair",
		"edge.always_share":       "sempre compartilhar",

		"editor.title":                   "kbs — Perfis",
		"editor.tab_targets":             "Targets",
		"editor.tab_controllers":         "Controllers",
		"editor.field_name":              "Nome",
		"editor.field_listen":            "Listen",
		"editor.field_token":             "Token",
		"editor.field_connect":           "Connect",
		"editor.field_edge":              "Edge",
		"editor.field_udp_mouse":         "Modo performance (mouse via UDP, exige Edge)",
		"editor.add":                     "+ Adicionar",
		"editor.remove":                  "- Remover",
		"editor.close":                   "Fechar",
		"editor.save":                    "Salvar",
		"editor.save_restart":            "Salvar e reiniciar em execução",
		"editor.new_target":              "Novo target",
		"editor.new_controller":          "Novo controller",
		"editor.confirm_remove_title":    "Remover perfil?",
		"editor.confirm_remove_body":     "Remover %q? Não tem como desfazer.",
		"editor.error_title":             "Não deu pra salvar",
		"editor.error_name_required":     "Todo perfil precisa de um nome.",
		"editor.error_name_duplicate":    "Dois perfis estão com o nome %q — os nomes precisam ser únicos.",
		"editor.error_listen_required":   "%q: o endereço Listen é obrigatório.",
		"editor.error_connect_required":  "%q: o endereço Connect é obrigatório.",
		"editor.error_token_required":    "%q: o Token é obrigatório.",
		"editor.error_udp_requires_edge": "%q: o modo performance (UDP) exige um lado de Edge configurado.",
	},
}
