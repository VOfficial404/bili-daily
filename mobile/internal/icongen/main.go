// Command icongen 生成 App 图标；在 mobile 目录运行 go run ./internal/icongen。
package main

import (
	"image"
	"image/color"
	"image/png"
	"os"
)

func main() {
	img := image.NewRGBA(image.Rect(0, 0, 1024, 1024))
	pink := color.RGBA{217, 65, 117, 255}
	white := color.RGBA{255, 255, 255, 255}
	for y := range 1024 {
		for x := range 1024 {
			img.SetRGBA(x, y, pink)
		}
	}
	fill := func(x0, y0, x1, y1 int, c color.RGBA) {
		for y := y0; y < y1; y++ {
			for x := x0; x < x1; x++ {
				img.SetRGBA(x, y, c)
			}
		}
	}
	fill(210, 270, 814, 754, white)
	fill(254, 314, 770, 710, pink)
	fill(332, 402, 380, 492, white)
	fill(644, 402, 692, 492, white)
	for n := range 125 {
		fill(382+n, 543+n/2, 426+n, 577+n/2, white)
	}
	for n := range 160 {
		fill(490+n, 605-n/2, 530+n, 639-n/2, white)
	}
	for n := range 100 {
		fill(320+n, 168+n, 360+n, 200+n, white)
		fill(664-n, 168+n, 704-n, 200+n, white)
	}
	if err := os.MkdirAll("assets", 0o755); err != nil {
		panic(err)
	}
	f, err := os.Create("assets/icon.png")
	if err != nil {
		panic(err)
	}
	if err = png.Encode(f, img); err != nil {
		panic(err)
	}
	if err = f.Close(); err != nil {
		panic(err)
	}
}
