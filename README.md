# cargo.mleczki.pl

System wypożyczalni sprzętu rowerowego zbudowany w Go, wykorzystujący architekturę Event-Sourcing, bazę SQLite, HTMX i szablony HTML.

## Architektura

### Event-Sourcing
Aplikacja wykorzystuje architekturę Event-Sourcing do przechowywania wszystkich zmian stanu jako zdarzeń:
- **Event Store**: Baza SQLite (`db/event_store.db`) przechowująca strumień zdarzeń (append-only)
- **Read Models**: Baza SQLite (`db/read_models.db`) z zdenormalizowanymi widokami dla szybkiego odczytu
- **Projectors**: Usługi w tle przetwarzające zdarzenia i aktualizujące read models

### Struktura projektu
```
.
├── cmd/server/          # Główna aplikacja serwera
├── internal/
│   ├── domain/         # Definicje domenowe (agregaty, komendy, zdarzenia)
│   ├── eventstore/     # Implementacja Event Store
│   ├── projections/    # System projekcji
│   └── products/       # Parser produktów Markdown (Flat-file CMS)
├── web/
│   ├── templates/      # Szablony HTML z TailwindCSS
│   └── static/         # Pliki statyczne
├── data/
│   └── products/       # Pliki Markdown z definicjami produktów
├── db/                 # Bazy danych (tworzone automatycznie)
├── Dockerfile          # Konfiguracja Docker
└── Makefile           # Skrypty budowania
```

## Funkcjonalności

### Zaimplementowane
- ✅ Event-Sourcing z SQLite
- ✅ System projekcji read models
- ✅ Flat-file CMS dla produktów (Markdown z YAML frontmatter)
- ✅ Szablony HTML z TailwindCSS
- ✅ HTMX dla interaktywności (koszyk, kalendarz)
- ✅ Przepływ rezerwacji i checkout
- ✅ Sesje koszyka (cookies)
- ✅ Podstawowe szablony paneli (User, Admin)

### Do zaimplementowania (zgodnie z planem)
- ⏳ IMAP worker dla automatycznego parsowania płatności BLIK
- ⏳ Pełna implementacja uwierzytelniania i sesji
- ⏳ RBAC (Role-Based Access Control)
- ⏳ GDPR - prawo do zapomnienia
- ⏳ Pełna integracja z Event-Sourcing dla zamówień

## Szybki start

### Wymagania
- Go 1.22+
- SQLite 3

### Instalacja
```bash
# Zainstaluj zależności
go mod download

# Zbuduj aplikację
go build ./cmd/server

# Uruchom serwer
./server
```

Serwer będzie dostępny na `http://localhost:8080`

### Docker
```bash
# Zbuduj obraz
make docker-build

# Uruchom kontener
make docker-run
```

## Dodawanie produktów

Produkty są definiowane jako pliki Markdown w katalogu `data/products/`:

```yaml
---
id: cargo
name: Rower Cargo (Longjohn)
base_price: 100
image: https://example.com/image.jpg
icon: 🚲
booked_dates:
  - "2026-06-10"
  - "2026-06-11"
addons:
  - id: daszek
    name: Daszek przeciwdeszczowy
    price: 15
    icon: ☂️
---

Idealny do przewozu dzieci i towarów. Szybki, zwinny i pakowny.
```

## API

### Publiczne endpointy
- `GET /` - Strona główna z listą produktów
- `GET /product/{id}` - Szczegóły produktu z kalendarzem
- `GET /login` - Formularz logowania
- `GET /checkout` - Koszyk i formularz zamówienia
- `GET /payment` - Strona płatności

### API HTMX
- `POST /cart/add` - Dodaj produkt do koszyka
- `POST /cart/remove/{id}` - Usuń z koszyka
- `POST /checkout/submit` - Złóż zamówienie
- `GET /product/{id}/calendar` - Widget kalendarza
- `POST /payment/confirm` - Potwierdź płatność
- `GET /payment/status/{id}` - Sprawdź status płatności (polling)

### Panele
- `GET /user` - Panel klienta
- `GET /admin` - Panel administratora
- `GET /admin/user/{id}` - Szczegóły użytkownika (CRM)

## Plan rozwoju

Szczegółowy plan rozwoju znajduje się w pliku `plan.md`.

### Sprint 1 ✅
- Inicjalizacja projektu Go
- Architektura Event-Sourcing
- Konfiguracja SQLite
- Dockerfile

### Sprint 2 ✅
- Agregaty domenowe (Order, User, Transfer)
- System projekcji
- Parser Markdown dla produktów

### Sprint 3 ✅
- Szablony HTML z TailwindCSS
- Integracja HTMX
- Przepływ rezerwacji i checkout

### Sprint 4 ⏳
- IMAP worker dla płatności BLIK
- Automatyczne dopasowywanie przelewów

### Sprint 5 ⏳
- Uwierzytelnianie i sesje
- RBAC
- GDPR - prawo do zapomnienia

### Sprint 6 ⏳
- Finalne wdrożenie
- Testy

## Licencja

Prywatny projekt.
