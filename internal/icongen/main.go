// Command icongen 画出所有平台用的图标；在仓库根目录运行 go run ./internal/icongen。
//
// 图标是粉色渐变的圆角底板上一台白色小电视，屏幕里是一枚完成的对勾。
// 网页的 web/static/icon.svg 是同一张图的矢量版，改这里时一起改。
package main

import (
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
)

func main() {
	save("app/resources/icon.png", tile(512, 116, 1))
	save("app/resources/tray.png", tray(32))
	// 手机系统自己裁圆角：铺满整张、不留透明，图案缩小一些留在安全区里
	save("mobile/assets/icon.png", tile(1024, 0, 0.84))
}

func save(name string, im image.Image) {
	f, err := os.Create(name)
	if err != nil {
		panic(err)
	}
	if err = png.Encode(f, im); err != nil {
		panic(err)
	}
	if err = f.Close(); err != nil {
		panic(err)
	}
}

/* 形状：都在 512×512 的画布上描述 */

type rgb [3]float64

var (
	white    = rgb{1, 1, 1}
	pinkFrom = rgb{1, 0.573, 0.702}     // #ff92b3，左上
	pinkTo   = rgb{0.925, 0.290, 0.518} // #ec4a84，右下
	tickPink = rgb{0.937, 0.337, 0.553} // #ef568d
	shadow   = rgb{0.722, 0.153, 0.376} // #b82760
)

func mix(a, b rgb, t float64) rgb {
	return rgb{a[0] + (b[0]-a[0])*t, a[1] + (b[1]-a[1])*t, a[2] + (b[2]-a[2])*t}
}

func rrect(px, py, x, y, w, h, r float64) bool {
	qx := math.Abs(px-(x+w/2)) - w/2 + r
	qy := math.Abs(py-(y+h/2)) - h/2 + r
	return math.Hypot(math.Max(qx, 0), math.Max(qy, 0))+math.Min(math.Max(qx, qy), 0) <= r
}

// capsule 是从 a 到 b、半径 r 的圆头线段
func capsule(px, py, ax, ay, bx, by, r float64) bool {
	dx, dy := bx-ax, by-ay
	t := math.Max(0, math.Min(1, ((px-ax)*dx+(py-ay)*dy)/(dx*dx+dy*dy)))
	return math.Hypot(px-ax-t*dx, py-ay-t*dy) <= r
}

// star 是四角星光
func star(px, py, cx, cy, r float64) bool {
	return math.Sqrt(math.Abs(px-cx))+math.Sqrt(math.Abs(py-cy)) <= math.Sqrt(r)
}

// 小电视画在 24×24 的格子上，和界面里的小标记用同一套坐标
type tv struct{ ox, oy, s float64 }

func (t tv) x(g float64) float64 { return t.ox + g*t.s }
func (t tv) y(g float64) float64 { return t.oy + g*t.s }

func (t tv) body(px, py float64) bool {
	return rrect(px, py, t.x(2.5), t.y(6.5), 19*t.s, 14*t.s, 4*t.s)
}

func (t tv) antennae(px, py float64) bool {
	return capsule(px, py, t.x(7.5), t.y(3.5), t.x(10), t.y(6.8), t.s) ||
		capsule(px, py, t.x(16.5), t.y(3.5), t.x(14), t.y(6.8), t.s)
}

func (t tv) feet(px, py float64) bool {
	return capsule(px, py, t.x(7.3), t.y(21.5), t.x(8.7), t.y(21.5), 0.85*t.s) ||
		capsule(px, py, t.x(15.3), t.y(21.5), t.x(16.7), t.y(21.5), 0.85*t.s)
}

func (t tv) tick(px, py, r float64) bool {
	return capsule(px, py, t.x(8), t.y(13.6), t.x(10.9), t.y(16.3), r*t.s) ||
		capsule(px, py, t.x(10.9), t.y(16.3), t.x(16.2), t.y(10.9), r*t.s)
}

/* 出图：每个像素取 4×4 个点求平均，边缘才平滑 */

const ss = 4

// tile 是带底板的图标。radius 是底板圆角（0 表示铺满），art 是图案相对画布的大小
func tile(size int, radius, art float64) *image.NRGBA {
	set := tv{ox: 256 - 12*15, oy: 256 - 12.4*15, s: 15}
	return render(size, 512, func(u, v float64) (rgb, float64) {
		if radius > 0 && !rrect(u, v, 0, 0, 512, 512, radius) {
			return rgb{}, 0
		}
		c := mix(pinkFrom, pinkTo, math.Max(0, math.Min(1, (u+v)/1024)))
		// 左上角的高光
		if d := math.Hypot(u-130, v-90) / 360; d < 1 {
			c = mix(c, white, 0.24*(1-d)*(1-d))
		}
		x, y := (u-256)/art+256, (v-256)/art+256
		if set.body(x, y-12) || set.feet(x, y-12) {
			c = mix(c, shadow, 0.2)
		}
		if set.body(x, y) || set.antennae(x, y) || set.feet(x, y) {
			c = white
		}
		if set.tick(x, y, 1.25) {
			c = tickPink
		}
		if star(x, y, 410, 112, 36) {
			c = mix(c, white, 0.95)
		}
		if math.Hypot(x-442, y-166) <= 7 {
			c = mix(c, white, 0.8)
		}
		return c, 1
	})
}

// tray 是托盘图标：单色，系统按深浅色菜单栏自己上色
func tray(size int) *image.NRGBA {
	set := tv{ox: 0, oy: 0.4, s: 1}
	return render(size, 24, func(u, v float64) (rgb, float64) {
		if (set.body(u, v) || set.antennae(u, v)) && !set.tick(u, v, 1.15) {
			return rgb{}, 1
		}
		return rgb{}, 0
	})
}

func render(size int, canvas float64, at func(u, v float64) (rgb, float64)) *image.NRGBA {
	im := image.NewNRGBA(image.Rect(0, 0, size, size))
	k := canvas / float64(size)
	for py := range size {
		for px := range size {
			var sum rgb
			var alpha float64
			for j := range ss {
				for i := range ss {
					c, a := at((float64(px)+(float64(i)+0.5)/ss)*k, (float64(py)+(float64(j)+0.5)/ss)*k)
					sum[0], sum[1], sum[2] = sum[0]+c[0]*a, sum[1]+c[1]*a, sum[2]+c[2]*a
					alpha += a
				}
			}
			if alpha == 0 {
				continue
			}
			b := func(v float64) uint8 { return uint8(math.Round(v / alpha * 255)) }
			im.SetNRGBA(px, py, color.NRGBA{R: b(sum[0]), G: b(sum[1]), B: b(sum[2]), A: uint8(math.Round(alpha / (ss * ss) * 255))})
		}
	}
	return im
}
