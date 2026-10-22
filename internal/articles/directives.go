package articles

import (
	"bufio"
	"fmt"
	"html"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/yuin/goldmark"
)

// Articles are Markdown with a few block directives for storytelling layouts.
// A directive opens with `::: name key=value` and closes with `:::`:
//
//	::: scene image=hel.jpg focus="50% 30%" tone=dark align=left
//	First card of text.
//
//	Still the first card.
//	+++
//	Second card, scrolls over the same photo.
//	:::
//
// scene   a photo that stays put while the text cards scroll over it
// figure  a full-width photo with a caption (the body is the caption)
// gallery a row of photos: `images=a.jpg,b.jpg,c.jpg`
// stats   big numbers, one `value | label` per line
// note    a highlighted aside
//
// Everything else is ordinary Markdown. Raw HTML stays disabled: directives are
// the only way to add layout, and every attribute is escaped or validated.

// ImageDir tells where article photos live and how they are addressed.
type ImageDir struct {
	Files string // directory on disk, used to notice photos that are missing
	URL   string // URL prefix, e.g. /data/images/articles/
}

type directive struct {
	name  string
	attrs map[string]string
	body  string
}

var (
	openFence  = regexp.MustCompile(`^:::\s*([a-z]+)\s*(.*)$`)
	closeFence = regexp.MustCompile(`^:::\s*$`)
	attrRe     = regexp.MustCompile(`([a-zA-Z]+)=("[^"]*"|\S+)`)
	focusRe    = regexp.MustCompile(`^[0-9]{1,3}%? [0-9]{1,3}%?$|^(left|right|center|top|bottom)( (left|right|center|top|bottom))?$`)
)

// segment is either a stretch of plain Markdown or a directive.
type segment struct {
	markdown  string
	directive *directive
}

// splitSegments cuts the body into Markdown and directives. An unclosed
// directive runs to the end of the document and is reported as an error so the
// author notices instead of losing the rest of the page.
func splitSegments(body string) ([]segment, error) {
	var (
		segments []segment
		plain    strings.Builder
		current  *directive
		inner    strings.Builder
		fence    string // ``` fence we are inside of, where `:::` has no meaning
	)
	flushPlain := func() {
		if strings.TrimSpace(plain.String()) != "" {
			segments = append(segments, segment{markdown: plain.String()})
		}
		plain.Reset()
	}

	scanner := bufio.NewScanner(strings.NewReader(body))
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		if current == nil {
			if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
				marker := trimmed[:3]
				if fence == "" {
					fence = marker
				} else if fence == marker {
					fence = ""
				}
			}
			if fence == "" {
				if m := openFence.FindStringSubmatch(trimmed); m != nil {
					flushPlain()
					current = &directive{name: m[1], attrs: parseAttrs(m[2])}
					inner.Reset()
					continue
				}
			}
			plain.WriteString(line + "\n")
			continue
		}

		if closeFence.MatchString(trimmed) {
			current.body = inner.String()
			segments = append(segments, segment{directive: current})
			current = nil
			continue
		}
		inner.WriteString(line + "\n")
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if current != nil {
		return nil, fmt.Errorf("directive %q is not closed with :::", current.name)
	}
	flushPlain()
	return segments, nil
}

func parseAttrs(raw string) map[string]string {
	attrs := map[string]string{}
	for _, m := range attrRe.FindAllStringSubmatch(raw, -1) {
		attrs[strings.ToLower(m[1])] = strings.Trim(m[2], `"`)
	}
	return attrs
}

// renderer turns segments into HTML.
type renderer struct {
	md     goldmark.Markdown
	images ImageDir
}

// RenderBody renders an article body: Markdown plus directives. Photos that do
// not exist yet are reported through missing, so the admin can see what to add.
func RenderBody(md goldmark.Markdown, body string, images ImageDir) (rendered string, missing []string, err error) {
	segments, err := splitSegments(body)
	if err != nil {
		return "", nil, err
	}
	r := &renderer{md: md, images: images}
	var out strings.Builder
	for _, seg := range segments {
		if seg.directive == nil {
			chunk, err := r.markdown(seg.markdown)
			if err != nil {
				return "", nil, err
			}
			out.WriteString(chunk)
			continue
		}
		chunk, gone, err := r.directive(seg.directive)
		if err != nil {
			return "", nil, err
		}
		missing = append(missing, gone...)
		out.WriteString(chunk)
	}
	return out.String(), missing, nil
}

func (r *renderer) markdown(src string) (string, error) {
	var out strings.Builder
	if err := r.md.Convert([]byte(src), &out); err != nil {
		return "", fmt.Errorf("convert markdown: %w", err)
	}
	return out.String(), nil
}

func (r *renderer) directive(d *directive) (string, []string, error) {
	switch d.name {
	case "scene":
		return r.scene(d)
	case "figure":
		return r.figure(d)
	case "gallery":
		return r.gallery(d)
	case "stats":
		return r.stats(d), nil, nil
	case "note":
		return r.note(d)
	default:
		return "", nil, fmt.Errorf("unknown directive %q (use scene, figure, gallery, stats or note)", d.name)
	}
}

// imageURL resolves an image name to a URL and reports whether the file exists.
func (r *renderer) imageURL(name string) (string, bool) {
	name = strings.TrimSpace(name)
	switch {
	case name == "":
		return "", false
	case strings.HasPrefix(name, "http://") || strings.HasPrefix(name, "https://"):
		if u, err := url.Parse(name); err != nil || u.Host == "" {
			return "", false
		}
		return name, true // remote photos are trusted to exist
	case strings.HasPrefix(name, "/"):
		return name, true
	}
	name = filepath.ToSlash(filepath.Clean(name))
	if strings.HasPrefix(name, "..") {
		return "", false
	}
	exists := false
	if r.images.Files != "" {
		_, err := os.Stat(filepath.Join(r.images.Files, filepath.FromSlash(name)))
		exists = err == nil
	}
	return r.images.URL + name, exists
}

func esc(s string) string { return html.EscapeString(s) }

func choice(value string, allowed []string, fallback string) string {
	for _, a := range allowed {
		if value == a {
			return value
		}
	}
	return fallback
}

func focus(value string) string {
	if focusRe.MatchString(value) {
		return value
	}
	return "50% 50%"
}

// caption renders inline Markdown of a caption without the surrounding paragraph.
func (r *renderer) caption(src string) (string, error) {
	src = strings.TrimSpace(src)
	if src == "" {
		return "", nil
	}
	rendered, err := r.markdown(src)
	if err != nil {
		return "", err
	}
	rendered = strings.TrimSpace(rendered)
	rendered = strings.TrimPrefix(rendered, "<p>")
	return strings.TrimSuffix(rendered, "</p>"), nil
}

func (r *renderer) photo(class, name, alt string) (string, bool) {
	src, exists := r.imageURL(name)
	if src == "" || !exists {
		return fmt.Sprintf(`<div class="%s %s--placeholder" role="img" aria-label="%s"></div>`, class, class, esc(alt)), src != ""
	}
	return fmt.Sprintf(`<img class="%s" src="%s" alt="%s" loading="lazy" decoding="async">`, class, esc(src), esc(alt)), false
}

func (r *renderer) scene(d *directive) (string, []string, error) {
	tone := choice(d.attrs["tone"], []string{"dark", "light"}, "dark")
	align := choice(d.attrs["align"], []string{"left", "center", "right"}, "left")
	alt := d.attrs["alt"]

	bg, absent := r.photo("scene__bg", d.attrs["image"], alt)
	var missing []string
	if absent {
		missing = append(missing, d.attrs["image"])
	}

	var cards strings.Builder
	for _, part := range strings.Split(d.body, "\n+++\n") {
		if strings.TrimSpace(part) == "" {
			continue
		}
		rendered, err := r.markdown(part)
		if err != nil {
			return "", nil, err
		}
		fmt.Fprintf(&cards, `<div class="scene__card">%s</div>`, rendered)
	}

	credit := ""
	if c, err := r.caption(d.attrs["credit"]); err != nil {
		return "", nil, err
	} else if c != "" {
		credit = `<p class="scene__credit">` + c + `</p>`
	}
	return fmt.Sprintf(`<section class="scene scene--%s scene--%s" style="--focus:%s">%s<div class="scene__cards">%s</div>%s</section>`+"\n",
		tone, align, focus(d.attrs["focus"]), bg, cards.String(), credit), missing, nil
}

func (r *renderer) figure(d *directive) (string, []string, error) {
	size := choice(d.attrs["size"], []string{"wide", "tall", "inline"}, "wide")
	capSrc := d.attrs["caption"]
	if strings.TrimSpace(d.body) != "" {
		capSrc = d.body
	}
	img, absent := r.photo("figure__img", d.attrs["image"], firstNonEmpty(d.attrs["alt"], stripMarkdown(capSrc)))
	var missing []string
	if absent {
		missing = append(missing, d.attrs["image"])
	}
	caption, err := r.caption(capSrc)
	if err != nil {
		return "", nil, err
	}
	figcaption := ""
	if caption != "" {
		figcaption = "<figcaption>" + caption + "</figcaption>"
	}
	return fmt.Sprintf(`<figure class="figure figure--%s" style="--focus:%s">%s%s</figure>`+"\n", size, focus(d.attrs["focus"]), img, figcaption), missing, nil
}

func (r *renderer) gallery(d *directive) (string, []string, error) {
	var items strings.Builder
	var missing []string
	count := 0
	for _, name := range strings.Split(d.attrs["images"], ",") {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		img, absent := r.photo("gallery__img", name, "")
		if absent {
			missing = append(missing, name)
		}
		items.WriteString("<figure>" + img + "</figure>")
		count++
	}
	if count == 0 {
		return "", nil, fmt.Errorf("gallery needs images=a.jpg,b.jpg")
	}
	caption, err := r.caption(firstNonEmpty(d.body, d.attrs["caption"]))
	if err != nil {
		return "", nil, err
	}
	figcaption := ""
	if caption != "" {
		figcaption = `<p class="gallery__caption">` + caption + "</p>"
	}
	return fmt.Sprintf(`<div class="gallery gallery--%d">%s</div>%s`+"\n", min(count, 4), items.String(), figcaption), missing, nil
}

func (r *renderer) stats(d *directive) string {
	var items strings.Builder
	for _, line := range strings.Split(d.body, "\n") {
		value, label, ok := strings.Cut(line, "|")
		value, label = strings.TrimSpace(value), strings.TrimSpace(label)
		if !ok || value == "" {
			continue
		}
		fmt.Fprintf(&items, `<li><strong>%s</strong><span>%s</span></li>`, esc(value), esc(label))
	}
	return `<ul class="stats">` + items.String() + "</ul>\n"
}

func (r *renderer) note(d *directive) (string, []string, error) {
	rendered, err := r.markdown(d.body)
	if err != nil {
		return "", nil, err
	}
	title := ""
	if t := strings.TrimSpace(d.attrs["title"]); t != "" {
		title = `<p class="note__title">` + esc(t) + "</p>"
	}
	return `<aside class="note">` + title + rendered + "</aside>\n", nil, nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

var markdownMarks = regexp.MustCompile("[*_`\\[\\]]|\\(https?://[^)]*\\)")

func stripMarkdown(s string) string {
	return strings.TrimSpace(markdownMarks.ReplaceAllString(s, ""))
}
