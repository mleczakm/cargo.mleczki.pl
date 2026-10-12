---
# PRZYKŁAD – produkt z wariantami (każdy ma własny SKU i stan) oraz zamówieniem "pod zamówienie".
id: przyklad-kask
name: Kask rowerowy (przykład)
category: akcesoria
price: 120
icon: ⛑️
visibility: admin
relatedProducts: [cargo]
relatedArticles: [trekking-bike-guide]
stock:
  backorder: true            # gdy brak na półce, można zamówić
  leadTime: 5–7 dni roboczych
variants:
  - sku: przyklad-kask-s
    name: Rozmiar S
    stock: 4
  - sku: przyklad-kask-l
    name: Rozmiar L
    price: 130                # własna cena; bez tego obowiązuje price z góry
    stock: 0
---
Lekki kask z regulacją obwodu. Zobacz też [[article:trekking-bike-guide|przewodnik po wyborze roweru]].
