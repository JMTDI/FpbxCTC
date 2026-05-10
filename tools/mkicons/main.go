// Run with: go run ./tools/mkicons <input.png> <output-dir>
// Generates icon16.png, icon32.png, icon48.png, icon128.png in <output-dir>.
package main

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
)

var sizes = []int{16, 32, 48, 128}

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: mkicons <input.png> <output-dir>")
		os.Exit(1)
	}

	f, err := os.Open(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, "open:", err)
		os.Exit(1)
	}
	src, _, err := image.Decode(f)
	f.Close()
	if err != nil {
		fmt.Fprintln(os.Stderr, "decode:", err)
		os.Exit(1)
	}

	outDir := os.Args[2]
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, "mkdir:", err)
		os.Exit(1)
	}

	for _, sz := range sizes {
		dst := resize(src, sz, sz)
		name := filepath.Join(outDir, fmt.Sprintf("icon%d.png", sz))
		if err := writePNG(name, dst); err != nil {
			fmt.Fprintln(os.Stderr, "write:", err)
			os.Exit(1)
		}
	}
}

// resize returns a new image scaled to (w × h) using bilinear interpolation.
func resize(src image.Image, w, h int) *image.NRGBA {
	sb := src.Bounds()
	sw := float64(sb.Dx())
	sh := float64(sb.Dy())
	dst := image.NewNRGBA(image.Rect(0, 0, w, h))

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			// map destination pixel centre to source coordinates
			sx := (float64(x)+0.5)*sw/float64(w) - 0.5
			sy := (float64(y)+0.5)*sh/float64(h) - 0.5

			x0 := int(sx)
			y0 := int(sy)
			x1 := x0 + 1
			y1 := y0 + 1

			// fractional parts
			fx := sx - float64(x0)
			fy := sy - float64(y0)

			// clamp to source bounds
			clampX := func(v int) int {
				if v < 0 {
					return 0
				}
				if v >= sb.Dx() {
					return sb.Dx() - 1
				}
				return v
			}
			clampY := func(v int) int {
				if v < 0 {
					return 0
				}
				if v >= sb.Dy() {
					return sb.Dy() - 1
				}
				return v
			}

			c00 := toNRGBA(src.At(sb.Min.X+clampX(x0), sb.Min.Y+clampY(y0)))
			c10 := toNRGBA(src.At(sb.Min.X+clampX(x1), sb.Min.Y+clampY(y0)))
			c01 := toNRGBA(src.At(sb.Min.X+clampX(x0), sb.Min.Y+clampY(y1)))
			c11 := toNRGBA(src.At(sb.Min.X+clampX(x1), sb.Min.Y+clampY(y1)))

			dst.SetNRGBA(x, y, color.NRGBA{
				R: lerp2(c00.R, c10.R, c01.R, c11.R, fx, fy),
				G: lerp2(c00.G, c10.G, c01.G, c11.G, fx, fy),
				B: lerp2(c00.B, c10.B, c01.B, c11.B, fx, fy),
				A: lerp2(c00.A, c10.A, c01.A, c11.A, fx, fy),
			})
		}
	}
	return dst
}

func toNRGBA(c color.Color) color.NRGBA {
	r, g, b, a := c.RGBA() // 0–65535
	if a == 0 {
		return color.NRGBA{}
	}
	// un-premultiply
	return color.NRGBA{
		R: uint8(r * 0xff / a),
		G: uint8(g * 0xff / a),
		B: uint8(b * 0xff / a),
		A: uint8(a >> 8),
	}
}

func lerp2(v00, v10, v01, v11 uint8, fx, fy float64) uint8 {
	top := float64(v00)*(1-fx) + float64(v10)*fx
	bot := float64(v01)*(1-fx) + float64(v11)*fx
	v := top*(1-fy) + bot*fy
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return uint8(v)
}

func writePNG(path string, img image.Image) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}
