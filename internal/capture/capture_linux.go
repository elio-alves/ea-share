//go:build linux

package capture

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"kbs/internal/keys"
)

// Linux input-event-codes.h constants relevant to capture. Kept local
// (rather than depending on a cgo-based evdev library, which would defeat
// the point of a CGO-free Linux build).
const (
	evSyn = 0x00
	evKey = 0x01
	evRel = 0x02

	relX     = 0x00
	relY     = 0x01
	relWheel = 0x08

	btnLeft   = 0x110
	btnRight  = 0x111
	btnMiddle = 0x112

	// size of struct input_event on 64-bit Linux: two 8-byte timeval
	// fields, then uint16 type, uint16 code, int32 value.
	inputEventSize = 24
)

// virtualDeviceMarker must match the name substring used by the Linux
// inject backend (inject_linux.go) for the uinput devices it creates, so a
// controller and target running on the same machine (e.g. for testing)
// don't capture and re-forward their own injected events in a loop.
const virtualDeviceMarker = "kbs-virtual-input"

var handlersLineRE = regexp.MustCompile(`(?m)^H:\s*Handlers=(.*)$`)
var eventNodeRE = regexp.MustCompile(`event(\d+)`)

type linuxSource struct {
	events  chan Event
	stopCh  chan struct{}
	devices []*os.File
}

// New returns a capture.Source backed by raw reads from /dev/input/eventN
// devices, identified via /proc/bus/input/devices (no ioctl/cgo required).
func New() Source {
	return &linuxSource{}
}

func (s *linuxSource) Start() (<-chan Event, error) {
	paths, err := discoverInputDevices()
	if err != nil {
		return nil, fmt.Errorf("capture: discovering input devices: %w", err)
	}

	var opened []*os.File
	for _, p := range paths {
		f, err := os.Open(p)
		if err != nil {
			continue // typically permission denied on a node we don't care about
		}
		opened = append(opened, f)
	}
	if len(opened) == 0 {
		return nil, errors.New("capture: no readable keyboard/mouse devices found " +
			"(run as root or add your user to the 'input' group)")
	}

	s.devices = opened
	s.events = make(chan Event, 2048)
	s.stopCh = make(chan struct{})

	var wg sync.WaitGroup
	for _, f := range opened {
		wg.Add(1)
		go func(file *os.File) {
			defer wg.Done()
			s.readLoop(file)
		}(f)
	}
	go func() {
		wg.Wait()
		close(s.events)
	}()

	return s.events, nil
}

func (s *linuxSource) Stop() error {
	if s.stopCh != nil {
		close(s.stopCh)
	}
	for _, f := range s.devices {
		f.Close()
	}
	return nil
}

func (s *linuxSource) readLoop(f *os.File) {
	buf := make([]byte, inputEventSize)
	var dx, dy int32
	for {
		select {
		case <-s.stopCh:
			return
		default:
		}

		if _, err := io.ReadFull(f, buf); err != nil {
			return
		}
		typ := binary.LittleEndian.Uint16(buf[16:18])
		code := binary.LittleEndian.Uint16(buf[18:20])
		value := int32(binary.LittleEndian.Uint32(buf[20:24]))

		switch typ {
		case evKey:
			if value != 0 && value != 1 {
				continue // ignore autorepeat
			}
			down := value == 1
			switch int(code) {
			case btnLeft:
				s.sendEvent(Event{Kind: MouseButtonEvent, Button: ButtonLeft, Down: down})
			case btnRight:
				s.sendEvent(Event{Kind: MouseButtonEvent, Button: ButtonRight, Down: down})
			case btnMiddle:
				s.sendEvent(Event{Kind: MouseButtonEvent, Button: ButtonMiddle, Down: down})
			default:
				if name, ok := keys.KeycodeToName[int(code)]; ok {
					s.sendEvent(Event{Kind: KeyEvent, Key: name, Down: down})
				}
			}
		case evRel:
			switch int(code) {
			case relX:
				dx += value
			case relY:
				dy += value
			case relWheel:
				s.sendEvent(Event{Kind: MouseWheelEvent, Amount: value})
			}
		case evSyn:
			if dx != 0 || dy != 0 {
				s.sendEvent(Event{Kind: MouseMoveEvent, DX: dx, DY: dy})
				dx, dy = 0, 0
			}
		}
	}
}

func (s *linuxSource) sendEvent(e Event) {
	select {
	case s.events <- e:
	default:
		// Drop under backpressure rather than block a device read loop.
	}
}

// discoverInputDevices parses /proc/bus/input/devices to find event nodes
// whose Handlers line advertises a keyboard ("kbd") or mouse ("mouse")
// handler, skipping our own virtual devices to avoid feedback loops.
func discoverInputDevices() ([]string, error) {
	f, err := os.Open("/proc/bus/input/devices")
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var blocks []string
	var cur strings.Builder
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if cur.Len() > 0 {
				blocks = append(blocks, cur.String())
				cur.Reset()
			}
			continue
		}
		cur.WriteString(line)
		cur.WriteByte('\n')
	}
	if cur.Len() > 0 {
		blocks = append(blocks, cur.String())
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	var paths []string
	for _, block := range blocks {
		if strings.Contains(block, virtualDeviceMarker) {
			continue
		}
		m := handlersLineRE.FindStringSubmatch(block)
		if m == nil {
			continue
		}
		handlers := m[1]
		if !strings.Contains(handlers, "kbd") && !strings.Contains(handlers, "mouse") {
			continue
		}
		evm := eventNodeRE.FindString(handlers)
		if evm == "" {
			continue
		}
		paths = append(paths, filepath.Join("/dev/input", evm))
	}
	return paths, nil
}
