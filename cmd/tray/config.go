//go:build windows

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// TargetProfile is a saved "listen as target" preset.
type TargetProfile struct {
	Name   string `json:"name"`
	Listen string `json:"listen"`
	Token  string `json:"token"`
}

// ControllerProfile is a saved "connect as controller" preset.
type ControllerProfile struct {
	Name    string `json:"name"`
	Connect string `json:"connect"`
	Token   string `json:"token"`
	// Edge is left|right|top|bottom for edge-triggered switching, or ""
	// for legacy always-share mode.
	Edge string `json:"edge"`
}

type Config struct {
	// Lang is the tray UI language: "en" or "pt". Defaults to "en" when
	// empty (e.g. a profile file saved before this field existed).
	Lang        string              `json:"lang"`
	Targets     []TargetProfile     `json:"targets"`
	Controllers []ControllerProfile `json:"controllers"`
}

func configPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "kbs", "tray_profiles.json"), nil
}

// defaultConfig seeds the profile file with placeholder examples so the
// menu isn't empty on first run. Edit the generated file (or use "Edit
// profiles") to add your own machines and a real token.
func defaultConfig() Config {
	return Config{
		Lang: string(langEN),
		Targets: []TargetProfile{
			{Name: "This PC", Listen: ":7777", Token: ""},
		},
		Controllers: []ControllerProfile{
			{Name: "Other PC", Connect: "192.168.1.50:7777", Token: "change-me", Edge: "right"},
		},
	}
}

func loadConfig() (Config, string, error) {
	path, err := configPath()
	if err != nil {
		return Config{}, "", err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		cfg := defaultConfig()
		if err := saveConfig(path, cfg); err != nil {
			return cfg, path, err
		}
		return cfg, path, nil
	}
	if err != nil {
		return Config{}, path, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, path, err
	}
	if cfg.Lang == "" {
		cfg.Lang = string(langEN)
	}
	return cfg, path, nil
}

func saveConfig(path string, cfg Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}
