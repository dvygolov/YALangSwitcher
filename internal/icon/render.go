// Package icon renders YALangSwitcher's tray badge in pure Go: a rounded
// square with the letter of the language (A = English, Я = Russian). No image
// assets, no fonts, no CGO - the same code produces the .ico files at build
// time and the runtime icon, so the badge is identical everywhere.
package icon

import (
	"image"
	"image/color"
	"math"
)

// Palette for the badges.
var (
	enTop    = color.RGBA{0x54, 0x8D, 0xFF, 0xFF}
	enBottom = color.RGBA{0x1B, 0x45, 0xC9, 0xFF}
	ruTop    = color.RGBA{0xFF, 0x7B, 0x5C, 0xFF}
	ruBottom = color.RGBA{0xB5, 0x22, 0x18, 0xFF}
	white    = color.RGBA{0xFF, 0xFF, 0xFF, 0xFF}
)

type point struct{ x, y float64 }

type segment struct{ a, b point }

// shape is a stroked polyline living in icon coordinates (0..1 = the icon
// square), with a round stroke of radius r.
type shape struct {
	segs []segment
	r    float64
}

func (s shape) inside(x, y float64) bool {
	best := math.Inf(1)
	for _, sg := range s.segs {
		if d := distToSegment(x, y, sg.a, sg.b); d < best {
			best = d
		}
	}
	return best <= s.r
}

func distToSegment(px, py float64, a, b point) float64 {
	vx, vy := b.x-a.x, b.y-a.y
	wx, wy := px-a.x, py-a.y
	den := vx*vx + vy*vy
	t := 0.0
	if den > 0 {
		t = (wx*vx + wy*vy) / den
		if t < 0 {
			t = 0
		} else if t > 1 {
			t = 1
		}
	}
	dx, dy := px-(a.x+t*vx), py-(a.y+t*vy)
	return math.Hypot(dx, dy)
}

// letterA returns the A glyph centred at (cx, cy) with height h.
func letterA(cx, cy, h float64) shape {
	hw := 0.80 * h
	x0, x1 := cx-hw/2, cx+hw/2
	top, bot := cy-h/2, cy+h/2
	bar := cy + 0.16*h
	return shape{
		segs: []segment{
			{point{x0, bot}, point{cx, top}},
			{point{cx, top}, point{x1, bot}},
			{point{x0 + 0.20*hw, bar}, point{x1 - 0.20*hw, bar}},
		},
		r: 0.085 * h,
	}
}

// letterYa returns the Cyrillic Я glyph centred at (cx, cy) with height h.
func letterYa(cx, cy, h float64) shape {
	hw := 0.84 * h
	x0, x1 := cx-hw/2, cx+hw/2
	top, bot := cy-h/2, cy+h/2
	mid := cy + 0.04*h
	return shape{
		segs: []segment{
			{point{x1, bot}, point{x1, top}},           // stem
			{point{x1, top}, point{x0, top}},           // top bar
			{point{x0, top}, point{x0, mid}},           // left side of the bowl
			{point{x0, mid}, point{x1, mid}},           // bottom of the bowl
			{point{x1 - 0.03*hw, mid}, point{x0, bot}}, // leg
		},
		r: 0.078 * h,
	}
}

type sample struct {
	top, bottom   color.RGBA // left (or only) gradient
	top2, bottom2 color.RGBA // right gradient, used when split is set
	split         bool       // draw two languages side by side
	glyphs        []shape
	radius        float64 // corner radius, in icon units (0..0.5)
	border        float64 // inner highlight strength
}

// RenderLang draws the tray badge for a language ("en" or "ru").
func RenderLang(lang string, size int) *image.RGBA {
	top, bottom := enTop, enBottom
	g := letterA(0.5, 0.5, 0.66)
	if lang == "ru" {
		top, bottom = ruTop, ruBottom
		g = letterYa(0.5, 0.5, 0.64)
	}
	return render(size, sample{top: top, bottom: bottom, glyphs: []shape{g}, radius: 0.24, border: 0.22})
}

// RenderApp draws the application icon: one badge split into the English blue
// and the Russian red half with a thin divider. Used for the .exe and the
// About box, where both languages have to fit at once.
func RenderApp(size int) *image.RGBA {
	return render(size, sample{
		top:     enTop,
		bottom:  enBottom,
		top2:    ruTop,
		bottom2: ruBottom,
		split:   true,
		radius:  0.22,
		border:  0.12,
	})
}

// render supersamples the badge: at icon sizes (<= 64 px) 4x4 samples per pixel
// give smooth edges, above that 2x2 is plenty and twice as fast.
func render(size int, s sample) *image.RGBA {
	if size < 8 {
		size = 8
	}
	ss := 4
	if size >= 96 {
		ss = 2
	}
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	inv := 1.0 / float64(ss*ss)
	n := float64(size)
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			var r, g, b, a float64
			for sy := 0; sy < ss; sy++ {
				for sx := 0; sx < ss; sx++ {
					px := (float64(x) + (float64(sx)+0.5)/float64(ss)) / n
					py := (float64(y) + (float64(sy)+0.5)/float64(ss)) / n
					cr, cg, cb, ca := s.at(px, py)
					r += cr * ca
					g += cg * ca
					b += cb * ca
					a += ca
				}
			}
			r *= inv
			g *= inv
			b *= inv
			a *= inv
			if a <= 0.0001 {
				continue
			}
			img.SetRGBA(x, y, color.RGBA{
				R: clamp8(r / a),
				G: clamp8(g / a),
				B: clamp8(b / a),
				A: clamp8(a * 255),
			})
		}
	}
	return img
}

// at returns the straight (non-premultiplied) colour at a point in icon units.
func (s sample) at(px, py float64) (float64, float64, float64, float64) {
	d := roundedRectSDF(px, py, 1, 1, s.radius)
	if d > 0 {
		return 0, 0, 0, 0
	}
	t := py
	if t < 0 {
		t = 0
	} else if t > 1 {
		t = 1
	}
	left, right := s.top, s.bottom
	if s.split && px >= 0.5 {
		left, right = s.top2, s.bottom2
	}
	cr := lerp(float64(left.R), float64(right.R), t)
	cg := lerp(float64(left.G), float64(right.G), t)
	cb := lerp(float64(left.B), float64(right.B), t)
	if s.split && math.Abs(px-0.5) < 0.014 {
		cr, cg, cb = 255, 255, 255
	}
	// A thin lighter rim separates the badge from a dark or light taskbar.
	if s.border > 0 && d > -0.045 {
		k := 1 - (-d)/0.045
		if k < 0 {
			k = 0
		}
		cr = lerp(cr, 255, s.border*k)
		cg = lerp(cg, 255, s.border*k)
		cb = lerp(cb, 255, s.border*k)
	}
	for _, g := range s.glyphs {
		if g.inside(px, py) {
			return float64(white.R), float64(white.G), float64(white.B), 1
		}
	}
	return cr, cg, cb, 1
}

// roundedRectSDF is the signed distance to a rounded rectangle: negative
// inside, positive outside.
func roundedRectSDF(px, py, w, h, r float64) float64 {
	qx := math.Abs(px-w/2) - (w/2 - r)
	qy := math.Abs(py-h/2) - (h/2 - r)
	out := math.Hypot(math.Max(qx, 0), math.Max(qy, 0)) + math.Min(math.Max(qx, qy), 0) - r
	return out
}

func lerp(a, b, t float64) float64 { return a + (b-a)*t }

func clamp8(v float64) uint8 {
	if v <= 0 {
		return 0
	}
	if v >= 255 {
		return 255
	}
	return uint8(v + 0.5)
}
