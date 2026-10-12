package crosslinks_test

import (
	"strings"
	"testing"

	"cargo.mleczki.pl/internal/crosslinks"
)

func testIndex() *crosslinks.Index {
	index := &crosslinks.Index{}
	index.Add(crosslinks.Target{Kind: crosslinks.KindProduct, ID: "cargo", Title: "Rower cargo", URL: "/product/cargo"})
	index.Add(crosslinks.Target{Kind: crosslinks.KindArticle, ID: "guide", Title: "Jak wybrać <rower>", URL: "/article/guide"})
	index.Add(crosslinks.Target{Kind: crosslinks.KindShop, ID: "dzwonek", Title: "Dzwonek", URL: "/shop/dzwonek"})
	return index
}

func TestExpandResolvesAllKinds(t *testing.T) {
	ix := testIndex()
	out, unresolved := crosslinks.Expand(
		`<p>Zobacz [[product:cargo]], [[article:guide]] i [[shop:dzwonek|ten dzwonek]].</p>`, ix.Lookup)

	if len(unresolved) != 0 {
		t.Fatalf("unexpected unresolved: %+v", unresolved)
	}
	for _, want := range []string{
		`<a href="/product/cargo" class="text-emerald-700 underline hover:text-emerald-800" data-link-kind="product">Rower cargo</a>`,
		`href="/article/guide"`,
		`Jak wybrać &lt;rower&gt;</a>`, // titles are escaped
		`href="/shop/dzwonek"`,
		`>ten dzwonek</a>`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestExpandDegradesUnknownTargetsToPlainText(t *testing.T) {
	out, unresolved := crosslinks.Expand(`<p>[[shop:gone]] i [[product:gone|stary rower]]</p>`, testIndex().Lookup)

	if out != `<p>gone i stary rower</p>` {
		t.Errorf("out = %q", out)
	}
	if len(unresolved) != 2 || unresolved[0].Kind != crosslinks.KindShop || unresolved[1].Label != "stary rower" {
		t.Errorf("unresolved = %+v", unresolved)
	}
}

func TestExpandEscapesCustomLabels(t *testing.T) {
	// goldmark has already HTML-escaped the text node by the time crosslinks.Expand runs.
	out, _ := crosslinks.Expand(`<p>[[shop:dzwonek|&lt;b&gt;x&lt;/b&gt; &amp; y]]</p>`, testIndex().Lookup)
	if strings.Contains(out, "<b>") || !strings.Contains(out, "&lt;b&gt;x&lt;/b&gt; &amp; y</a>") {
		t.Errorf("label must stay escaped: %s", out)
	}
}

func TestExpandLeavesOtherBracketsAlone(t *testing.T) {
	in := `<p>[[unknown:x]] [[shop:UPPER]] [single] [[shop:]]</p>`
	out, unresolved := crosslinks.Expand(in, testIndex().Lookup)
	if out != in || len(unresolved) != 0 {
		t.Errorf("only well-formed references may be touched, got %q (%v)", out, unresolved)
	}
}

func TestFindAndStrip(t *testing.T) {
	refs := crosslinks.Find(`a [[shop:x]] b [[article:y|opis]]`)
	if len(refs) != 2 || refs[0].ID != "x" || refs[1].Label != "opis" {
		t.Errorf("refs = %+v", refs)
	}
	if got := crosslinks.Strip(`a [[shop:x]] b [[article:y|opis]]`); got != "a  b opis" {
		t.Errorf("Strip = %q", got)
	}
}
