//go:build windows

// Command tray puts a kbs icon in the Windows notification area to start
// listening as target or connect as controller from saved profiles,
// without needing a terminal. Build with -ldflags "-H=windowsgui" so no
// console window flashes on launch; see README.
package main

import (
	"log"
	"os"
	"path/filepath"

	"fyne.io/systray"
)

func main() {
	if dir, err := os.UserConfigDir(); err == nil {
		logDir := filepath.Join(dir, "kbs")
		if err := os.MkdirAll(logDir, 0700); err == nil {
			if f, err := os.OpenFile(filepath.Join(logDir, "tray.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600); err == nil {
				log.SetOutput(f)
			}
		}
	}

	cfg, path, err := loadConfig()
	if err != nil {
		log.Fatalf("loading config: %v", err)
	}
	setLang(lang(cfg.Lang))

	a := &app{cfg: cfg, cfgPath: path}
	systray.Run(a.onReady, a.onExit)
}
