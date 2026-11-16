//go:build ignore

package main

import (
	"image"
	"image/color"
	"image/png"
	"log"
	"os"
)

func roundedRectangle(canvas *image.NRGBA, bounds image.Rectangle, radius int, ink color.NRGBA) {
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			nearestX := max(bounds.Min.X+radius, min(x, bounds.Max.X-radius-1))
			nearestY := max(bounds.Min.Y+radius, min(y, bounds.Max.Y-radius-1))
			dx, dy := x-nearestX, y-nearestY
			if dx*dx+dy*dy <= radius*radius {
				canvas.SetNRGBA(x, y, ink)
			}
		}
	}
}

func main() {
	canvas := image.NewNRGBA(image.Rect(0, 0, 512, 512))
	green := color.NRGBA{R: 52, G: 82, B: 69, A: 255}
	page := color.NRGBA{R: 249, G: 250, B: 244, A: 255}
	backPage := color.NRGBA{R: 144, G: 172, B: 151, A: 255}
	roundedRectangle(canvas, image.Rect(24, 24, 488, 488), 96, green)
	roundedRectangle(canvas, image.Rect(132, 154, 322, 406), 20, backPage)
	roundedRectangle(canvas, image.Rect(186, 104, 376, 376), 20, page)
	roundedRectangle(canvas, image.Rect(222, 188, 340, 204), 8, green)
	roundedRectangle(canvas, image.Rect(222, 234, 314, 250), 8, green)
	roundedRectangle(canvas, image.Rect(222, 292, 246, 316), 12, green)
	file, err := os.Create("build/appicon.png")
	if err != nil {
		log.Fatal(err)
	}
	if err := png.Encode(file, canvas); err != nil {
		file.Close()
		log.Fatal(err)
	}
	if err := file.Close(); err != nil {
		log.Fatal(err)
	}
}
