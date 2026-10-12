---
# PRZYKŁAD – widoczny tylko dla admina (visibility: admin). Usuń plik albo zmień na public.
id: przyklad-dzwonek
name: Dzwonek rowerowy (przykład)
category: akcesoria
price: 24.90
icon: 🔔
visibility: admin
priority: 10
relatedProducts: [cargo]
stock:
  initial: 10        # stan startowy, wczytywany tylko raz; dalej zmieniasz go w panelu admina
  lowThreshold: 3    # od tylu sztuk "Ostatnie sztuki"
  maxPerOrder: 5
---
Donośny dzwonek, który zmieści się na każdej kierownicy – także w rowerze cargo.

Sprawdź nasz [[product:cargo]], a do niego dobierz [[shop:przyklad-kask|kask w dwóch rozmiarach]].
