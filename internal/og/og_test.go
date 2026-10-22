package og

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func decode(t *testing.T, data []byte) image.Image {
	t.Helper()
	img, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("not a JPEG: %v", err)
	}
	return img
}

func writePhoto(t *testing.T, name string, w, h int, c color.RGBA) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetRGBA(x, y, c)
		}
	}
	path := filepath.Join(t.TempDir(), name)
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestRenderProducesTheOpenGraphSize(t *testing.T) {
	red := writePhoto(t, "red.png", 300, 200, color.RGBA{R: 220, G: 30, B: 30, A: 255})
	blue := writePhoto(t, "blue.png", 80, 300, color.RGBA{R: 30, G: 30, B: 220, A: 255})

	for name, spec := range map[string]Spec{
		"product":          {Layout: LayoutProduct, Badge: "Do wynajęcia", Title: "Elektryczny Rower Cargo (Longjohn)", Subtitle: "Dla całej rodziny", Price: "150 zł / dobę", CTA: "Wypróbuj mnie!", Photos: []string{red}},
		"product no photo": {Layout: LayoutProduct, Title: "Lampka"},
		"collage":          {Layout: LayoutProduct, Title: "Sklep", Photos: []string{red, blue, red, blue}},
		"cover":            {Layout: LayoutCover, Badge: "Artykuł", Title: "Jak jeden dzień na półwyspie zmienił moje życie", Subtitle: "Siedemdziesiąt kilometrów z dziećmi", CTA: "7 min czytania", Photos: []string{blue}},
		"cover no photo":   {Layout: LayoutCover, Title: "Bez zdjęcia"},
		"missing photo":    {Layout: LayoutProduct, Title: "Brak pliku", Photos: []string{"/nie/ma/takiego.jpg"}},
	} {
		data, err := Render(spec)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if b := decode(t, data).Bounds(); b.Dx() != Width || b.Dy() != Height {
			t.Errorf("%s: size %v", name, b)
		}
		if len(data) > 600_000 {
			t.Errorf("%s: %d bytes is heavy for a preview", name, len(data))
		}
		if dir := os.Getenv("OG_DUMP"); dir != "" {
			_ = os.WriteFile(filepath.Join(dir, strings.ReplaceAll(name, " ", "-")+".jpg"), data, 0o600)
		}
	}
}

func TestRenderSurvivesAbsurdText(t *testing.T) {
	long := strings.Repeat("Superdługiesłowobezspacji", 12)
	for _, spec := range []Spec{
		{Layout: LayoutProduct, Title: long, Subtitle: long, Price: strings.Repeat("9", 40), CTA: long, Badge: long},
		{Layout: LayoutCover, Title: strings.Repeat("słowo ", 200), Subtitle: strings.Repeat("opis ", 200), CTA: long, Badge: long},
		{Layout: LayoutCover, Title: "ąćęłńóśźż ĄĆĘŁŃÓŚŹŻ — „cytat” 70 km"},
	} {
		if _, err := Render(spec); err != nil {
			t.Errorf("%.30q: %v", spec.Title, err)
		}
	}
	if _, err := Render(Spec{}); err == nil {
		t.Error("a picture without a title must be refused")
	}
}

func TestHashChangesWithTheSpecAndThePhoto(t *testing.T) {
	photo := writePhoto(t, "p.png", 10, 10, color.RGBA{A: 255})
	a := Spec{Layout: LayoutProduct, Title: "A", Photos: []string{photo}}
	if a.Hash() != a.Hash() {
		t.Fatal("hash must be stable")
	}
	b := a
	b.Title = "B"
	if a.Hash() == b.Hash() {
		t.Error("a different title must change the hash")
	}
	before := a.Hash()
	if err := os.WriteFile(photo, []byte("changed and longer"), 0o600); err != nil {
		t.Fatal(err)
	}
	if a.Hash() == before {
		t.Error("a changed photo must change the hash")
	}
}

func TestWrapBreaksLongWordsAndFitShrinks(t *testing.T) {
	f := face(true, 40)
	lines := wrap(f, strings.Repeat("W", 80), 300)
	if len(lines) < 2 {
		t.Fatalf("a long word must be cut, got %v", lines)
	}
	for _, l := range lines {
		if textWidth(f, l) > 300 {
			t.Errorf("%q is %dpx wide", l, textWidth(f, l))
		}
	}
	_, _, size := fit("krótki", true, 500, 1, 60, 30)
	if size != 60 {
		t.Errorf("short text keeps the start size, got %v", size)
	}
	got, _, size := fit(strings.Repeat("dłuższy tekst ", 30), true, 400, 2, 60, 30)
	if size != 30 || len(got) != 2 || !strings.HasSuffix(got[1], "…") {
		t.Errorf("overflowing text is cut with an ellipsis: %v (size %v)", got, size)
	}
}
