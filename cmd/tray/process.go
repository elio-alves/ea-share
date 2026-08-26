//go:build windows

package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
)

// createNoWindow (CREATE_NO_WINDOW) stops Windows from popping up a
// console for the spawned target.exe/controller.exe, which are ordinary
// console-subsystem binaries.
const createNoWindow = 0x08000000

// runningProc is a spawned target.exe or controller.exe under tray control.
type runningProc struct {
	cmd *exec.Cmd
}

// Stop forcibly ends the process. target/controller don't have a reliable
// way to receive Ctrl+C from a parent that gave them no console (see
// createNoWindow), so this kills rather than signals; both processes only
// hold OS resources (hooks, sockets) that Windows reclaims on exit.
func (p *runningProc) Stop() {
	if p == nil || p.cmd == nil || p.cmd.Process == nil {
		return
	}
	p.cmd.Process.Kill()
}

// binPath resolves name (e.g. "target.exe") relative to the tray's own
// executable directory, where the sibling CLI binaries are expected to live.
func binPath(name string) (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(exe), name), nil
}

// siblingBinaryName derives the sibling CLI binary's name from the tray's
// own executable name, carrying over whatever suffix it was built with
// (see scripts/build.sh --suffix): running as tray2.exe calls
// target2.exe/controller2.exe, tray.exe calls target.exe/controller.exe.
// Without this, a tray built under a suffix (the project's convention for
// testing new work without disturbing an already-deployed session) would
// silently keep spawning the unrelated, unsuffixed binaries sitting next
// to it.
func siblingBinaryName(base string) string {
	exe, err := os.Executable()
	if err != nil {
		return base + ".exe"
	}
	trayName := filepath.Base(exe)
	trayName = strings.TrimSuffix(trayName, filepath.Ext(trayName))
	suffix := strings.TrimPrefix(trayName, "tray")
	return base + suffix + ".exe"
}

// startProcess launches exeName with args, hidden (no console window),
// streaming its combined stdout+stderr as lines to onLine (called
// sequentially from a single goroutine, so onLine may keep state without
// its own locking) and reporting exit via onExit.
func startProcess(exeName string, args []string, onLine func(string), onExit func(error)) (*runningProc, error) {
	path, err := binPath(exeName)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(path); err != nil {
		return nil, fmt.Errorf("%s not found next to tray.exe: %w", exeName, err)
	}

	cmd := exec.Command(path, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("starting %s: %w", exeName, err)
	}

	lines := make(chan string, 64)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); scanInto(stdout, lines) }()
	go func() { defer wg.Done(); scanInto(stderr, lines) }()
	go func() { wg.Wait(); close(lines) }()
	go func() {
		for line := range lines {
			onLine(line)
		}
	}()

	go func() {
		onExit(cmd.Wait())
	}()

	return &runningProc{cmd: cmd}, nil
}

func scanInto(r io.Reader, out chan<- string) {
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		out <- sc.Text()
	}
	_ = sc.Err() // best-effort line forwarding; exit status is reported separately via cmd.Wait
}
