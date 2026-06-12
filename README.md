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
- ✅ IMAP worker dla automatycznego parsowania płatności BLIK
- ✅ Pełna implementacja uwierzytelniania i sesji
- ✅ RBAC (Role-Based Access Control)
- ✅ Pełna integracja z Event-Sourcing dla zamówień
- ✅ Globalne zamknięcia sklepu (admin panel)
- ✅ Pobieranie rzeczywistych danych w panelu użytkownika

## Szybki start

### Wymagania
- Go 1.26.4+
- SQLite 3
- (Opcjonalnie) Air - hot reload (`go install github.com/cosmtrek/air@latest`)
- (Opcjonalnie) golangci-lint - linting (`curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(go env GOPATH)/bin`)
- (Opcjonalnie) gosec - security checks (`go install github.com/securego/gosec/v2/cmd/gosec@latest`)

### Instalacja i uruchomienie

#### Proste uruchomienie
```bash
# Zainstaluj zależności
go mod download

# Uruchom serwer
go run ./cmd/server
```

Serwer będzie dostępny na `http://localhost:8080`

#### Przy użyciu Makefile
```bash
# Wyświetl dostępne komendy
make help

# Zbuduj aplikację
make build

# Uruchom serwer
make run

# Uruchom z hot reload (wymagane: air)
make dev
```

### Docker
```bash
# Zbuduj obraz
make docker-build

# Uruchom kontener
make docker-run
```

## Środowisko Programowania (Development)

### Uruchomienie aplikacji z hot reload

Podczas programowania możesz używać hot reload, aby aplikacja automatycznie restartowała się po zmianach w kodzie:

```bash
# Zainstaluj air (jeśli nie masz)
go install github.com/cosmtrek/air@latest

# Uruchom aplikację z hot reload
make dev
```

Air będzie obserwować pliki `.go` i `.html` w projekcie i automatycznie restartować aplikację po zmianach.
Konfiguracja znajduje się w pliku `.air.toml`.

### Bazy danych lokalne

Aplikacja automatycznie tworzy bazy SQLite w katalogu `db/`:
- `db/event_store.db` - Event Store (append-only stream zdarzeń)
- `db/read_models.db` - Read Models (zdenormalizowane widoki)

Aby wyczyścić bazy i zacząć od nowa:

```bash
# Czyszczenie plików bazy danych
make clean
```

## Kontrola Jakości (QA)

### Uruchomienie testów

```bash
# Uruchom wszystkie testy
make test

# Uruchom testy z raport pokrycia kodu
make test-coverage
# Raport otworzy się w przeglądarce (coverage.html)
```

### Uruchomienie analizy kodu

```bash
# Sprawdzenie formatowania kodu
make fmt-check

# Format kodu (auto-fix)
make fmt

# Go vet (analiza statyczna)
make vet

# Linting (wymaga golangci-lint)
make lint

# Security checks (wymaga gosec)
make security

# WSZYSTKIE QA checks naraz
make qa
```

### Instalacja narzędzi QA

Jeśli narzędzia nie są zainstalowane, możesz je zainstalować następująco:

```bash
# golangci-lint - comprehensive linter
curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(go env GOPATH)/bin

# gosec - security checker
go install github.com/securego/gosec/v2/cmd/gosec@latest

# air - hot reload
go install github.com/cosmtrek/air@latest
```

## GitHub Actions CI/CD

Projekt zawiera automatyczną pipeline CI/CD skonfigurowaną w `.github/workflows/ci.yml`.

### Co sprawdza CI/CD

Pipeline automatycznie uruchamia się na każde push'a i pull request do gałęzi `main` i `develop`:

1. **Tests** - Uruchamia wszystkie testy z race condition detection
   - Wynik jest wysyłany do Codecov
   
2. **Lint** - Sprawdza kod przy użyciu golangci-lint
   - Konfiguracja w `.golangci.yml`
   
3. **Format Check** - Sprawdza czy kod jest poprawnie sformatowany
   - Wymusza gofmt na całym kodzie
   
4. **Build** - Kompiluje aplikację
   - Generuje artefakt (`cargo-server`)
   
5. **Security** - Uruchamia Gosec dla security checks
   - Wyniki widoczne w GitHub Security tab
   
6. **Docker** - Buduje Docker image (tylko na main branch)
   - Cache'uje warstwy dla szybszych buildów

### Widok statusu

Status pipeline'u vidać na karcie **Actions** w repozytorium GitHub.

Aby pull request został zaakceptowany, wszystkie checki muszą przejść.

### Lokalne testowanie pipeline'u

Możesz testować workflow lokalnie używając act (https://github.com/nektos/act):

```bash
# Zainstaluj act
brew install act  # macOS
# lub dla Ubuntu/andere systemy patrz: https://github.com/nektos/act

# Uruchom workflow lokalnie
act -j test      # Uruchom konkretny job
act               # Uruchom wszystkie jobs
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
