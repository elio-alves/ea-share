//go:build windows

package main

import (
	"fmt"
	"syscall"
	"unsafe"
)

var (
	user32c              = syscall.NewLazyDLL("user32.dll")
	kernel32c            = syscall.NewLazyDLL("kernel32.dll")
	procOpenClipboard    = user32c.NewProc("OpenClipboard")
	procCloseClipboard   = user32c.NewProc("CloseClipboard")
	procEmptyClipboard   = user32c.NewProc("EmptyClipboard")
	procSetClipboardData = user32c.NewProc("SetClipboardData")
	procGlobalAlloc      = kernel32c.NewProc("GlobalAlloc")
	procGlobalLock       = kernel32c.NewProc("GlobalLock")
	procGlobalUnlock     = kernel32c.NewProc("GlobalUnlock")
)

const (
	cfUnicodeText = 13
	gmemMoveable  = 0x0002
)

// setClipboardText puts s on the Windows clipboard as CF_UNICODETEXT.
func setClipboardText(s string) error {
	utf16, err := syscall.UTF16FromString(s)
	if err != nil {
		return err
	}
	size := len(utf16) * 2

	r, _, _ := procOpenClipboard.Call(0)
	if r == 0 {
		return fmt.Errorf("clipboard: OpenClipboard failed")
	}
	defer procCloseClipboard.Call()
	procEmptyClipboard.Call()

	h, _, _ := procGlobalAlloc.Call(gmemMoveable, uintptr(size))
	if h == 0 {
		return fmt.Errorf("clipboard: GlobalAlloc failed")
	}
	ptr, _, _ := procGlobalLock.Call(h)
	if ptr == 0 {
		return fmt.Errorf("clipboard: GlobalLock failed")
	}
	dst := unsafe.Slice((*uint16)(unsafe.Pointer(ptr)), len(utf16))
	copy(dst, utf16)
	procGlobalUnlock.Call(h)

	r, _, _ = procSetClipboardData.Call(cfUnicodeText, h)
	if r == 0 {
		return fmt.Errorf("clipboard: SetClipboardData failed")
	}
	return nil
}
