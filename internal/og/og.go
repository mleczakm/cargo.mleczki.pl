// Package og draws the preview images that social networks and search engines
// show when a page is shared: a 1200×630 picture with the product or the
// article, its title and what it costs, so every page has a thumbnail that is
// worth clicking.
package og

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	_ "image/png" // decoders for photos
	"os"
	"strings"
	"sync"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
	_ "golang.org/x/image/webp" // decoder for photos
)

// Size of the generated image.
const (
	Width  = 1200
	Height = 630
)

// Layout chooses how the picture is composed.
type Layout string

const (
	// LayoutProduct puts the photo on a white card next to the text: for things you can buy or rent.
	LayoutProduct Layout = "product"
	// LayoutCover lets the photo fill the whole picture: for articles.
	LayoutCover Layout = "cover"
)

// Spec describes one preview image.
type Spec struct {
	Layout   Layout   `json:"layout"`
	Badge    string   `json:"badge,omitempty"`    // small label above the title: "Do wynajęcia", "Sklep", "Artykuł"
	Title    string   `json:"title"`              //
	Subtitle string   `json:"subtitle,omitempty"` // a sentence or two
	Price    string   `json:"price,omitempty"`    // "150 zł / dobę"
	CTA      string   `json:"cta,omitempty"`      // "Wypróbuj mnie!"
	Photos   []string `json:"photos,omitempty"`   // files on disk; several are laid out as a grid on a product card
	Site     string   `json:"site,omitempty"`     // shown in the corner; defaults to cargo.mleczki.pl
}

// Hash identifies the picture a spec produces, including the photos' versions,
// so it can serve as a cache key, an ETag and a cache-busting query.
func (s Spec) Hash() string {
	h := sha256.New()
	raw, _ := json.Marshal(s)
	h.Write(raw)
	for _, p := range s.Photos {
		if info, err := os.Stat(p); err == nil {
			fmt.Fprintf(h, "|%s|%d|%d", p, info.Size(), info.ModTime().UnixNano())
		}
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}

var (
	boldFont, regularFont *opentype.Font
	faceMu                sync.Mutex
	faces                 = map[string]font.Face{}
)

func init() {
	var err error
	if boldFont, err = opentype.Parse(gobold.TTF); err != nil {
		panic(err)
	}
	if regularFont, err = opentype.Parse(goregular.TTF); err != nil {
		panic(err)
	}
}

func face(bold bool, size float64) font.Face {
	key := fmt.Sprintf("%t/%.1f", bold, size)
	faceMu.Lock()
	defer faceMu.Unlock()
	if f, ok := faces[key]; ok {
		return f
	}
	src := regularFont
	if bold {
		src = boldFont
	}
	f, err := opentype.NewFace(src, &opentype.FaceOptions{Size: size, DPI: 72, Hinting: font.HintingFull})
	if err != nil {
		panic(err)
	}
	faces[key] = f
	return f
}

func textWidth(f font.Face, s string) int { return font.MeasureString(f, s).Ceil() }

// wrap breaks text into lines no wider than width. Words wider than a line are cut.
func wrap(f font.Face, text string, width int) []string {
	var lines []string
	line := ""
	flush := func() {
		if line != "" {
			lines = append(lines, line)
			line = ""
		}
	}
	for _, word := range strings.Fields(text) {
		for textWidth(f, word) > width { // a single word longer than the line
			cut := len([]rune(word))
			for cut > 1 && textWidth(f, string([]rune(word)[:cut])) > width {
				cut--
			}
			flush()
			lines = append(lines, string([]rune(word)[:cut]))
			word = string([]rune(word)[cut:])
		}
		candidate := word
		if line != "" {
			candidate = line + " " + word
		}
		if textWidth(f, candidate) <= width {
			line = candidate
			continue
		}
		flush()
		line = word
	}
	flush()
	return lines
}

// fit finds the largest size at which text fits in maxLines lines; at the
// smallest size it cuts the text with an ellipsis instead.
func fit(text string, bold bool, width, maxLines int, start, minimum float64) ([]string, font.Face, float64) {
	for size := start; size >= minimum; size -= 2 {
		f := face(bold, size)
		if lines := wrap(f, text, width); len(lines) <= maxLines {
			return lines, f, size
		}
	}
	f := face(bold, minimum)
	lines := wrap(f, text, width)
	if len(lines) > maxLines {
		lines = lines[:maxLines]
		last := []rune(lines[maxLines-1])
		for len(last) > 1 && textWidth(f, string(last)+"…") > width {
			last = last[:len(last)-1]
		}
		lines[maxLines-1] = strings.TrimSpace(string(last)) + "…"
	}
	return lines, f, minimum
}

func drawText(img *image.RGBA, f font.Face, x, baseline int, s string, c color.RGBA) {
	d := &font.Drawer{Dst: img, Src: image.NewUniform(c), Face: f, Dot: fixed.P(x, baseline)}
	d.DrawString(s)
}

// drawLines paints lines starting at top and returns the y below the last one.
func drawLines(img *image.RGBA, f font.Face, x, top int, lines []string, lineHeight float64, c color.RGBA) int {
	ascent := f.Metrics().Ascent.Ceil()
	y := top
	for _, line := range lines {
		drawText(img, f, x, y+ascent, line, c)
		y += int(lineHeight)
	}
	return y
}

func pill(img *image.RGBA, x, y int, label string, f font.Face, bg, fg color.RGBA, padX, height int) int {
	w := textWidth(f, label) + 2*padX
	r := image.Rect(x, y, x+w, y+height)
	fillRounded(img, r, float64(height)/2, bg)
	metrics := f.Metrics()
	baseline := y + (height+metrics.Ascent.Ceil()-metrics.Descent.Ceil())/2
	drawText(img, f, x+padX, baseline, label, fg)
	return w
}

func loadPhoto(path string) (image.Image, error) {
	file, err := os.Open(path) // #nosec G304 - paths come from the catalog, not from the request
	if err != nil {
		return nil, err
	}
	defer file.Close()
	img, _, err := image.Decode(file)
	return img, err
}

func loadPhotos(paths []string) []image.Image {
	var out []image.Image
	for _, p := range paths {
		if img, err := loadPhoto(p); err == nil {
			out = append(out, img)
		}
	}
	return out
}

var (
	deep    = rgb(0x022c22)
	forest  = rgb(0x065f46)
	teal    = rgb(0x0f766e)
	mint    = rgb(0x6ee7b7)
	mintDim = rgb(0xa7f3d0)
	amber   = rgb(0xfbbf24)
	ink     = rgb(0x0f172a)
	white   = rgb(0xffffff)
)

// Render draws the image and returns it as a JPEG.
func Render(spec Spec) ([]byte, error) {
	if strings.TrimSpace(spec.Title) == "" {
		return nil, errors.New("og: a title is required")
	}
	if spec.Site == "" {
		spec.Site = "cargo.mleczki.pl"
	}
	img := image.NewRGBA(image.Rect(0, 0, Width, Height))
	photos := loadPhotos(spec.Photos)

	if spec.Layout == LayoutCover {
		renderCover(img, spec, photos)
	} else {
		renderProduct(img, spec, photos)
	}

	var out bytes.Buffer
	if err := jpeg.Encode(&out, img, &jpeg.Options{Quality: 90}); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func background(img *image.RGBA) {
	fillGradient(img, img.Bounds(), deep, forest)
	glow(img, 1050, 80, 520, mint, 0.22)
	glow(img, 80, 640, 480, teal, 0.45)
}

func brand(img *image.RGBA, spec Spec, x, y int, c color.RGBA) {
	drawLogo(img, float64(x), float64(y), 44, c)
	drawText(img, face(true, 26), x+58, y+33, spec.Site, c)
}

func renderProduct(img *image.RGBA, spec Spec, photos []image.Image) {
	background(img)

	// The photo card on the right.
	card := image.Rect(668, 56, 1148, 574)
	shadow(img, card, 38, 26, withAlpha(rgb(0x000000), 0.45))
	fillRounded(img, card, 38, rgb(0xf8fafc))
	switch {
	case len(photos) >= 2:
		cells := []image.Rectangle{
			image.Rect(card.Min.X+14, card.Min.Y+14, card.Max.X-14, card.Min.Y+14+(card.Dy()-42)/2),
		}
		half := (card.Dx() - 42) / 2
		top := cells[0].Max.Y + 14
		cells = append(cells,
			image.Rect(card.Min.X+14, top, card.Min.X+14+half, card.Max.Y-14),
			image.Rect(card.Min.X+28+half, top, card.Max.X-14, card.Max.Y-14))
		for i, cell := range cells {
			if i < len(photos) {
				drawPhoto(img, cell, 26, photos[i], fitCover, 0)
			}
		}
	case len(photos) == 1:
		drawPhoto(img, card, 38, photos[0], fitContain, 30)
	default:
		fillRounded(img, card.Inset(14), 28, rgb(0xecfdf5))
		drawLogo(img, float64(card.Min.X)+90, float64(card.Min.Y)+130, 300, forest)
	}

	// Text on the left.
	const left, textWidthPx = 64, 560
	titleLines, titleFace, titleSize := fit(spec.Title, true, textWidthPx, 3, 68, 40)
	var subLines []string
	var subFace font.Face
	var subSize float64
	if spec.Subtitle != "" {
		subLines, subFace, subSize = fit(spec.Subtitle, false, textWidthPx, 3, 28, 22)
	}

	// Centre the text between the top edge and the price row when it is short.
	block := int(float64(len(titleLines)) * titleSize * 1.12)
	if spec.Badge != "" {
		block += 44 + 26
	}
	if len(subLines) > 0 {
		block += 14 + int(float64(len(subLines))*subSize*1.35)
	}
	bottom := 566
	rowTop := bottom - 44 - 28 - 62 - 24
	y := 60
	if free := rowTop - 60 - block; free > 0 {
		y += free / 2
	}

	if spec.Badge != "" {
		pill(img, left, y, strings.ToUpper(spec.Badge), face(true, 22), amber, ink, 22, 44)
		y += 44 + 26
	}
	y = drawLines(img, titleFace, left, y, titleLines, titleSize*1.12, white)
	if len(subLines) > 0 {
		drawLines(img, subFace, left, y+14, subLines, subSize*1.35, mintDim)
	}

	// Price and call to action pinned to the bottom.
	brand(img, spec, left, bottom-44, mint)
	rowY := bottom - 44 - 28 - 62
	x := left
	if spec.Price != "" {
		pf := face(true, 46)
		drawText(img, pf, x, rowY+46, spec.Price, white)
		x += textWidth(pf, spec.Price) + 28
	}
	if spec.CTA != "" {
		pill(img, x, rowY-2, spec.CTA, face(true, 26), amber, ink, 26, 62)
	}
}

func renderCover(img *image.RGBA, spec Spec, photos []image.Image) {
	if len(photos) > 0 {
		drawPhoto(img, img.Bounds(), 0, photos[0], fitCover, 0)
	} else {
		background(img)
		drawLogo(img, 820, 90, 330, withAlpha(mint, 0.16))
	}
	// Darken the bottom so the title is readable on any photo.
	for y := 0; y < Height; y++ {
		t := clamp01(float64(y-Height/5) / float64(Height*4/5))
		a := 0.88 * t * t
		for x := 0; x < Width; x++ {
			blend(img, x, y, rgb(0x011a14), a)
		}
	}
	for y := 0; y < 170; y++ { // and a little at the top for the brand
		a := 0.5 * (1 - float64(y)/170)
		for x := 0; x < Width; x++ {
			blend(img, x, y, rgb(0x011a14), a)
		}
	}

	const left, textWidthPx = 64, 1000
	brand(img, spec, left, 44, white)
	if spec.Badge != "" {
		f := face(true, 22)
		label := strings.ToUpper(spec.Badge)
		pill(img, Width-64-(textWidth(f, label)+44), 50, label, f, amber, ink, 22, 44)
	}

	bottom := 566
	var subLines []string
	var subFace font.Face
	var subSize float64
	if spec.Subtitle != "" {
		subLines, subFace, subSize = fit(spec.Subtitle, false, 900, 2, 30, 24)
	}
	titleLines, titleFace, titleSize := fit(spec.Title, true, textWidthPx, 3, 78, 46)

	total := int(float64(len(titleLines)) * titleSize * 1.1)
	if len(subLines) > 0 {
		total += 18 + int(float64(len(subLines))*subSize*1.35)
	}
	if spec.CTA != "" || spec.Price != "" {
		total += 22 + 54
	}
	y := bottom - total
	y = drawLines(img, titleFace, left, y, titleLines, titleSize*1.1, white)
	if len(subLines) > 0 {
		y += 18
		y = drawLines(img, subFace, left, y, subLines, subSize*1.35, rgb(0xd1fae5))
	}
	x := left
	if spec.Price != "" || spec.CTA != "" {
		y += 22
		if spec.Price != "" {
			pf := face(true, 34)
			drawText(img, pf, x, y+40, spec.Price, white)
			x += textWidth(pf, spec.Price) + 24
		}
		if spec.CTA != "" {
			pill(img, x, y, spec.CTA, face(true, 24), amber, ink, 24, 54)
		}
	}
}
