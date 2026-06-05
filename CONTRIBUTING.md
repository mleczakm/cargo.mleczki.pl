# Contributing Guide

Dziękujemy za zainteresowanie wkładem w nasz projekt! Poniżej znajdziesz instrukcje jak pracować nad projektem.

## Setup Środowiska Programowania

### 1. Klonowanie repo
```bash
git clone https://github.com/yourusername/cargo.mleczki.pl.git
cd cargo.mleczki.pl
```

### 2. Instalacja zależności Go
```bash
go mod download
```

### 3. Instalacja narzędzi deweloperskich (opcjonalnie, ale zalecane)

```bash
# Hot reload
go install github.com/cosmtrek/air@latest

# Linting
curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(go env GOPATH)/bin

# Security checks
go install github.com/securego/gosec/v2/cmd/gosec@latest
```

## Workflow

### 1. Utwórz branch
```bash
git checkout -b feature/my-feature
# lub
git checkout -b bugfix/my-bugfix
```

### 2. Programowanie z hot reload
```bash
make dev
```

### 3. Pisanie testów
```bash
# Przegląd istniejących testów
find . -name "*_test.go" | head -10

# Uruchomienie testów
make test

# Testy z raport pokrycia
make test-coverage
```

### 4. Sprawdzenie jakości kodu

Przed pushowaniem, uruchom wszystkie QA checki:

```bash
# Wszystkie checks naraz
make qa

# Lub indywidualnie:
make fmt-check    # Formatowanie
make vet           # Go vet analysis
make lint          # Linting
make test          # Testy
```

Jeśli są problemy z formatowaniem, napraw je:
```bash
make fmt
```

### 5. Commit i push
```bash
git add .
git commit -m "feat: add new feature" # lub "fix: fix bug"
git push origin feature/my-feature
```

### 6. Pull Request
- Otwórz PR na GitHub
- Opisz zmiany w descriptions
- Czekaj na review i CI/CD checks

## Git Commit Conventions

Używamy [Conventional Commits](https://www.conventionalcommits.org/):

```
feat: add new feature
fix: fix a bug
docs: documentation changes
style: formatting, missing semicolons, etc
refactor: code refactoring
perf: performance improvements
test: adding tests
chore: maintenance
ci: CI/CD changes
```

Przykłady:
```bash
git commit -m "feat: add product calendar widget"
git commit -m "fix: resolve race condition in event store"
git commit -m "docs: update README with setup instructions"
```

## Code Style

### Formatowanie
Go code should be formatted using `gofmt`:
```bash
make fmt
```

### Konwencje nazewnictwa
- Package names: lowercase, concise
- Function names: camelCase, descriptive
- Constants: SCREAMING_SNAKE_CASE
- Private functions: start with lowercase

### Dokumentacja
- Eksportowane funkcje muszą mieć komentarze (godoc)
- Komentarze powinny zaczynać się od nazwy funkcji
```go
// HandleCheckout processes the checkout request
func (s *Server) HandleCheckout(w http.ResponseWriter, r *http.Request) {
	// ...
}
```

## Testowanie

### Uruchomienie testów
```bash
# Wszystkie testy
make test

# Konkretny package
go test -v ./internal/eventstore

# Z pokrycie kodu
make test-coverage
```

### Pisanie testów
```go
func TestMyFunction(t *testing.T) {
	result := MyFunction("input")
	expected := "output"
	if result != expected {
		t.Errorf("Expected %s, got %s", expected, result)
	}
}
```

## CI/CD Pipeline

Po push'u PR, GitHub Actions automatycznie sprawdza:

1. ✅ Tests - Czy testy przechodzą?
2. ✅ Lint - Czy kod jest poprawnie sformatowany i linted?
3. ✅ Format - Czy kod przechodzi gofmt?
4. ✅ Build - Czy aplikacja się kompiluje?
5. ✅ Security - Czy są security issues?

Wszystkie checks muszą przejść zanim PR zostanie zaakceptowany.

## Struktura Projektu

```
.
├── cmd/server/           # Główna aplikacja
├── internal/
│   ├── domain/          # Logika biznesowa (agregaty)
│   ├── eventstore/      # Event Store implementation
│   ├── handlers/        # HTTP handlers (future)
│   ├── projections/     # Read models projections
│   ├── products/        # Product parser
│   └── middleware/      # HTTP middleware (future)
├── web/
│   ├── templates/       # HTML templates
│   └── static/          # Static assets
├── data/
│   └── products/        # Product definitions (Markdown)
├── db/                  # SQLite databases (generated)
├── .github/workflows/   # GitHub Actions CI/CD
├── .golangci.yml        # Linter config
└── .air.toml            # Hot reload config
```

## Event-Sourcing Architecture

Projekt używa Event-Sourcing pattern:

1. **Events** - Zdarzenia reprezentujące zmianę stanu
2. **Event Store** - Append-only log zdarzeń (SQLite)
3. **Aggregates** - Domeny modele (Order, User, Transfer)
4. **Projections** - Procesory zdarzeń aktualizujące Read Models
5. **Read Models** - Zdenormalizowane dane dla szybkiego odczytu (SQLite)

### Dodawanie nowego zdarzenia

1. Zdefiniuj event w `internal/domain/`:
```go
type OrderPlacedEvent struct {
	EventID    string
	OrderID    string
	UserID     string
	Items      []string
	Total      int
	Timestamp  time.Time
}
```

2. Dodaj handler w Event Store:
```go
case "order.placed":
	var event domain.OrderPlacedEvent
	json.Unmarshal([]byte(e.Data), &event)
	// process event
```

3. Dodaj projekcję w `internal/projections/`:
```go
func (p *Projector) ProjectOrderPlaced(event domain.OrderPlacedEvent) error {
	// update read models
}
```

## Debugging

### Logi
```bash
# Włącz verbose mode w kodzie
log.Printf("Debug: %v", value)

# Uruchom z logami
make run
```

### Baza danych
```bash
# Przegląd event store
sqlite3 db/event_store.db
> SELECT * FROM events LIMIT 10;

# Przegląd read models
sqlite3 db/read_models.db
> SELECT * FROM orders LIMIT 10;
```

### Hot Reload Issues
Jeśli hot reload nie działa:
```bash
# Wyczyść tmp directory
rm -rf tmp/

# Uruchom ponownie
make dev
```

## FAQ

### Q: Jak dodać nową zależność Go?
A: 
```bash
go get github.com/username/package@v1.0.0
go mod tidy
```

### Q: Jak uruchomić konkretny test?
A:
```bash
go test -v -run TestMyFunction ./internal/package
```

### Q: Jak sprawdzić czy mój kod przejdzie CI?
A:
```bash
make qa      # Uruchom wszystkie QA checks
```

### Q: Gdzie są logi aplikacji?
A: Logi wypisywane są do stdout. W produkcji Configuration w docker-compose.

## Support

Jeśli masz problemy:
1. Sprawdź istniejące issues
2. Otwórz nowy issue z opisem problemu
3. Dla dużych zmian, otwórz discussion najpierw

---

Happy coding! 🚀

