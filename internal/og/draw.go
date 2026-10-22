package og

import (
	"image"
	"image/color"
	"image/draw"
	"math"

	xdraw "golang.org/x/image/draw"
)

func clamp01(v float64) float64 { return math.Max(0, math.Min(1, v)) }

func rgb(hex uint32) color.RGBA {
	return color.RGBA{R: uint8(hex >> 16), G: uint8(hex >> 8), B: uint8(hex), A: 255}
}

func withAlpha(c color.RGBA, a float64) color.RGBA {
	return color.RGBA{R: c.R, G: c.G, B: c.B, A: uint8(clamp01(a) * 255)}
}

// blend composites c with coverage cov (0..1) over the pixel at x, y.
func blend(img *image.RGBA, x, y int, c color.RGBA, cov float64) {
	if cov <= 0 || !image.Pt(x, y).In(img.Rect) {
		return
	}
	a := cov * float64(c.A) / 255
	if a <= 0 {
		return
	}
	i := img.PixOffset(x, y)
	p := img.Pix[i : i+4 : i+4]
	inv := 1 - a
	p[0] = uint8(float64(c.R)*a + float64(p[0])*inv)
	p[1] = uint8(float64(c.G)*a + float64(p[1])*inv)
	p[2] = uint8(float64(c.B)*a + float64(p[2])*inv)
	p[3] = 255
}

// fillGradient paints a diagonal gradient from c0 (top left) to c1 (bottom right).
func fillGradient(img *image.RGBA, r image.Rectangle, c0, c1 color.RGBA) {
	w, h := float64(r.Dx()), float64(r.Dy())
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			t := clamp01((float64(x-r.Min.X)/w*0.8 + float64(y-r.Min.Y)/h*0.6) / 1.4)
			i := img.PixOffset(x, y)
			img.Pix[i+0] = uint8(float64(c0.R)*(1-t) + float64(c1.R)*t)
			img.Pix[i+1] = uint8(float64(c0.G)*(1-t) + float64(c1.G)*t)
			img.Pix[i+2] = uint8(float64(c0.B)*(1-t) + float64(c1.B)*t)
			img.Pix[i+3] = 255
		}
	}
}

// glow adds a soft radial highlight.
func glow(img *image.RGBA, cx, cy, radius float64, c color.RGBA, strength float64) {
	b := img.Bounds()
	for y := max(b.Min.Y, int(cy-radius)); y < min(b.Max.Y, int(cy+radius)+1); y++ {
		for x := max(b.Min.X, int(cx-radius)); x < min(b.Max.X, int(cx+radius)+1); x++ {
			d := math.Hypot(float64(x)-cx, float64(y)-cy) / radius
			if d < 1 {
				blend(img, x, y, c, strength*(1-d)*(1-d))
			}
		}
	}
}

// roundedCoverage is the anti-aliased coverage of a rounded rectangle at pixel (x, y).
func roundedCoverage(r image.Rectangle, radius float64, x, y int) float64 {
	cx, cy := float64(r.Min.X+r.Max.X)/2, float64(r.Min.Y+r.Max.Y)/2
	hw, hh := float64(r.Dx())/2, float64(r.Dy())/2
	px, py := math.Abs(float64(x)+0.5-cx)-(hw-radius), math.Abs(float64(y)+0.5-cy)-(hh-radius)
	d := math.Hypot(math.Max(px, 0), math.Max(py, 0)) + math.Min(math.Max(px, py), 0) - radius
	return clamp01(0.5 - d)
}

func fillRounded(img *image.RGBA, r image.Rectangle, radius float64, c color.RGBA) {
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			blend(img, x, y, c, roundedCoverage(r, radius, x, y))
		}
	}
}

// shadow paints a soft drop shadow under a rounded rectangle.
func shadow(img *image.RGBA, r image.Rectangle, radius float64, spread int, c color.RGBA) {
	for i := spread; i > 0; i-- {
		grown := r.Inset(-i).Add(image.Pt(0, spread/2))
		fillRounded(img, grown, radius+float64(i), withAlpha(c, float64(c.A)/255*0.9/float64(spread)))
	}
}

type fitMode int

const (
	fitCover fitMode = iota
	fitContain
)

// drawPhoto scales src into r (cropping to fill, or fitting whole) with rounded corners.
func drawPhoto(dst *image.RGBA, r image.Rectangle, radius float64, src image.Image, mode fitMode, pad int) {
	inner := r.Inset(pad)
	sb := src.Bounds()
	if sb.Dx() == 0 || sb.Dy() == 0 || inner.Dx() <= 0 || inner.Dy() <= 0 {
		return
	}
	scaleX, scaleY := float64(inner.Dx())/float64(sb.Dx()), float64(inner.Dy())/float64(sb.Dy())

	switch mode {
	case fitCover:
		scale := math.Max(scaleX, scaleY)
		w, h := float64(sb.Dx())*scale, float64(sb.Dy())*scale
		scaled := image.NewRGBA(image.Rect(0, 0, int(math.Ceil(w)), int(math.Ceil(h))))
		xdraw.CatmullRom.Scale(scaled, scaled.Bounds(), src, sb, xdraw.Src, nil)
		offX, offY := (scaled.Bounds().Dx()-inner.Dx())/2, (scaled.Bounds().Dy()-inner.Dy())/2
		for y := inner.Min.Y; y < inner.Max.Y; y++ {
			for x := inner.Min.X; x < inner.Max.X; x++ {
				cov := roundedCoverage(r, radius, x, y)
				c := scaled.RGBAAt(x-inner.Min.X+offX, y-inner.Min.Y+offY)
				blend(dst, x, y, color.RGBA{R: c.R, G: c.G, B: c.B, A: 255}, cov)
			}
		}
	default:
		scale := math.Min(scaleX, scaleY)
		w, h := int(float64(sb.Dx())*scale), int(float64(sb.Dy())*scale)
		x0, y0 := inner.Min.X+(inner.Dx()-w)/2, inner.Min.Y+(inner.Dy()-h)/2
		target := image.Rect(x0, y0, x0+w, y0+h)
		scaled := image.NewRGBA(image.Rect(0, 0, w, h))
		xdraw.CatmullRom.Scale(scaled, scaled.Bounds(), src, sb, xdraw.Src, nil)
		draw.Draw(dst, target, scaled, image.Point{}, draw.Over)
	}
}

// paintFunc receives the coverage (0..1) of a pixel.
type paintFunc func(x, y int, cov float64)

func onImage(img *image.RGBA, c color.RGBA) paintFunc {
	return func(x, y int, cov float64) { blend(img, x, y, c, cov) }
}

// strokeLine draws an anti-aliased line with round caps.
func strokeLine(paint paintFunc, x0, y0, x1, y1, width float64) {
	minX, maxX := math.Min(x0, x1)-width, math.Max(x0, x1)+width
	minY, maxY := math.Min(y0, y1)-width, math.Max(y0, y1)+width
	dx, dy := x1-x0, y1-y0
	l2 := dx*dx + dy*dy
	for y := int(minY); y <= int(maxY); y++ {
		for x := int(minX); x <= int(maxX); x++ {
			px, py := float64(x)+0.5-x0, float64(y)+0.5-y0
			t := 0.0
			if l2 > 0 {
				t = clamp01((px*dx + py*dy) / l2)
			}
			d := math.Hypot(px-t*dx, py-t*dy) - width/2
			paint(x, y, clamp01(0.5-d))
		}
	}
}

// strokeCircle draws an anti-aliased ring.
func strokeCircle(paint paintFunc, cx, cy, r, width float64) {
	for y := int(cy - r - width); y <= int(cy+r+width); y++ {
		for x := int(cx - r - width); x <= int(cx+r+width); x++ {
			d := math.Abs(math.Hypot(float64(x)+0.5-cx, float64(y)+0.5-cy)-r) - width/2
			paint(x, y, clamp01(0.5-d))
		}
	}
}

// drawLogo paints the site's cargo bike mark (the same drawing as the header)
// into a size×size box. Strokes overlap without doubling up, so c may be translucent.
func drawLogo(img *image.RGBA, x, y, size float64, c color.RGBA) {
	f := size / 100
	w := 5 * f

	// Collect the coverage first so translucent logos do not get bright joints.
	side := int(size) + 2
	cover := make([]float64, side*side)
	ox, oy := int(x), int(y)
	paint := func(px, py int, cov float64) {
		px, py = px-ox, py-oy
		if px >= 0 && py >= 0 && px < side && py < side && cov > cover[py*side+px] {
			cover[py*side+px] = cov
		}
	}
	pt := func(px, py float64) (float64, float64) {
		return x - float64(ox) + px*f + float64(ox), y - float64(oy) + py*f + float64(oy)
	}
	circle := func(cx, cy, r float64) {
		ax, ay := pt(cx, cy)
		strokeCircle(paint, ax, ay, r*f, w)
	}
	line := func(x0, y0, x1, y1, lw float64) {
		ax, ay := pt(x0, y0)
		bx, by := pt(x1, y1)
		strokeLine(paint, ax, ay, bx, by, lw*f)
	}
	circle(18, 75, 14)
	circle(86, 79, 10)
	line(18, 75, 86, 79, 5)
	line(38, 75, 26, 40, 5)
	line(18, 75, 26, 40, 5)
	line(18, 40, 34, 40, 6)
	line(49, 76, 46, 32, 5)
	line(38, 32, 52, 32, 6)
	line(47, 40, 76, 52, 5)
	line(76, 52, 79, 78, 5)
	line(79, 78, 49, 76, 5)

	for py := 0; py < side; py++ {
		for px := 0; px < side; px++ {
			blend(img, ox+px, oy+py, c, cover[py*side+px])
		}
	}
}
