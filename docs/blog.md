# Blog

Wpisy to pliki Markdown w `data/articles/`, zdjęcia w `data/images/articles/`. Działa bez restartu.
Strony: `/articles`, `/articles/{historie|porady|recenzje|przewodnik}`, `/article/{id}`.

```yaml
---
id: moj-wpis                 # a-z, 0-9, myślnik; adres /article/moj-wpis
title: Tytuł
subtitle: Zdanie pod tytułem (opcjonalnie)
category: historie           # historie | porady | recenzje | przewodnik
author: Imię Nazwisko
publishedAt: 2026-10-22
image: hero.jpg              # zdjęcie otwierające wpis (i miniatura w sieciach społecznościowych)
heroFocus: 50% 40%           # którą część zdjęcia zachować przy kadrowaniu
summary: Krótki opis do list
tags: [rower cargo]
status: draft                # draft = tylko dla admina; usuń linię, żeby opublikować
visibility: public           # public | logged-in | admin | hidden
relatedProducts: [cargo]     # wynajem pod wpisem
relatedShop: [rzepy-boczne]  # sklep pod wpisem
menu: [nav, footer]          # linki w menu głównym i w stopce
menuLabel: Moja historia     # tekst w menu (domyślnie tytuł)
menuOrder: 10                # niżej = wcześniej
seo:
  title: Tytuł dla wyszukiwarki
  description: Opis do 160 znaków
  noindex: false
---
```

**Szkice** widzi tylko zalogowany administrator: nie ma ich na listach, w menu, w mapie strony ani w
podglądach, a strona ma `noindex`. Błąd w jednym pliku ukrywa tylko ten wpis; lista „do poprawienia”
jest w panelu admina (zakładka Sklep).

## Układ „opowieść”

Zwykły Markdown plus bloki `::: nazwa ... :::`:

```
::: scene image=hel.jpg focus="50% 30%" tone=dark align=left alt="Opis zdjęcia" credit="Foto: ja"
Pierwsza karta tekstu przewija się nad nieruchomym zdjęciem.
+++
Druga karta, to samo zdjęcie.
:::

::: figure image=a.jpg size=wide      # wide | tall | inline; treść bloku to podpis
Podpis zdjęcia
:::

::: gallery images="a.jpg, b.jpg, c.jpg"
Podpis galerii
:::

::: stats
70 km | pierwszy dzień
35 km | do dziadków
:::

::: note title="Uwaga"
Wyróżniona uwaga.
:::
```

`> cytat` to duży cytat. Zdjęcia, których jeszcze nie ma, są zastąpione tłem, a szkic pokazuje
adminowi listę brakujących plików. Zdjęcia z telefonu zmniejsz do ok. 2000 px szerokości (JPEG, do ok. 500 KB).

## Linki do rzeczy

* `[[shop:ID]]`, `[[shop:ID|własny tekst]]`: po najechaniu karta produktu z ceną i ikoną „do koszyka”.
  Produkt afiliacyjny: link ze znaczkiem ↗ w nowym oknie.
* `[[product:ID]]`: link do wynajmu z kartą „Wypróbuj mnie!”.
* `[[rent:ID]]` w osobnej linii: duża ramka „Do wynajęcia u mnie” z przyciskiem „Wypróbuj mnie!”.
* `[[article:ID]]`: link do wpisu.

Cel niewidoczny dla odwiedzającego (`visibility`, szkic) wyświetla się jako zwykły tekst.

## SEO i miniatury

Każda strona ma `<title>`, opis, adres kanoniczny, Open Graph, Twitter Card i dane strukturalne (Product,
BlogPosting, BikeStore). Miniatury 1200×630 (`/og/...jpg`) rysuje serwer: dla produktów zdjęcie, tytuł,
cena i „Wypróbuj mnie!”, dla wpisów zdjęcie z tytułem. `/sitemap.xml` i `/robots.txt` powstają same.
Adres publiczny ustaw w `SITE_URL` (domyślnie https://cargo.mleczki.pl).
