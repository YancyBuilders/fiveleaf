// Command icongen fits the light and dark app icon art of
// cmd/fiveleaf/icon-art.png on the macOS icon grid, as resources/icon.png and
// resources/icon-dark.png, with the logos the welcome page shows in
// cmd/fiveleaf/icon.png and cmd/fiveleaf/icon-dark.png:
//
//	go run ./tools/icongen
package main

import (
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"log"
	"math"
	"os"

	xdraw "golang.org/x/image/draw"
)

// tile is a square of the art of an icon, in its pixels.
type tile struct {
	x, y, side int
}

// The art's tiles: the light one first, and the dark one, in the 1536 px
// art. Each is cropped just inside the rounded corner drawn around it.
var (
	lightTile = tile{x: 66, y: 66, side: 664}
	darkTile  = tile{x: 804, y: 804, side: 664}
)

func main() {
	f, err := os.Open("cmd/fiveleaf/icon-art.png")
	if err != nil {
		log.Fatal(err)
	}
	art, err := png.Decode(f)
	f.Close()
	if err != nil {
		log.Fatal(err)
	}
	// The app icon: the macOS grid's 824 DIP squircle on a 1024 canvas,
	// with the art's own tile round.
	writeIcon("resources/icon.png", art, lightTile)
	writeIcon("resources/icon-dark.png", art, darkTile)
	// The welcome page's logo: the tile itself, at 52 DIPs.
	writeLogo("cmd/fiveleaf/icon.png", art, lightTile)
	writeLogo("cmd/fiveleaf/icon-dark.png", art, darkTile)
}

// writeIcon fits the art's tile t on the macOS icon grid, writing a
// transparent 1024 px PNG.
func writeIcon(name string, art image.Image, t tile) {
	const side, inset, radius = 1024, 100, 185
	inner := side - 2*inset
	canvas := image.NewRGBA(image.Rect(0, 0, side, side))
	draw.DrawMask(canvas, image.Rect(inset, inset, inset+inner, inset+inner),
		scaleTile(art, t, inner), image.Point{}, roundedMask(inner, radius), image.Point{}, draw.Over)
	writePNG(name, canvas)
}

// writeLogo writes the art's tile t at 256 px, its rounded corners cut
// from the square.
func writeLogo(name string, art image.Image, t tile) {
	const side = 256
	canvas := image.NewRGBA(image.Rect(0, 0, side, side))
	draw.DrawMask(canvas, canvas.Bounds(), scaleTile(art, t, side), image.Point{},
		roundedMask(side, side*185/824), image.Point{}, draw.Over)
	writePNG(name, canvas)
}

// scaleTile crops the art's tile t just inside its rounded corner, and
// scales it to side pixels square.
func scaleTile(art image.Image, t tile, side int) image.Image {
	r := image.Rect(t.x+10, t.y+10, t.x+t.side-10, t.y+t.side-10)
	dst := image.NewRGBA(image.Rect(0, 0, side, side))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), art, r, xdraw.Src, nil)
	return dst
}

// roundedMask is a white square with round corners, side pixels square,
// each corner a quarter of a circle of radius, smoothed at its edge.
func roundedMask(side int, radius float64) *image.Alpha {
	mask := image.NewAlpha(image.Rect(0, 0, side, side))
	for y := 0; y < side; y++ {
		for x := 0; x < side; x++ {
			cx, cy := float64(x)+0.5, float64(y)+0.5
			dx, dy := 0.0, 0.0
			switch {
			case cx < radius:
				dx = radius - cx
			case cx > float64(side)-radius:
				dx = cx - (float64(side) - radius)
			}
			switch {
			case cy < radius:
				dy = radius - cy
			case cy > float64(side)-radius:
				dy = cy - (float64(side) - radius)
			}
			d := math.Hypot(dx, dy)
			switch {
			case d <= radius-0.5:
				mask.SetAlpha(x, y, color.Alpha{A: 0xff})
			case d < radius+0.5:
				mask.SetAlpha(x, y, color.Alpha{A: uint8(255 * (radius + 0.5 - d))})
			}
		}
	}
	return mask
}

func writePNG(name string, img image.Image) {
	out, err := os.Create(name)
	if err != nil {
		log.Fatal(err)
	}
	defer out.Close()
	if err := png.Encode(out, img); err != nil {
		log.Fatal(err)
	}
}
