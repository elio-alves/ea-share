//go:build windows

package main

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
)

// generateIcon renders a simple two-overlapping-squares glyph (representing
// two machines) and wraps it as a Windows .ico (PNG-in-ICO, supported since
// Vista) so the tray doesn't need an external asset file.
func generateIcon() []byte {
	const size = 32
	img := image.NewRGBA(image.Rect(0, 0, size, size))

	bg := color.RGBA{0x3E, 0x63, 0xE0, 0xFF}
	fg := color.RGBA{0xFF, 0xFF, 0xFF, 0xFF}

	fillRoundedRect(img, 2, 2, 21, 21, 5, bg)
	fillRoundedRect(img, 12, 12, 29, 29, 5, bg) // border/gap for the overlap
	fillRoundedRect(img, 14, 14, 27, 27, 4, fg)

	var pngBuf bytes.Buffer
	if err := png.Encode(&pngBuf, img); err != nil {
		return nil
	}
	return pngToICO(pngBuf.Bytes(), size, size)
}

func fillRoundedRect(img *image.RGBA, x0, y0, x1, y1, radius int, col color.RGBA) {
	for y := y0; y <= y1; y++ {
		for x := x0; x <= x1; x++ {
			if insideRoundedRect(x, y, x0, y0, x1, y1, radius) {
				img.SetRGBA(x, y, col)
			}
		}
	}
}

func insideRoundedRect(x, y, x0, y0, x1, y1, r int) bool {
	if x < x0 || x > x1 || y < y0 || y > y1 {
		return false
	}
	left, top, right, bottom := x < x0+r, y < y0+r, x > x1-r, y > y1-r
	switch {
	case left && top:
		return withinCircle(x, y, x0+r, y0+r, r)
	case right && top:
		return withinCircle(x, y, x1-r, y0+r, r)
	case left && bottom:
		return withinCircle(x, y, x0+r, y1-r, r)
	case right && bottom:
		return withinCircle(x, y, x1-r, y1-r, r)
	default:
		return true
	}
}

func withinCircle(x, y, cx, cy, r int) bool {
	dx, dy := float64(x-cx), float64(y-cy)
	return dx*dx+dy*dy <= float64(r*r)
}

// pngToICO wraps a single PNG image in a minimal .ico container. Windows
// Vista+ supports PNG-compressed icon frames directly, so no legacy
// BMP+AND-mask encoding is needed.
func pngToICO(pngData []byte, w, h int) []byte {
	var buf bytes.Buffer
	// ICONDIR
	binary.Write(&buf, binary.LittleEndian, uint16(0)) // reserved
	binary.Write(&buf, binary.LittleEndian, uint16(1)) // type: icon
	binary.Write(&buf, binary.LittleEndian, uint16(1)) // image count

	widthByte, heightByte := byte(w), byte(h)
	if w >= 256 {
		widthByte = 0
	}
	if h >= 256 {
		heightByte = 0
	}

	// ICONDIRENTRY
	buf.WriteByte(widthByte)
	buf.WriteByte(heightByte)
	buf.WriteByte(0)                                    // color count
	buf.WriteByte(0)                                    // reserved
	binary.Write(&buf, binary.LittleEndian, uint16(1))  // planes
	binary.Write(&buf, binary.LittleEndian, uint16(32)) // bit count
	binary.Write(&buf, binary.LittleEndian, uint32(len(pngData)))
	binary.Write(&buf, binary.LittleEndian, uint32(22)) // offset: 6 (ICONDIR) + 16 (ICONDIRENTRY)

	buf.Write(pngData)
	return buf.Bytes()
}
