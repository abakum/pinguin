// Command mkico converts a PNG image to a multi-size Windows ICO file.
//
// Usage: go run ./cmd/mkico [input.png] [output.ico]
package main

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"image"
	"image/png"
	"os"

	"golang.org/x/image/draw"
)

var sizes = []int{256, 128, 64, 48, 32, 16}

func main() {
	in, out := "pinguin.png", "pinguin.ico"
	if len(os.Args) > 1 {
		in = os.Args[1]
	}
	if len(os.Args) > 2 {
		out = os.Args[2]
	}
	f, err := os.Open(in)
	check(err)
	src, err := png.Decode(f)
	f.Close()
	check(err)

	var blobs [][]byte
	for _, s := range sizes {
		dst := image.NewRGBA(image.Rect(0, 0, s, s))
		draw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Over, nil)
		var bb bytes.Buffer
		w := bufio.NewWriter(&bb)
		check(png.Encode(w, dst))
		check(w.Flush())
		blobs = append(blobs, bb.Bytes())
	}
	check(writeIco(out, blobs))
}

// writeIco writes an ICO container with PNG-encoded images.
func writeIco(path string, blobs [][]byte) error {
	var buf bytes.Buffer
	hdr := []uint16{0, 1, uint16(len(blobs))}
	check(binary.Write(&buf, binary.LittleEndian, hdr))
	offset := uint32(6 + 16*len(blobs))
	for i, b := range blobs {
		s := sizes[i]
		w := uint8(s)
		if s >= 256 {
			w = 0
		}
		entry := []interface{}{
			w, w, uint8(0), uint8(0),
			uint16(1), uint16(32),
			uint32(len(b)), offset,
		}
		for _, v := range entry {
			check(binary.Write(&buf, binary.LittleEndian, v))
		}
		offset += uint32(len(b))
	}
	for _, b := range blobs {
		buf.Write(b)
	}
	return os.WriteFile(path, buf.Bytes(), 0o644)
}

func check(err error) {
	if err != nil {
		panic(err)
	}
}
