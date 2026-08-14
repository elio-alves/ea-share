// Command controller connects to a target and forwards this machine's
// local keyboard/mouse input to it. This is the machine doing the
// controlling.
package main

import (
	"bufio"
	"crypto/tls"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"kbs/internal/capture"
	"kbs/internal/protocol"
	"kbs/internal/tlsutil"
)

func main() {
	addr := flag.String("connect", "", "target address, host:port (required)")
	token := flag.String("token", os.Getenv("KBS_TOKEN"), "shared secret to present to the target (env KBS_TOKEN)")
	fingerprint := flag.String("fingerprint", "", "pin the target's expected certificate fingerprint (optional; otherwise trust-on-first-use)")
	yes := flag.Bool("yes", false, "automatically trust an unpinned target's certificate on first connect, without prompting")
	knownHostsPath := flag.String("known-hosts", defaultKnownHostsPath(), "path to the trust-on-first-use store")
	edgeFlag := flag.String("edge", "", "enable edge-triggered switching: left|right|top|bottom is the side the target's screen is on (Windows only; default: always share, no switching)")
	flag.Parse()

	if *addr == "" {
		fmt.Fprintln(os.Stderr, "error: -connect host:port is required")
		flag.Usage()
		os.Exit(2)
	}
	if *token == "" {
		fmt.Fprintln(os.Stderr, "error: -token (or KBS_TOKEN) is required")
		os.Exit(2)
	}
	var edge protocol.Edge
	switch *edgeFlag {
	case "":
	case "left":
		edge = protocol.EdgeLeft
	case "right":
		edge = protocol.EdgeRight
	case "top":
		edge = protocol.EdgeTop
	case "bottom":
		edge = protocol.EdgeBottom
	default:
		fmt.Fprintf(os.Stderr, "error: -edge must be one of left|right|top|bottom, got %q\n", *edgeFlag)
		os.Exit(2)
	}

	knownHosts, err := tlsutil.OpenKnownHosts(*knownHostsPath)
	if err != nil {
		log.Fatalf("opening known-hosts store %s: %v", *knownHostsPath, err)
	}

	conn, err := dialAndVerify(*addr, *fingerprint, *yes, knownHosts)
	if err != nil {
		log.Fatalf("connecting to %s: %v", *addr, err)
	}
	defer conn.Close()

	if err := protocol.WriteMessage(conn, protocol.Message{Type: protocol.MsgAuth, Token: *token}); err != nil {
		log.Fatalf("sending auth: %v", err)
	}
	conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	resp, err := protocol.ReadMessage(conn)
	if err != nil {
		log.Fatalf("reading auth response: %v", err)
	}
	if resp.Type != protocol.MsgAuthOK {
		log.Fatalf("authentication rejected by target (wrong token?)")
	}
	conn.SetReadDeadline(time.Time{})

	fmt.Printf("Connected and authenticated to %s.\n", *addr)

	if edge != "" {
		var clip *clipClient
		if fp, ok := knownHosts.Lookup(*addr); ok {
			c, err := dialClipboard(*addr, *token, pinnedTLSConfig(fp))
			if err != nil {
				log.Printf("clipboard sync unavailable: %v", err)
			} else {
				clip = c
				defer clip.Close()
			}
		}
		if err := runEdgeAware(conn, edge, clip); err != nil {
			log.Fatalf("edge mode: %v", err)
		}
		return
	}
	runLegacy(conn)
}

func runLegacy(conn net.Conn) {
	fmt.Println("Sharing this machine's keyboard/mouse. Press Ctrl+C to stop.")

	src := capture.New()
	events, err := src.Start()
	if err != nil {
		log.Fatalf("starting input capture: %v", err)
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Println("\nStopping...")
		src.Stop()
		conn.Close()
		os.Exit(0)
	}()

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case e, ok := <-events:
			if !ok {
				log.Fatal("input capture stopped unexpectedly")
			}
			msg, ok := toMessage(e)
			if !ok {
				continue
			}
			if err := protocol.WriteMessage(conn, msg); err != nil {
				log.Fatalf("connection lost: %v", err)
			}
		case <-ticker.C:
			if err := protocol.WriteMessage(conn, protocol.Message{Type: protocol.MsgPing}); err != nil {
				log.Fatalf("connection lost: %v", err)
			}
		}
	}
}

func toMessage(e capture.Event) (protocol.Message, bool) {
	switch e.Kind {
	case capture.KeyEvent:
		return protocol.Message{Type: protocol.MsgKey, Key: string(e.Key), Down: e.Down}, true
	case capture.MouseMoveEvent:
		return protocol.Message{Type: protocol.MsgMouseMove, DX: e.DX, DY: e.DY}, true
	case capture.MouseButtonEvent:
		return protocol.Message{Type: protocol.MsgMouseButton, Button: string(e.Button), Down: e.Down}, true
	case capture.MouseWheelEvent:
		return protocol.Message{Type: protocol.MsgMouseWheel, Amount: e.Amount}, true
	default:
		return protocol.Message{}, false
	}
}

// dialAndVerify establishes a TLS connection to addr, pinning/checking the
// target's certificate fingerprint the same way SSH pins host keys:
//   - if a fingerprint was already trusted for addr, it must match exactly
//   - otherwise, the user is prompted to confirm (or -yes/-fingerprint
//     short-circuits the prompt) and the fingerprint is then saved
func dialAndVerify(addr, pinnedFlag string, autoYes bool, knownHosts *tlsutil.KnownHosts) (net.Conn, error) {
	pinnedFlag = normalizeFingerprint(pinnedFlag)
	var verifyErr error

	cfg := &tls.Config{
		InsecureSkipVerify: true, // we do our own verification below (TLS cert pinning, not CA trust)
		VerifyConnection: func(cs tls.ConnectionState) error {
			if len(cs.PeerCertificates) == 0 {
				return errors.New("no certificate presented")
			}
			got := tlsutil.Fingerprint(cs.PeerCertificates[0].Raw)

			if stored, ok := knownHosts.Lookup(addr); ok {
				if got != stored {
					verifyErr = fmt.Errorf(
						"certificate fingerprint for %s changed!\n  expected: %s\n  got:      %s\n"+
							"this could mean the target was reinstalled, or someone is intercepting the connection.\n"+
							"if this is expected, remove the old entry from %s and reconnect",
						addr, stored, got, knownHosts.Path())
					return verifyErr
				}
				return nil
			}

			if pinnedFlag != "" {
				if got != pinnedFlag {
					verifyErr = fmt.Errorf("certificate fingerprint does not match -fingerprint\n  expected: %s\n  got:      %s", pinnedFlag, got)
					return verifyErr
				}
				return knownHosts.Trust(addr, got)
			}

			if !autoYes {
				fmt.Printf("Unknown target %s.\nCertificate fingerprint: %s\nTrust and remember this target? [y/N]: ", addr, got)
				reader := bufio.NewReader(os.Stdin)
				line, _ := reader.ReadString('\n')
				if strings.TrimSpace(strings.ToLower(line)) != "y" && strings.TrimSpace(strings.ToLower(line)) != "yes" {
					verifyErr = errors.New("connection not trusted by user")
					return verifyErr
				}
			}
			return knownHosts.Trust(addr, got)
		},
	}

	conn, err := tls.Dial("tcp", addr, cfg)
	if err != nil {
		if verifyErr != nil {
			return nil, verifyErr
		}
		return nil, err
	}
	return conn, nil
}

// pinnedTLSConfig builds a TLS config that accepts only a connection
// presenting exactly fingerprint fp - no prompting, no known-hosts
// lookup. Used for the clipboard connection, which is a second connection
// to a target whose identity was already confirmed (and pinned) moments
// earlier by dialAndVerify on the main connection.
func pinnedTLSConfig(fp string) *tls.Config {
	return &tls.Config{
		InsecureSkipVerify: true,
		VerifyConnection: func(cs tls.ConnectionState) error {
			if len(cs.PeerCertificates) == 0 {
				return errors.New("no certificate presented")
			}
			if got := tlsutil.Fingerprint(cs.PeerCertificates[0].Raw); got != fp {
				return fmt.Errorf("clipboard connection certificate fingerprint changed: expected %s, got %s", fp, got)
			}
			return nil
		},
	}
}

func normalizeFingerprint(fp string) string {
	return strings.ToUpper(strings.TrimSpace(fp))
}

func defaultKnownHostsPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ".kbs/known_hosts.json"
	}
	return filepath.Join(dir, "kbs", "known_hosts.json")
}
