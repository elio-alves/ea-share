//go:build windows

// Package clipboard reads and writes the local Windows clipboard, in
// support of the kbs clipboard-transfer hotkey (Ctrl+Alt+V). Only plain
// text (CF_UNICODETEXT) and images (CF_DIB, exchanged over the wire as
// PNG) are supported.
package clipboard

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math/bits"
	"syscall"
	"time"
	"unicode/utf16"
	"unsafe"
)

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")

	procOpenClipboard    = user32.NewProc("OpenClipboard")
	procCloseClipboard   = user32.NewProc("CloseClipboard")
	procEmptyClipboard   = user32.NewProc("EmptyClipboard")
	procGetClipboardData = user32.NewProc("GetClipboardData")
	procSetClipboardData = user32.NewProc("SetClipboardData")

	procGlobalAlloc  = kernel32.NewProc("GlobalAlloc")
	procGlobalLock   = kernel32.NewProc("GlobalLock")
	procGlobalUnlock = kernel32.NewProc("GlobalUnlock")
)

const (
	cfUnicodeText = 13
	cfDIB         = 8

	gmemMoveable = 0x0002

	biRGB       = 0
	biBitfields = 3
)

// bitmapInfoHeader mirrors Win32's BITMAPINFOHEADER (40 bytes), the format
// CF_DIB clipboard data starts with.
type bitmapInfoHeader struct {
	Size          uint32
	Width         int32
	Height        int32
	Planes        uint16
	BitCount      uint16
	Compression   uint32
	SizeImage     uint32
	XPelsPerMeter int32
	YPelsPerMeter int32
	ClrUsed       uint32
	ClrImportant  uint32
}

func openClipboard() error {
	var lastErr error
	for i := 0; i < 10; i++ {
		r, _, err := procOpenClipboard.Call(0)
		if r != 0 {
			return nil
		}
		lastErr = err
		time.Sleep(20 * time.Millisecond)
	}
	return fmt.Errorf("clipboard: OpenClipboard failed: %v", lastErr)
}

// ReadText returns the clipboard's text content, if any (ok is false when
// the clipboard doesn't currently hold CF_UNICODETEXT).
func ReadText() (text string, ok bool, err error) {
	if err := openClipboard(); err != nil {
		return "", false, err
	}
	defer procCloseClipboard.Call()

	h, _, _ := procGetClipboardData.Call(cfUnicodeText)
	if h == 0 {
		return "", false, nil
	}
	ptr, _, _ := procGlobalLock.Call(h)
	if ptr == 0 {
		return "", false, fmt.Errorf("clipboard: GlobalLock failed")
	}
	defer procGlobalUnlock.Call(h)

	var u16 []uint16
	for i := 0; ; i++ {
		c := *(*uint16)(unsafe.Pointer(ptr + uintptr(i*2)))
		if c == 0 {
			break
		}
		u16 = append(u16, c)
	}
	return string(utf16.Decode(u16)), true, nil
}

// WriteText sets the clipboard to s as CF_UNICODETEXT.
func WriteText(s string) error {
	u16, err := syscall.UTF16FromString(s)
	if err != nil {
		return err
	}
	size := len(u16) * 2

	if err := openClipboard(); err != nil {
		return err
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
	dst := unsafe.Slice((*uint16)(unsafe.Pointer(ptr)), len(u16))
	copy(dst, u16)
	procGlobalUnlock.Call(h)

	if r, _, _ := procSetClipboardData.Call(cfUnicodeText, h); r == 0 {
		return fmt.Errorf("clipboard: SetClipboardData failed")
	}
	return nil
}

// ReadImagePNG returns the clipboard's image content PNG-encoded, if any
// (ok is false when the clipboard doesn't currently hold a supported
// CF_DIB image). Only uncompressed 24/32bpp DIBs are understood; alpha is
// ignored (screenshots are always opaque in practice, and many apps leave
// the 32bpp alpha byte unused/zero, which would otherwise render as a
// fully transparent image).
func ReadImagePNG() (pngData []byte, ok bool, err error) {
	if err := openClipboard(); err != nil {
		return nil, false, err
	}
	defer procCloseClipboard.Call()

	h, _, _ := procGetClipboardData.Call(cfDIB)
	if h == 0 {
		return nil, false, nil
	}
	ptr, _, _ := procGlobalLock.Call(h)
	if ptr == 0 {
		return nil, false, fmt.Errorf("clipboard: GlobalLock failed")
	}
	defer procGlobalUnlock.Call(h)

	var hdr bitmapInfoHeader
	hdrSize := int(unsafe.Sizeof(hdr))
	hdrBytes := unsafe.Slice((*byte)(unsafe.Pointer(ptr)), hdrSize)
	if err := binary.Read(bytes.NewReader(hdrBytes), binary.LittleEndian, &hdr); err != nil {
		return nil, false, err
	}
	if hdr.Compression != biRGB && hdr.Compression != biBitfields {
		return nil, false, fmt.Errorf("clipboard: unsupported DIB compression %d", hdr.Compression)
	}
	if hdr.BitCount != 24 && hdr.BitCount != 32 {
		return nil, false, fmt.Errorf("clipboard: unsupported DIB bit depth %d", hdr.BitCount)
	}
	if hdr.Compression == biBitfields && hdr.BitCount != 32 {
		return nil, false, fmt.Errorf("clipboard: BI_BITFIELDS only supported for 32bpp DIBs, got %dbpp", hdr.BitCount)
	}

	width := int(hdr.Width)
	height := int(hdr.Height)
	topDown := height < 0
	if topDown {
		height = -height
	}
	if width <= 0 || height <= 0 {
		return nil, false, fmt.Errorf("clipboard: invalid DIB size %dx%d", width, height)
	}

	// BI_RGB defaults to the standard byte order; BI_BITFIELDS (near-
	// universal for 32bpp screenshots on modern Windows) spells out the
	// channel masks explicitly in 3 extra DWORDs right after the header.
	pixelOffset := hdrSize
	rMask, gMask, bMask := uint32(0x00FF0000), uint32(0x0000FF00), uint32(0x000000FF)
	if hdr.Compression == biBitfields {
		maskBytes := unsafe.Slice((*byte)(unsafe.Pointer(ptr+uintptr(hdrSize))), 12)
		rMask = binary.LittleEndian.Uint32(maskBytes[0:4])
		gMask = binary.LittleEndian.Uint32(maskBytes[4:8])
		bMask = binary.LittleEndian.Uint32(maskBytes[8:12])
		pixelOffset += 12
	}
	rShift, gShift, bShift := bits.TrailingZeros32(rMask), bits.TrailingZeros32(gMask), bits.TrailingZeros32(bMask)

	bytesPerPixel := int(hdr.BitCount) / 8
	rowSize := ((width*bytesPerPixel + 3) / 4) * 4
	pixelDataSize := rowSize * height
	pixels := unsafe.Slice((*byte)(unsafe.Pointer(ptr+uintptr(pixelOffset))), pixelDataSize)

	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		srcY := y
		if !topDown {
			srcY = height - 1 - y
		}
		rowStart := srcY * rowSize
		for x := 0; x < width; x++ {
			i := rowStart + x*bytesPerPixel
			var r, g, b byte
			if bytesPerPixel == 4 {
				v := binary.LittleEndian.Uint32(pixels[i : i+4])
				r = byte((v & rMask) >> rShift)
				g = byte((v & gMask) >> gShift)
				b = byte((v & bMask) >> bShift)
			} else {
				b, g, r = pixels[i], pixels[i+1], pixels[i+2]
			}
			img.SetRGBA(x, y, color.RGBA{R: r, G: g, B: b, A: 255})
		}
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, false, err
	}
	return buf.Bytes(), true, nil
}

// WriteImagePNG decodes pngData and sets the clipboard to it as a 32bpp
// CF_DIB (bottom-up, BI_RGB); Windows auto-derives CF_BITMAP from CF_DIB
// for consumers that ask for the older format.
func WriteImagePNG(pngData []byte) error {
	img, err := png.Decode(bytes.NewReader(pngData))
	if err != nil {
		return err
	}
	b := img.Bounds()
	width, height := b.Dx(), b.Dy()
	if width <= 0 || height <= 0 {
		return fmt.Errorf("clipboard: invalid image size %dx%d", width, height)
	}

	const bytesPerPixel = 4
	rowSize := width * bytesPerPixel
	pixelDataSize := rowSize * height

	hdr := bitmapInfoHeader{
		Size:        40,
		Width:       int32(width),
		Height:      int32(height), // positive: bottom-up, widest app compatibility
		Planes:      1,
		BitCount:    32,
		Compression: 0,
		SizeImage:   uint32(pixelDataSize),
	}
	var hdrBuf bytes.Buffer
	if err := binary.Write(&hdrBuf, binary.LittleEndian, hdr); err != nil {
		return err
	}
	total := hdrBuf.Len() + pixelDataSize

	if err := openClipboard(); err != nil {
		return err
	}
	defer procCloseClipboard.Call()
	procEmptyClipboard.Call()

	h, _, _ := procGlobalAlloc.Call(gmemMoveable, uintptr(total))
	if h == 0 {
		return fmt.Errorf("clipboard: GlobalAlloc failed")
	}
	ptr, _, _ := procGlobalLock.Call(h)
	if ptr == 0 {
		return fmt.Errorf("clipboard: GlobalLock failed")
	}

	dst := unsafe.Slice((*byte)(unsafe.Pointer(ptr)), total)
	copy(dst, hdrBuf.Bytes())
	pixels := dst[hdrBuf.Len():]
	for y := 0; y < height; y++ {
		dstY := height - 1 - y // bottom-up
		rowStart := dstY * rowSize
		for x := 0; x < width; x++ {
			r, g, bl, _ := img.At(b.Min.X+x, b.Min.Y+y).RGBA()
			i := rowStart + x*bytesPerPixel
			pixels[i] = byte(bl >> 8)
			pixels[i+1] = byte(g >> 8)
			pixels[i+2] = byte(r >> 8)
			pixels[i+3] = 0
		}
	}
	procGlobalUnlock.Call(h)

	if r, _, _ := procSetClipboardData.Call(cfDIB, h); r == 0 {
		return fmt.Errorf("clipboard: SetClipboardData failed")
	}
	return nil
}
