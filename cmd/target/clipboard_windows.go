//go:build windows

package main

import (
	"crypto/tls"
	"log"
	"net"
	"sync"
	"time"

	"kbs/internal/auth"
	"kbs/internal/clipboard"
	"kbs/internal/clipsync"
	"kbs/internal/inject"
	"kbs/internal/keys"
)

// startClipboardListener starts a second TLS listener, one port above
// listenAddr, dedicated to the clipboard-transfer hotkey (see
// internal/clipsync): a large clipboard payload never queues behind, or
// delays, the latency-sensitive mouse/keyboard connection because it
// never shares that connection in the first place. Reuses the same
// certificate as the main listener.
func startClipboardListener(cert tls.Certificate, listenAddr string, injector inject.Injector, token string) error {
	clipAddr, err := clipsync.ClipAddr(listenAddr)
	if err != nil {
		return err
	}
	ln, err := tls.Listen("tcp", clipAddr, &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	})
	if err != nil {
		return err
	}
	go serveClipboard(ln, injector, token)
	return nil
}

func serveClipboard(ln net.Listener, injector inject.Injector, token string) {
	defer ln.Close()
	var mu sync.Mutex
	var active net.Conn
	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Printf("clipboard: accept: %v", err)
			return
		}
		mu.Lock()
		if active != nil {
			mu.Unlock()
			conn.Close()
			continue
		}
		active = conn
		mu.Unlock()
		go func() {
			handleClipConn(conn, injector, token)
			mu.Lock()
			active = nil
			mu.Unlock()
		}()
	}
}

func handleClipConn(conn net.Conn, injector inject.Injector, token string) {
	defer conn.Close()

	conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	kind, payload, err := clipsync.ReadFrame(conn)
	if err != nil || kind != clipsync.KindAuth || !auth.TokensEqual(string(payload), token) {
		clipsync.WriteFrame(conn, clipsync.KindAuthFail, nil)
		return
	}
	conn.SetReadDeadline(time.Time{})
	if err := clipsync.WriteFrame(conn, clipsync.KindAuthOK, nil); err != nil {
		return
	}
	log.Print("clipboard: controller connected")

	for {
		kind, payload, err := clipsync.ReadFrame(conn)
		if err != nil {
			return
		}
		switch kind {
		case clipsync.KindRequest:
			if err := replyWithLocalClipboard(conn); err != nil {
				log.Printf("clipboard: reading local clipboard: %v", err)
			}
		case clipsync.KindText:
			if err := clipboard.WriteText(string(payload)); err != nil {
				log.Printf("clipboard: writing text: %v", err)
				continue
			}
			log.Print("clipboard: received text, pasting")
			injectPaste(injector)
		case clipsync.KindImage:
			if err := clipboard.WriteImagePNG(payload); err != nil {
				log.Printf("clipboard: writing image: %v", err)
				continue
			}
			log.Print("clipboard: received image, pasting")
			injectPaste(injector)
		}
	}
}

func replyWithLocalClipboard(conn net.Conn) error {
	if text, ok, err := clipboard.ReadText(); err != nil {
		return err
	} else if ok {
		return clipsync.WriteFrame(conn, clipsync.KindText, []byte(text))
	}
	if png, ok, err := clipboard.ReadImagePNG(); err != nil {
		return err
	} else if ok {
		return clipsync.WriteFrame(conn, clipsync.KindImage, png)
	}
	return clipsync.WriteFrame(conn, clipsync.KindText, nil)
}

// modifierKeys are released before injecting the paste chord: the
// Ctrl+Alt+V hotkey only ever suppresses V, so Ctrl and Alt's own
// down/up events are forwarded and injected like any other key - meaning
// they're still marked "held" in this machine's key state when the paste
// fires. Without this, the synthetic Ctrl+V lands on top of the still-held
// Alt (and Ctrl) and the target sees Ctrl+Alt+V, which pastes nothing.
var modifierKeys = []keys.Name{
	keys.ControlLeft, keys.ControlRight,
	keys.AltLeft, keys.AltRight,
	keys.ShiftLeft, keys.ShiftRight,
}

// injectPasteMu serializes injectPaste: two overlapping runs (e.g. from a
// mistimed extra hotkey press) would interleave their synthetic
// modifier up/down calls and could leave one stuck "held" afterwards.
var injectPasteMu sync.Mutex

// injectPaste synthesizes Ctrl+V so the freshly-written clipboard content
// actually lands in whatever's focused.
func injectPaste(injector inject.Injector) {
	injectPasteMu.Lock()
	defer injectPasteMu.Unlock()

	for _, k := range modifierKeys {
		injector.Key(k, false)
	}
	time.Sleep(15 * time.Millisecond)
	injector.Key(keys.ControlLeft, true)
	time.Sleep(15 * time.Millisecond)
	injector.Key(keys.V, true)
	time.Sleep(15 * time.Millisecond)
	injector.Key(keys.V, false)
	time.Sleep(15 * time.Millisecond)
	injector.Key(keys.ControlLeft, false)
}
