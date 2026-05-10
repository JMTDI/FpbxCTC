// Run with: go run ./tools/mkico <input.png> <output.ico>
package main

import (
	"bytes"
	"encoding/binary"
	"image/png"
	"os"
)

func main() {
	if len(os.Args) != 3 {
		os.Stderr.WriteString("usage: mkico <input.png> <output.ico>\n")
		os.Exit(1)
	}

	f, err := os.Open(os.Args[1])
	if err != nil {
		os.Stderr.WriteString("open: " + err.Error() + "\n")
		os.Exit(1)
	}
	img, err := png.Decode(f)
	f.Close()
	if err != nil {
		os.Stderr.WriteString("decode: " + err.Error() + "\n")
		os.Exit(1)
	}

	// Re-encode PNG cleanly (strips any unknown chunks)
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		os.Stderr.WriteString("encode: " + err.Error() + "\n")
		os.Exit(1)
	}
	pngData := buf.Bytes()

	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()

	out, err := os.Create(os.Args[2])
	if err != nil {
		os.Stderr.WriteString("create: " + err.Error() + "\n")
		os.Exit(1)
	}
	defer out.Close()

	// ICONDIR header (6 bytes)
	binary.Write(out, binary.LittleEndian, uint16(0)) // idReserved
	binary.Write(out, binary.LittleEndian, uint16(1)) // idType = 1 (icon)
	binary.Write(out, binary.LittleEndian, uint16(1)) // idCount = 1

	// ICONDIRENTRY (16 bytes)
	// Width/height: 0 means 256 in the ICO format
	wByte, hByte := byte(w), byte(h)
	if w >= 256 {
		wByte = 0
	}
	if h >= 256 {
		hByte = 0
	}
	imageOffset := uint32(6 + 16) // ICONDIR + one ICONDIRENTRY
	binary.Write(out, binary.LittleEndian, wByte)                // bWidth
	binary.Write(out, binary.LittleEndian, hByte)                // bHeight
	binary.Write(out, binary.LittleEndian, uint8(0))             // bColorCount
	binary.Write(out, binary.LittleEndian, uint8(0))             // bReserved
	binary.Write(out, binary.LittleEndian, uint16(1))            // wPlanes
	binary.Write(out, binary.LittleEndian, uint16(32))           // wBitCount
	binary.Write(out, binary.LittleEndian, uint32(len(pngData))) // dwBytesInRes
	binary.Write(out, binary.LittleEndian, imageOffset)          // dwImageOffset

	// PNG image data (Vista+ ICO format supports embedded PNGs)
	out.Write(pngData)
}
