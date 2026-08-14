//go:build windows

package main

import (
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"kbs/internal/clipboard"
	"kbs/internal/clipsync"
	"kbs/internal/inject"
	"kbs/internal/keys"
)

// clipClient is the controller's side of the dedicated clipboard
// connection (see internal/clipsync); one is dialed per session, kept
// open, and used for every Ctrl+Alt+V press.
type clipClient struct {
	mu   sync.Mutex
	conn net.Conn
}

// dialClipboard connects to the target's clipboard port (see
// clipsync.ClipAddr) and authenticates with token. tlsCfg should pin the
// exact certificate fingerprint already trusted for the main connection -
// the clipboard channel is a second connection to the same target, not a
// separate trust decision.
func dialClipboard(mainAddr, token string, tlsCfg *tls.Config) (*clipClient, error) {
	clipAddr, err := clipsync.ClipAddr(mainAddr)
	if err != nil {
		return nil, err
	}
	conn, err := tls.Dial("tcp", clipAddr, tlsCfg)
	if err != nil {
		return nil, err
	}
	conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	if err := clipsync.WriteFrame(conn, clipsync.KindAuth, []byte(token)); err != nil {
		conn.Close()
		return nil, err
	}
	kind, _, err := clipsync.ReadFrame(conn)
	if err != nil {
		conn.Close()
		return nil, err
	}
	if kind != clipsync.KindAuthOK {
		conn.Close()
		return nil, fmt.Errorf("clipboard: auth rejected by target")
	}
	conn.SetReadDeadline(time.Time{})
	return &clipClient{conn: conn}, nil
}

func (c *clipClient) Close() error {
	return c.conn.Close()
}

// pushLocalClipboard reads this machine's own clipboard and sends it to
// the target (used while engaged: the controller already has the
// content, no round trip needed).
func (c *clipClient) pushLocalClipboard() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if text, ok, err := clipboard.ReadText(); err != nil {
		return err
	} else if ok {
		return clipsync.WriteFrame(c.conn, clipsync.KindText, []byte(text))
	}
	if png, ok, err := clipboard.ReadImagePNG(); err != nil {
		return err
	} else if ok {
		return clipsync.WriteFrame(c.conn, clipsync.KindImage, png)
	}
	return nil // nothing on the clipboard worth sending
}

// pullRemoteClipboard asks the target for its clipboard, writes it into
// this machine's own clipboard, and pastes it into whatever's focused
// locally (used while disengaged).
func (c *clipClient) pullRemoteClipboard(injector inject.Injector) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := clipsync.WriteFrame(c.conn, clipsync.KindRequest, nil); err != nil {
		return err
	}
	c.conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	kind, payload, err := clipsync.ReadFrame(c.conn)
	c.conn.SetReadDeadline(time.Time{})
	if err != nil {
		return err
	}
	switch kind {
	case clipsync.KindText:
		if len(payload) == 0 {
			return nil
		}
		if err := clipboard.WriteText(string(payload)); err != nil {
			return err
		}
	case clipsync.KindImage:
		if err := clipboard.WriteImagePNG(payload); err != nil {
			return err
		}
	default:
		return fmt.Errorf("clipboard: unexpected reply kind %d", kind)
	}
	injectPaste(injector)
	return nil
}

// handlePasteHotkey runs the Ctrl+Alt+V transfer for one press, in its own
// goroutine so a network round trip never stalls the input-forwarding
// loop. engaged reflects whether the controller was driving the target at
// the moment the hotkey fired: if so, this machine's own clipboard is
// already the relevant one and gets pushed; otherwise the target's
// clipboard is pulled and pasted here.
func handlePasteHotkey(clip *clipClient, engaged bool) {
	if engaged {
		if err := clip.pushLocalClipboard(); err != nil {
			log.Printf("clipboard: sending to target: %v", err)
			return
		}
		log.Print("clipboard: sent to target")
		return
	}

	localInjector, err := inject.New()
	if err != nil {
		log.Printf("clipboard: local injector: %v", err)
		return
	}
	defer localInjector.Close()
	if err := clip.pullRemoteClipboard(localInjector); err != nil {
		log.Printf("clipboard: pulling from target: %v", err)
		return
	}
	log.Print("clipboard: pasted from target")
}

// modifierKeys are released before injecting the paste chord: the
// Ctrl+Alt+V hotkey only ever suppresses V, so Ctrl and Alt's own
// down/up events pass through normally and are still "held" in this
// machine's real key state when the paste fires (they were genuinely
// pressed, just not released yet). Without this, the synthetic Ctrl+V
// lands on top of the still-held Alt (and Ctrl) and whatever's focused
// sees Ctrl+Alt+V, which pastes nothing.
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
