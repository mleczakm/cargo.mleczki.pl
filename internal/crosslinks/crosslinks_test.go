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

func cardIndex() *crosslinks.Index {
	ix := testIndex()
	ix.Add(crosslinks.Target{Kind: crosslinks.KindShop, ID: "rzepy", Title: "Rzepy", URL: "/shop/rzepy", Card: `<b>karta rzepów</b>`})
	ix.Add(crosslinks.Target{Kind: crosslinks.KindShop, ID: "lampka", Title: "Lampka", URL: "/shop/lampka/go", External: true, Card: `<b>karta lampki</b>`})
	ix.Add(crosslinks.Target{Kind: crosslinks.KindRent, ID: "brompton", Title: "Brompton", URL: "/product/brompton", Card: `<aside class="rent">Wypróbuj mnie!</aside>`})
	return ix
}

func TestLinksWithACardShowItOnHover(t *testing.T) {
	out, _ := crosslinks.Expand(`<p>Weź [[shop:rzepy]] i [[shop:dzwonek]].</p>`, cardIndex().Lookup)
	want := `<span class="hc"><a href="/shop/rzepy" class="text-emerald-700 underline hover:text-emerald-800" data-link-kind="shop">Rzepy</a><span class="hc-card" role="tooltip"><b>karta rzepów</b></span></span>`
	if !strings.Contains(out, want) {
		t.Errorf("missing the hover card:\n%s", out)
	}
	if strings.Contains(out, `/shop/dzwonek" class="text-emerald-700 underline hover:text-emerald-800" data-link-kind="shop">Dzwonek</a><span`) {
		t.Errorf("a link without a card must stay a plain link:\n%s", out)
	}
}

func TestExternalLinksOpenInANewWindowAndSayThey(t *testing.T) {
	out, _ := crosslinks.Expand(`<p>[[shop:lampka|polecana lampka]]</p>`, cardIndex().Lookup)
	for _, want := range []string{
		`<a href="/shop/lampka/go"`, `target="_blank"`, `rel="sponsored nofollow noopener noreferrer"`, `>polecana lampka</a>`,
		`<span class="hc-ext" aria-hidden="true" title="Otwiera się w nowym oknie">`, `<b>karta lampki</b>`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

func TestRentReferenceBecomesABlockEmbed(t *testing.T) {
	out, unresolved := crosslinks.Expand("<p>Przed.</p>\n<p>[[rent:brompton]]</p>\n<p>Po [[rent:brompton]] w zdaniu i [[rent:nie-ma]].</p>", cardIndex().Lookup)
	if !strings.Contains(out, "<p>Przed.</p>\n<aside class=\"rent\">Wypróbuj mnie!</aside>\n<p>Po ") {
		t.Errorf("the embed must replace its own paragraph:\n%s", out)
	}
	if strings.Contains(out, "<p><aside") {
		t.Errorf("an embed is a block and must not sit inside a paragraph when it stands alone:\n%s", out)
	}
	if len(unresolved) != 1 || unresolved[0].ID != "nie-ma" || strings.Contains(out, "nie-ma") {
		t.Errorf("an unknown embed leaves no trace but is reported: %+v\n%s", unresolved, out)
	}
	if got := crosslinks.PlainText(`[[rent:brompton]]`, cardIndex().Lookup); got != "Brompton" {
		t.Errorf("PlainText = %q", got)
	}
}
