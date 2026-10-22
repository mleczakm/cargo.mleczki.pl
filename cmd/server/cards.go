package main

import (
	"bytes"
	stdhtml "html"
	"log"
	"net/http"
	"regexp"
	"strings"
	"unicode/utf8"

	"cargo.mleczki.pl/internal/crosslinks"
	"cargo.mleczki.pl/internal/domain"
)

var htmlTag = regexp.MustCompile(`<[^>]*>`)

// plainSummary turns rendered HTML into a short plain-text teaser.
func plainSummary(rendered string, limit int) string {
	text := htmlTag.ReplaceAllString(crosslinks.Strip(rendered), " ")
	text = strings.Join(strings.Fields(stdhtml.UnescapeString(text)), " ")
	if utf8.RuneCountInString(text) <= limit {
		return text
	}
	runes := []rune(text)
	cut := strings.TrimSpace(string(runes[:limit-1]))
	if i := strings.LastIndex(cut, " "); i > limit/2 {
		cut = cut[:i]
	}
	return cut + "…"
}

// productCardView is what the product preview and the rental embed show.
type productCardView struct {
	domain.Product
	Summary string
}

func (s *Server) renderCard(name string, data any) string {
	var buf bytes.Buffer
	if err := s.templates.ExecuteTemplate(&buf, name, data); err != nil {
		log.Printf("Card template %s: %v", name, err)
		return ""
	}
	return buf.String()
}

// linkLookup resolves [[kind:id]] references for the visitor and equips them
// with a preview card: the shop item with its price and "add to cart" button,
// the rental with its "try me" button, or the whole rental embed.
func (s *Server) linkLookup(r *http.Request) func(kind, id string) (crosslinks.Target, bool) {
	index := s.linkIndex(r)
	return func(kind, id string) (crosslinks.Target, bool) {
		target, ok := index.Lookup(kind, id)
		if !ok {
			return target, false
		}
		switch kind {
		case crosslinks.KindShop:
			item, found := s.shopItemByID(r, id)
			if !found {
				return crosslinks.Target{}, false
			}
			view := s.shopViews(r, []domain.ShopItem{item})[0]
			target.Card = s.renderCard("hover-card-shop", view)
			if item.IsAffiliate() {
				// A link that leaves the site says so and opens in a new window.
				target.URL = "/shop/" + item.ID + "/go"
				target.External = true
			}
		case crosslinks.KindProduct, crosslinks.KindRent:
			if s.productParser == nil {
				return target, true
			}
			product, err := s.productParser.LoadProductByID(id)
			if err != nil {
				return crosslinks.Target{}, false
			}
			view := productCardView{Product: *product, Summary: plainSummary(product.Description, 140)}
			if kind == crosslinks.KindRent {
				target.Card = s.renderCard("rent-embed", view)
			} else {
				target.Card = s.renderCard("hover-card-product", view)
			}
		}
		return target, true
	}
}
