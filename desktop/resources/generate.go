//go:build ignore

// 生成原创图标：电视轮廓里的一枚完成标记。
package main

import (
	"image"
	"image/color"
	"image/png"
	"os"
)

func main() {
	for _, s := range []struct {
		name string
		size int
		tray bool
	}{{"resources/icon.png", 512, false}, {"resources/tray.png", 32, true}} {
		im := image.NewNRGBA(image.Rect(0, 0, s.size, s.size))
		for y := range s.size {
			for x := range s.size {
				fx, fy := float64(x)/float64(s.size), float64(y)/float64(s.size)
				c := color.NRGBA{}
				if !s.tray {
					c = color.NRGBA{R: 239, G: 75, B: 122, A: 255}
				}
				body := fx > .17 && fx < .83 && fy > .26 && fy < .79
				inner := fx > .23 && fx < .77 && fy > .32 && fy < .73
				aerial := (fy > .12 && fy < .27) && ((fx > .32 && fx < .38) || (fx > .62 && fx < .68))
				tick := (fx > .34 && fx < .49 && fy > .46 && fy < .62 && fy > fx+.05 && fy < fx+.18) || (fx >= .46 && fx < .68 && fy > .41 && fy < .65 && fy > 1.00-fx && fy < 1.13-fx)
				if (body && !inner) || aerial || tick {
					if s.tray {
						c = color.NRGBA{A: 255}
					} else {
						c = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
					}
				}
				im.SetNRGBA(x, y, c)
			}
		}
		f, e := os.Create(s.name)
		if e != nil {
			panic(e)
		}
		if e = png.Encode(f, im); e != nil {
			panic(e)
		}
		if e = f.Close(); e != nil {
			panic(e)
		}
	}
}
