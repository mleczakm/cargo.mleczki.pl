# Sklep

Sklep sprzedaje akcesoria (**direct**: koszyk, płatność BLIK lub gotówka przy odbiorze, odbiór w Radzyminie albo wysyłka) i poleca
produkty z zewnętrznych serwisów (**affiliate**: tylko link). Katalog to pliki Markdown w `data/shop/`,
tak jak produkty i artykuły. Działa od razu po dodaniu pliku, bez restartu.

Strony: `/shop`, `/shop/{id}`. Link „Sklep” w menu pojawia się, gdy jest co pokazać.

## Ceny

Wszystkie kwoty (sklep, `basePrice` i `price` dodatków w produktach do wynajęcia, zamówienia, przelewy)
są liczone dokładnie, w groszach, przez bibliotekę
[go-money](https://github.com/Rhymond/go-money) – bez liczb zmiennoprzecinkowych. W plikach możesz pisać
`25`, `24.90` albo `"24,90"` (przecinek w cudzysłowie); więcej niż dwa miejsca po przecinku jest błędem
zamiast cichego zaokrąglenia. Na stronach kwoty wyglądają jak `25 zł` i `24,90 zł`.

W zdarzeniach kwoty są zapisywane jako tekst (`"24.90"`). Zdarzenia sprzed wprowadzenia groszy miały
liczbę całkowitą w złotych (`150`) i nadal są czytane poprawnie.

## Plik produktu

```yaml
---
id: dzwonek                 # a-z, 0-9, myślnik; trafia do adresu URL
name: Dzwonek rowerowy
category: akcesoria         # filtr na stronie /shop
price: 24.90                # w złotych, do 2 miejsc po przecinku (24.90, "24,90" albo 25)
image: dzwonek.jpg          # data/images/shop/dzwonek.jpg, albo pełny URL
images: [dzwonek.jpg, dzwonek-2.jpg]
icon: 🔔                    # gdy brak zdjęcia
summary: Krótki opis do kart (opcjonalnie; domyślnie początek opisu)
visibility: public          # public | logged-in | admin | hidden
priority: 10                # wyżej = wcześniej

relatedProducts: [cargo]            # sprzęt z wypożyczalni
relatedArticles: [jak-wybrac-rower]
relatedShop: [kask]

stock:
  mode: tracked             # tracked (domyślnie) | unlimited
  initial: 10               # stan początkowy, wczytany RAZ przy pierwszym uruchomieniu
  lowThreshold: 3           # <= tyle sztuk => „Ostatnie sztuki” (domyślnie 3)
  backorder: false          # true => można zamówić mimo braku
  leadTime: 5–7 dni         # pokazywane przy backorderze
  maxPerOrder: 5            # domyślnie 10
---
Opis w Markdown. Linki do reszty serwisu: [[product:cargo]], [[article:jak-wybrac-rower]],
[[shop:kask]] albo z własnym tekstem: [[shop:kask|nasz kask]].
```

### Warianty (rozmiar, kolor…)

```yaml
price: 120                  # cena domyślna
variants:
  - sku: kask-s             # SKU unikalne w całym sklepie
    name: Rozmiar S
    stock: 4                # stan początkowy tego wariantu
  - sku: kask-l
    name: Rozmiar L
    price: 130              # opcjonalna własna cena
```

`stock.initial` nie łączy się z wariantami; każdy wariant ma własny stan.

### Produkt afiliacyjny

```yaml
id: lampka
name: Lampka USB
affiliate:
  url: https://allegro.pl/oferta/...
  merchant: allegro         # opcjonalnie; domyślnie z adresu (Allegro, AliExpress, Amazon…)
  label: Zobacz ofertę na Allegro
  priceHint: od ok. 40 zł   # tekst, nie cena do zapłaty
```

Bez `price`, `stock` i wariantów. Przycisk prowadzi przez `/shop/{id}/go`, które liczy kliknięcia
(panel admina → Sklep → Linki afiliacyjne) i przekierowuje do oferty. Linki mają
`rel="sponsored nofollow"`, a strona zawiera informację o linkach afiliacyjnych.

## Powiązania

* `[[product:ID]]`, `[[article:ID]]`, `[[shop:ID]]` działają w opisach produktów, artykułów i sklepu.
  Domyślnym tekstem jest aktualna nazwa celu; `[[shop:ID|tekst]]` ustawia własny.
* Cel, którego odwiedzający nie może zobaczyć (`visibility`), pokazuje się jako zwykły tekst – link nie
  zdradza ukrytych rzeczy.
* Relacje wystarczy zadeklarować po jednej stronie: `relatedProducts` / `relatedArticles` / `relatedShop`
  w produkcie sklepu, `shop:` w produkcie do wynajęcia, `relatedShop:` w artykule. Strony pokazują obie strony relacji.
* Literówki wychodzą w panelu admina (zakładka **Sklep**, „Do poprawienia w plikach katalogu”).

## Magazyn

Stan żyje w bazie (`shop_stock`, dziennik `shop_stock_movements`), nie w plikach.

* **Na stanie** – sztuki fizycznie u nas. **Zarezerwowane** – sztuki z niezrealizowanych zamówień.
  **Dostępne** = na stanie − zarezerwowane.
* Złożenie zamówienia rezerwuje sztuki w jednej, atomowej operacji – dwie osoby nie kupią ostatniej sztuki.
* Anulowanie zamówienia (przez klienta lub wygaśnięcie) zwalnia rezerwację.
* Admin oznacza potwierdzone zamówienie jako **zrealizowane** – wtedy sztuki schodzą ze stanu.
  Przy backorderze trzeba najpierw przyjąć dostawę, inaczej system odmówi wydania.
* Panel admina → **Sklep**: dostawa, zwrot od klienta, strata/uszkodzenie, inwentaryzacja, historia ruchów.
  Korekta nie zejdzie poniżej tego, co trzymają otwarte zamówienia.
* Nieopłacone zamówienia ze sklepu wygasają po `SHOP_RESERVATION_TTL_HOURS` godzin (domyślnie 48, `0` = nie
  wygasają) i oddają towar na półkę. Dotyczy to też wypożyczeń w tym samym zamówieniu.

## Dostawa (Polkurier)

Opcje dostawy i dane nadawcy są w `data/shipping.yml` (wczytywany na bieżąco; błąd w pliku nie psuje
sklepu, działa poprzednia wersja, a panel pokazuje ostrzeżenie). Odbiór osobisty działa zawsze.
Paczkomat i kurier pojawiają się w koszyku dopiero, gdy ustawisz dane konta Polkurier:

```
POLKURIER_LOGIN=<id konta>
POLKURIER_TOKEN=<token API>
POLKURIER_SANDBOX=false     # domyślnie true (środowisko testowe)
```

* Cenę dostawy ustalasz sam (`price`, `freeFrom` = darmowa od tej wartości towaru). Prawdziwy koszt
  przesyłki widzisz w panelu przy zamówieniu, razem z marżą.
* Klient wyszukuje paczkomat po mieście, ulicy lub kodzie (podpowiedzi z Polkuriera) albo podaje adres dla
  kuriera. Przesyłkę opłaca BLIK-iem, gotówka przy odbiorze dotyczy tylko odbioru osobistego.
* Zamówienie z wypożyczanym sprzętem jest zawsze do odbioru osobistego, tak samo produkty z `shipping:
  {pickupOnly: true}` (np. duży daszek).
* Waga i wymiary produktu: `shipping: {weightG: 400, lengthCm: 30, widthCm: 20, heightCm: 8}` w pliku produktu;
  bez tego liczy się domyślna paczka z `shipping.yml`.
* Panel admina → Zamówienia: kolejka **Do wysłania** i strona zamówienia (`/admin/order/{id}`): wycena
  przewoźników, formularz paczki, etykieta PDF, śledzenie, anulowanie. Po nadaniu zamówienie można od
  razu oznaczyć jako wysłane (towar schodzi ze stanu, klient dostaje e-mail ze śledzeniem).
* Co 30 minut statusy paczek są odświeżane; paczka „w drodze” oznacza zamówienie jako wysłane.

## Ograniczenia

* Regulamin (`/terms`) opisuje wynajem; zakupy w sklepie wymagają własnych zapisów (odstąpienie od umowy,
  reklamacje).

## Koszyk i ceny wynajmu

Koszyk trzyma w ciasteczku tylko to, co wybrał klient: produkt, terminy, dodatki, ilości. Nazwy, ceny,
liczba dni i sumy są za każdym razem wyliczane na serwerze z plików katalogu – przy dodawaniu, na stronie
koszyka i przy składaniu zamówienia – więc zmiana ciasteczka niczego nie tanieje.

* Cena wynajmu = (cena bazowa + dodatki „za dobę”) × liczba dni + dodatki jednorazowe.
* Każda pozycja wynajmu ma własne terminy i blokuje w kalendarzu tylko swoje dni.
* Pozycje, które się nakładają (ten sam sprzęt w koszyku), są zajęte lub z przeszłością, są oznaczone na
  czerwono i blokują złożenie zamówienia. Jedna pozycja może trwać najwyżej 90 dni.

## Linki w tekstach

W artykułach, opisach produktów i sklepu `[[shop:ID]]` pokazuje po najechaniu kartę: zdjęcie, opis,
cenę, dostępność i ikonę „do koszyka”. Produkty z wariantami prowadzą do wyboru wersji. Produkt
afiliacyjny jest linkiem ze znaczkiem ↗, który otwiera ofertę w nowym oknie. `[[product:ID]]` pokazuje
kartę z przyciskiem „Wypróbuj mnie!”, a `[[rent:ID]]` w osobnej linii wstawia dużą ramkę wynajmu.
