// Package crosslinks resolves [[kind:id]] references that authors put in the
// markdown of products, articles and shop items.
//
// Supported forms:
//
//	[[product:cargo]]               link to a rental product, labelled with its name
//	[[article:trekking-bike-guide]] link to an article, labelled with its title
//	[[shop:dzwonek]]                link to a shop item, labelled with its name
//	[[shop:dzwonek|ten dzwonek]]    same, with a custom label
//
// References are expanded on the rendered HTML, so the markdown stays valid
// and the target's current name is used whenever the label is omitted.
package crosslinks

import (
	"fmt"
	"html"
	"regexp"
	"strings"
)

// Kinds of things that can be linked to.
const (
	KindProduct = "product"
	KindArticle = "article"
	KindShop    = "shop"
)

var tokenPattern = regexp.MustCompile(`\[\[(product|article|shop):([a-z0-9][a-z0-9-]*)(?:\|([^\]]*))?\]\]`)

// Target is something a reference can resolve to.
type Target struct {
	Kind  string
	ID    string
	Title string
	URL   string
}

// Index maps (kind, id) to targets. The zero value is ready to use.
type Index struct {
	targets map[string]Target
}

func key(kind, id string) string { return kind + ":" + id }

// Add registers a target.
func (ix *Index) Add(t Target) {
	if ix.targets == nil {
		ix.targets = make(map[string]Target)
	}
	ix.targets[key(t.Kind, t.ID)] = t
}

// Lookup finds a target.
func (ix *Index) Lookup(kind, id string) (Target, bool) {
	t, ok := ix.targets[key(kind, id)]
	return t, ok
}

// Reference is one [[kind:id]] occurrence.
type Reference struct {
	Kind  string
	ID    string
	Label string // custom label, empty when none was given
	Raw   string
}

// Find lists every reference in s, in order of appearance.
func Find(s string) []Reference {
	matches := tokenPattern.FindAllStringSubmatch(s, -1)
	refs := make([]Reference, 0, len(matches))
	for _, parts := range matches {
		refs = append(refs, Reference{Kind: parts[1], ID: parts[2], Label: strings.TrimSpace(html.UnescapeString(parts[3])), Raw: parts[0]})
	}
	return refs
}

// Expand replaces references in HTML with anchors. References whose target is
// unknown (or filtered out of the index, e.g. hidden from the current visitor)
// degrade to their plain label so no dead link is shown. The unknown ones are
// returned so callers can report them.
func Expand(htmlText string, lookup func(kind, id string) (Target, bool)) (string, []Reference) {
	if !strings.Contains(htmlText, "[[") {
		return htmlText, nil
	}
	var unresolved []Reference
	out := tokenPattern.ReplaceAllStringFunc(htmlText, func(raw string) string {
		parts := tokenPattern.FindStringSubmatch(raw)
		ref := Reference{Kind: parts[1], ID: parts[2], Label: strings.TrimSpace(html.UnescapeString(parts[3])), Raw: raw}
		target, ok := lookup(ref.Kind, ref.ID)
		if !ok {
			unresolved = append(unresolved, ref)
			if ref.Label != "" {
				return html.EscapeString(ref.Label)
			}
			return html.EscapeString(ref.ID)
		}
		label := ref.Label
		if label == "" {
			label = target.Title
		}
		return fmt.Sprintf(`<a href="%s" class="text-emerald-700 underline hover:text-emerald-800" data-link-kind="%s">%s</a>`,
			html.EscapeString(target.URL), html.EscapeString(ref.Kind), html.EscapeString(label))
	})
	return out, unresolved
}

// PlainText replaces references with the label (or the target's title), without
// anchors. Used where the text sits inside another link, such as a card.
func PlainText(htmlText string, lookup func(kind, id string) (Target, bool)) string {
	if !strings.Contains(htmlText, "[[") {
		return htmlText
	}
	return tokenPattern.ReplaceAllStringFunc(htmlText, func(raw string) string {
		parts := tokenPattern.FindStringSubmatch(raw)
		if label := strings.TrimSpace(html.UnescapeString(parts[3])); label != "" {
			return html.EscapeString(label)
		}
		if target, ok := lookup(parts[1], parts[2]); ok {
			return html.EscapeString(target.Title)
		}
		return html.EscapeString(parts[2])
	})
}

// Strip removes references from s, keeping custom labels. Used for plain-text summaries.
func Strip(s string) string {
	return tokenPattern.ReplaceAllStringFunc(s, func(raw string) string {
		parts := tokenPattern.FindStringSubmatch(raw)
		return strings.TrimSpace(html.UnescapeString(parts[3]))
	})
}
