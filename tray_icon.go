package main

import (
	"bytes"
	"encoding/binary"
)

func trayIconBytes() []byte {
	const (
		width      = 32
		height     = 32
		colorBytes = width * height * 4
		maskBytes  = width * height / 8
		dibBytes   = 40 + colorBytes + maskBytes
		imageOff   = 22
	)

	pixels := make([]byte, colorBytes)
	set := func(x, y int, r, g, b, a byte) {
		if x < 0 || y < 0 || x >= width || y >= height {
			return
		}
		row := height - 1 - y
		i := (row*width + x) * 4
		pixels[i+0] = b
		pixels[i+1] = g
		pixels[i+2] = r
		pixels[i+3] = a
	}

	for y := 6; y <= 22; y++ {
		for x := 4; x <= 27; x++ {
			switch {
			case x == 4 || x == 27 || y == 6 || y == 22:
				set(x, y, 245, 248, 250, 255)
			case y <= 10:
				set(x, y, 37, 99, 235, 255)
			default:
				set(x, y, 16, 185, 129, 255)
			}
		}
	}
	for y := 23; y <= 25; y++ {
		for x := 13; x <= 18; x++ {
			set(x, y, 245, 248, 250, 255)
		}
	}
	for y := 26; y <= 27; y++ {
		for x := 9; x <= 22; x++ {
			set(x, y, 245, 248, 250, 255)
		}
	}
	for y := 12; y <= 18; y++ {
		for x := 9; x <= 22; x++ {
			if x == 9 || x == 22 || y == 12 || y == 18 {
				set(x, y, 15, 23, 42, 255)
			}
		}
	}

	var out bytes.Buffer
	write16 := func(v uint16) { _ = binary.Write(&out, binary.LittleEndian, v) }
	write32 := func(v uint32) { _ = binary.Write(&out, binary.LittleEndian, v) }

	write16(0)
	write16(1)
	write16(1)
	out.WriteByte(width)
	out.WriteByte(height)
	out.WriteByte(0)
	out.WriteByte(0)
	write16(1)
	write16(32)
	write32(dibBytes)
	write32(imageOff)

	write32(40)
	write32(width)
	write32(height * 2)
	write16(1)
	write16(32)
	write32(0)
	write32(colorBytes)
	write32(0)
	write32(0)
	write32(0)
	write32(0)
	out.Write(pixels)
	out.Write(make([]byte, maskBytes))

	return out.Bytes()
}
