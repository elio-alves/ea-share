// Command target listens for a controller connection and injects the
// keyboard/mouse events it receives into the local machine. This is the
// machine being remotely driven.
package main

import (
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"kbs/internal/auth"
	"kbs/internal/inject"
	"kbs/internal/keys"
	"kbs/internal/protocol"
	"kbs/internal/tlsutil"
)

func main() {
	listenAddr := flag.String("listen", ":7777", "address to listen on")
	token := flag.String("token", os.Getenv("KBS_TOKEN"), "shared secret the controller must present (env KBS_TOKEN); random if empty")
	dataDir := flag.String("data-dir", defaultDataDir(), "directory for the TLS certificate")
	verbose := flag.Bool("verbose", false, "periodically log how many events of each kind have been received (diagnostic)")
	flag.Parse()

	if *token == "" {
		generated, err := randomToken()
		if err != nil {
			log.Fatalf("generating token: %v", err)
		}
		*token = generated
		fmt.Printf("No -token given; generated one for this session:\n\n  %s\n\n", *token)
	}

	cert, err := tlsutil.LoadOrGenerateCert(*dataDir)
	if err != nil {
		log.Fatalf("loading/generating TLS certificate: %v", err)
	}
	fmt.Printf("Certificate fingerprint (verify this matches on the controller):\n\n  %s\n\n", tlsutil.Fingerprint(cert.Certificate[0]))

	injector, err := inject.New()
	if err != nil {
		log.Fatalf("initializing input injector: %v", err)
	}
	defer injector.Close()

	if err := startClipboardListener(cert, *listenAddr, injector, *token); err != nil {
		log.Printf("clipboard sync unavailable: %v", err)
	}

	ln, err := tls.Listen("tcp", *listenAddr, &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	})
	if err != nil {
		log.Fatalf("listening on %s: %v", *listenAddr, err)
	}
	defer ln.Close()

	fmt.Printf("Listening on %s. Waiting for a controller to connect...\n", *listenAddr)

	if *verbose {
		go func() {
			ticker := time.NewTicker(2 * time.Second)
			defer ticker.Stop()
			var lastKey, lastMove, lastBtn, lastWheel int64
			for range ticker.C {
				key, move, btn, wheel := keyCount.Load(), moveCount.Load(), buttonCount.Load(), wheelCount.Load()
				if key != lastKey || move != lastMove || btn != lastBtn || wheel != lastWheel {
					log.Printf("received so far: key=%d mouse_move=%d mouse_button=%d mouse_wheel=%d", key, move, btn, wheel)
					lastKey, lastMove, lastBtn, lastWheel = key, move, btn, wheel
				}
			}
		}()
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Println("\nShutting down...")
		ln.Close()
		injector.Close()
		os.Exit(0)
	}()

	var mu sync.Mutex
	var active net.Conn

	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Printf("accept: %v", err)
			return
		}

		mu.Lock()
		if active != nil {
			mu.Unlock()
			log.Printf("rejecting %s: a controller is already connected", conn.RemoteAddr())
			conn.Close()
			continue
		}
		active = conn
		mu.Unlock()

		go func() {
			handleConn(conn, injector, *token)
			mu.Lock()
			active = nil
			mu.Unlock()
		}()
	}
}

func handleConn(conn net.Conn, injector inject.Injector, token string) {
	defer conn.Close()
	remote := conn.RemoteAddr()
	log.Printf("connection from %s: awaiting auth", remote)

	conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	m, err := protocol.ReadMessage(conn)
	if err != nil || m.Type != protocol.MsgAuth || !auth.TokensEqual(m.Token, token) {
		log.Printf("connection from %s: auth failed", remote)
		protocol.WriteMessage(conn, protocol.Message{Type: protocol.MsgAuthFail})
		return
	}
	conn.SetReadDeadline(time.Time{})
	if err := protocol.WriteMessage(conn, protocol.Message{Type: protocol.MsgAuthOK}); err != nil {
		return
	}
	w, h := ownScreenBounds()
	if err := protocol.WriteMessage(conn, protocol.Message{Type: protocol.MsgScreenInfo, Width: w, Height: h}); err != nil {
		return
	}
	log.Printf("connection from %s: authenticated, now controlling this machine", remote)
	defer log.Printf("connection from %s: disconnected", remote)

	for {
		m, err := protocol.ReadMessage(conn)
		if err != nil {
			return
		}
		if err := dispatch(injector, m); err != nil {
			log.Printf("injecting event from %s: %v", remote, err)
		}
	}
}

var (
	keyCount    atomic.Int64
	moveCount   atomic.Int64
	buttonCount atomic.Int64
	wheelCount  atomic.Int64
)

func dispatch(injector inject.Injector, m protocol.Message) error {
	switch m.Type {
	case protocol.MsgKey:
		keyCount.Add(1)
		return injector.Key(keys.Name(m.Key), m.Down)
	case protocol.MsgMouseMove:
		moveCount.Add(1)
		return injector.MouseMove(m.DX, m.DY)
	case protocol.MsgMouseButton:
		buttonCount.Add(1)
		var btn inject.MouseButton
		switch m.Button {
		case "left":
			btn = inject.ButtonLeft
		case "right":
			btn = inject.ButtonRight
		case "middle":
			btn = inject.ButtonMiddle
		default:
			return fmt.Errorf("unknown button %q", m.Button)
		}
		return injector.MouseButton(btn, m.Down)
	case protocol.MsgMouseWheel:
		wheelCount.Add(1)
		return injector.MouseWheel(m.Amount)
	case protocol.MsgEngage:
		w, h := ownScreenBounds()
		entryEdge := m.Edge.Opposite()
		x, y := entryPosition(entryEdge, m.RelPos, w, h)
		return warpCursor(x, y)
	case protocol.MsgPing:
		return nil
	default:
		return fmt.Errorf("unknown message type %q", m.Type)
	}
}

// entryPosition is where the cursor should land when entering entryEdge,
// relPos fraction of the way along that edge.
func entryPosition(entryEdge protocol.Edge, relPos float64, w, h int32) (x, y int32) {
	switch entryEdge {
	case protocol.EdgeLeft:
		return 0, int32(relPos * float64(h))
	case protocol.EdgeRight:
		return w - 1, int32(relPos * float64(h))
	case protocol.EdgeTop:
		return int32(relPos * float64(w)), 0
	case protocol.EdgeBottom:
		return int32(relPos * float64(w)), h - 1
	default:
		return 0, 0
	}
}

func randomToken() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func defaultDataDir() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ".kbs"
	}
	return filepath.Join(dir, "kbs")
}
