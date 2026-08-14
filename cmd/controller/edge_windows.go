//go:build windows

package main

import (
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	"kbs/internal/capture"
	"kbs/internal/protocol"
	"kbs/internal/screen"
)

// runEdgeAware implements Synergy-style edge switching: local input passes
// through untouched until the cursor reaches the configured edge, at which
// point local input is suppressed and forwarded to the target. The
// target's cursor position is simulated locally (from target's reported
// screen size and the deltas we've sent) so that pushing back past the
// same entry edge hands control back, with no extra network round trip
// needed to detect the release.
func runEdgeAware(conn net.Conn, edge protocol.Edge, clip *clipClient) error {
	m, err := protocol.ReadMessage(conn)
	if err != nil {
		return fmt.Errorf("reading target screen info: %w", err)
	}
	if m.Type != protocol.MsgScreenInfo {
		return fmt.Errorf("expected screen_info from target, got %q", m.Type)
	}
	targetW, targetH := m.Width, m.Height
	if targetW <= 0 || targetH <= 0 {
		return fmt.Errorf("target reported invalid screen size %dx%d", targetW, targetH)
	}

	controllerBounds := screen.GetBounds()

	src, err := capture.NewEdgeAware(capture.Edge(edge))
	if err != nil {
		return fmt.Errorf("initializing edge-aware capture: %w", err)
	}
	events, err := src.Start()
	if err != nil {
		return fmt.Errorf("starting input capture: %w", err)
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

	fmt.Printf("Edge mode active: target is to the %s. Move the cursor off that edge to hand off control; push back the other way to return.\nPress Ctrl+C to stop.\n", edge)

	entryEdge := edge.Opposite()

	var engaged bool
	var movedAway bool
	var vx, vy int32 // simulated target cursor position, valid while engaged

	for e := range events {
		switch e.Kind {
		case capture.EdgeCrossedEvent:
			engaged = true
			movedAway = false
			vx, vy = entryPosition(entryEdge, e.RelPos, targetW, targetH)
			if err := protocol.WriteMessage(conn, protocol.Message{
				Type: protocol.MsgEngage, Edge: edge, RelPos: e.RelPos,
			}); err != nil {
				return fmt.Errorf("connection lost: %w", err)
			}
			log.Print("engaged: now controlling the target")

		case capture.MouseMoveEvent:
			if !engaged {
				continue
			}
			nx, ny := vx+e.DX, vy+e.DY
			pushingOutEntry := pushesPast(entryEdge, nx, ny, targetW, targetH)

			if movedAway && pushingOutEntry {
				relPos := releaseRelPos(entryEdge, clampI(nx, 0, targetW-1), clampI(ny, 0, targetH-1), targetW, targetH)
				warpX, warpY := controllerWarpPosition(edge, relPos, controllerBounds.X, controllerBounds.Y, controllerBounds.W, controllerBounds.H)
				if err := src.Disengage(warpX, warpY); err != nil {
					log.Printf("disengage: %v", err)
				}
				engaged = false
				log.Print("disengaged: local input restored")
				continue // this move belongs to the controller's own screen now
			}

			vx, vy = clampI(nx, 0, targetW-1), clampI(ny, 0, targetH-1)
			if !pushingOutEntry && !movedAway {
				movedAway = hasMovedAway(entryEdge, vx, vy, targetW, targetH)
			}
			if err := protocol.WriteMessage(conn, protocol.Message{Type: protocol.MsgMouseMove, DX: e.DX, DY: e.DY}); err != nil {
				return fmt.Errorf("connection lost: %w", err)
			}

		case capture.KeyEvent:
			if !engaged {
				continue
			}
			if err := protocol.WriteMessage(conn, protocol.Message{Type: protocol.MsgKey, Key: string(e.Key), Down: e.Down}); err != nil {
				return fmt.Errorf("connection lost: %w", err)
			}

		case capture.MouseButtonEvent:
			if !engaged {
				continue
			}
			if err := protocol.WriteMessage(conn, protocol.Message{Type: protocol.MsgMouseButton, Button: string(e.Button), Down: e.Down}); err != nil {
				return fmt.Errorf("connection lost: %w", err)
			}

		case capture.MouseWheelEvent:
			if !engaged {
				continue
			}
			if err := protocol.WriteMessage(conn, protocol.Message{Type: protocol.MsgMouseWheel, Amount: e.Amount}); err != nil {
				return fmt.Errorf("connection lost: %w", err)
			}

		case capture.HotkeyPasteEvent:
			if clip == nil {
				log.Print("clipboard: Ctrl+Alt+V pressed, but clipboard sync isn't connected")
				continue
			}
			engagedNow := engaged
			go handlePasteHotkey(clip, engagedNow)
		}
	}
	return errors.New("input capture stopped unexpectedly")
}
