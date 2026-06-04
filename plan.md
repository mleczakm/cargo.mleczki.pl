Plan Pracy: System Wypożyczalni Rowerów (Go, SQLite, Event-Sourcing, HTMX)

Poniższy dokument stanowi szczegółowy plan działania (roadmap) dla agenta AI/programisty, mający na celu transformację dostarczonego prototypu React (mockupu) w pełni funkcjonalną aplikację serwerową napisaną w języku Go, opartą o architekturę Event-Sourcing i bazę SQLite. Aplikacja ma dzielić środowisko i sposób wdrożenia z projektem classy-cash.

Faza 1: Inicjalizacja projektu i architektura bazowa

Cel: Utworzenie szkieletu aplikacji w Go, konfiguracja bazy danych i przygotowanie środowiska pod deployment analogiczny do classy-cash.

Konfiguracja repozytorium i Go Modules:

Inicjalizacja go mod init.

Ustalenie struktury katalogów (np. standard Standard Go Project Layout: /cmd, /internal, /web/templates, /web/static).

Architektura Event-Sourcing (Szkielet):

Definicja interfejsów dla EventStore (Zapis/Odczyt zdarzeń).

Definicja bazowego interfejsu Event oraz struktury logu zdarzeń (np. aggregate_id, aggregate_type, event_type, payload, created_at).

Konfiguracja bazy SQLite:

Integracja sterownika SQLite (np. mattn/go-sqlite3 lub czystego sterownika CGo-free modernc.org/sqlite).

Inicjalizacja schematu bazy danych. Powinny powstać dwie przestrzenie (lub dwa pliki .db dla czystości):

event_store.db - Tabela events przechowująca strumień zdarzeń (append-only).

read_models.db - Tabele projekcji (zdenormalizowane widoki: orders, users, transfers).

Przygotowanie pod Deployment (wzór classy-cash):

Stworzenie wieloetapowego pliku Dockerfile budującego statyczny plik binarny Go (z wbudowanym SQLite i szablonami HTML za pomocą embed).

Konfiguracja skryptu CI/CD (np. GitHub Actions) lub pliku Makefile, który buduje obraz i przesyła go na serwer (lub synchronizuje binarkę via rsync / systemd w zależności od dokładnego flow serwera, na którym stoi classy-cash).

Faza 2: Domena, Komendy i Zdarzenia (Event-Sourcing)

Cel: Modelowanie procesów biznesowych wypożyczalni za pomocą logiki zdarzeniowej.

Agregat: Zamówienie (Order):

Komendy: PlaceOrder (złóż rezerwację), MarkAsPaid (oznacz jako opłacone - gotówka/BLIK), CancelOrder (anuluj).

Zdarzenia (Events): OrderPlaced (zawiera JSON z wybranym sprzętem, datami, kwotą, danymi klienta), OrderPaid (potwierdzenie płatności), OrderCancelled.

Agregat: Użytkownik (User):

Komendy: RegisterUser, UpdateUserDetails, RequestAccountDeletion.

Zdarzenia: UserRegistered, UserDetailsUpdated, UserDeletionRequested.

Tworzenie Projekcji (Projections/Read Models):

Napisanie "nasłuchiwaczy" (Projectors), które czytają nowe zdarzenia z Event Store i aktualizują standardowe tabele w SQLite.

Przykład: Gdy system odbierze OrderPlaced, projektor dodaje wiersz w tabeli orders_view, a po OrderPaid aktualizuje status tego wiersza na "opłacony".

Agregat: Płatności (Transfers):

Komendy: RegisterTransfer (rejestracja wpłaty ze skrzynki e-mail), LinkTransferToOrder (ręczne lub automatyczne sparowanie wpłaty).

Zdarzenia: TransferReceived, TransferLinked.

Faza 3: Moduł Płatności BLIK (Parsowanie Maili)

Cel: Zbudowanie niezależnego serwisu/workera działającego w tle (goroutine) wewnątrz aplikacji Go, który obsługuje autorski system płatności.

Integracja IMAP:

Użycie biblioteki do obsługi protokołu IMAP w Go (np. github.com/emersion/go-imap).

Połączenie ze skrzynką e-mail skonfigurowaną do odbierania powiadomień bankowych.

Parsowanie i ekstrakcja danych:

Worker uruchamiany cyklicznie (np. co 30 sekund).

Odczyt nieprzeczytanych wiadomości, mapowanie tematu wiadomości ("CARGO-XXXX"), kwoty przelewu oraz danych nadawcy za pomocą wyrażeń regularnych (Regex).

Logika obsługi płatności:

Po pomyślnym sparsowaniu maila, system emituje komendę RegisterTransfer.

System automatycznie sprawdza, czy istnieje aktywne zamówienie o ID pasującym do tytułu ("CARGO-XXXX") oraz czy kwota się zgadza.

Jeśli tak -> Emituje LinkTransferToOrder i następnie MarkAsPaid dla zamówienia.

Oznaczenie wiadomości e-mail na serwerze jako przeczytanej.

Faza 4: Warstwa Prezentacji (HTMX, Go Templates, Markdown)

Cel: Przeniesienie wyglądu z mockupu React do natywnego ekosystemu Go (Server-Side Rendering + HTMX).

Obsługa produktów (Flat-file CMS):

Wykorzystanie biblioteki goldmark do renderowania opisów sprzętów.

Zdefiniowanie struktury folderu /data/products/*.md. Każdy plik Markdown zawiera Frontmatter (YAML) z atrybutami (cena, nazwa, ID, zdjęcia, opcje dodatkowe) oraz właściwy opis.

Dodanie opcji "bookedDates" (lub wyliczanie ich dynamicznie z bazy danych na podstawie aktywnych rezerwacji z projekcji).

Renderowanie szablonów HTML:

Pocięcie mockupu React na szablony html/template (np. layout.html, home.html, product.html, checkout.html, admin.html).

Zastosowanie klas TailwindCSS skopiowanych bezpośrednio z mockupu.

Integracja HTMX (Interaktywność bez JS):

Zastąpienie logiki useState z React wywołaniami AJAX za pomocą atrybutów HTMX (np. hx-post, hx-get, hx-target).

Przykład Koszyka: Kliknięcie "Dodaj do koszyka" wysyła request na backend. Backend odpowiada małym fragmentem HTML aktualizującym ikonę koszyka w nawigacji.

Przykład Czekania na BLIK: Ekran ładowania płatności używa hx-trigger="every 3s" uderzając w /api/order/status/CARGO-1234. Gdy backend zaktualizuje status na opłacony, zwraca fragment HTML z podziękowaniem, zastępując ekran ładowania.

Panel Klienta i Administratora (SSR):

Wyświetlanie list zamówień i użytkowników na podstawie odpytywania bazy projekcji (Read Models SQLite).

Działania admina (np. "Połącz przelew") obsługiwane jako zwykłe żądania POST z przeładowaniem tabeli (HTMX hx-swap="outerHTML").

Faza 5: Bezpieczeństwo i Sesje

Cel: Zabezpieczenie paneli oraz implementacja koszyka i logowania zgodnie z wymogami prawnymi.

Obsługa Sesji i Uwierzytelniania:

Implementacja sesji opartych na ciasteczkach (HttpOnly, Secure) - użycie np. biblioteki gorilla/sessions.

Mechanizm logowania z hashem haseł (bcrypt).

Konto koszyka "gościa" oparte na ID sesji przed zalogowaniem/rejestracją.

Autoryzacja (RBAC):

Prosty podział na role: Client i Admin.

Middleware w Go weryfikujący token/ciasteczko dla endpointów z grupy /admin oraz /user.

Zgodność RODO (Prawo do zapomnienia):

Obsługa komendy RequestAccountDeletion.

Dodanie logicznego usuwania danych z projekcji (anonimizacja) lub maskowania danych w logu zdarzeń (zależnie od wybranego podejścia do Event-Sourcingu, np. Crypto-shredding - szyfrowanie danych wrażliwych kluczem, który jest trwale niszczony).

Kolejność wykonywania zadań (Dla Agenta)

Sprint 1 (Backend Core): Konfiguracja Go, SQLite, struktura projektu, logika Event-Sourcing (Interfejsy, komendy, eventy bazowe).

Sprint 2 (Produkty & Katalog): Parser Markdown (Flat-file CMS dla sprzętu), podstawowe szablony HTML (Home, Produkt), konfiguracja TailwindCSS.

Sprint 3 (Booking & Checkout): Logika koszyka w sesji, mechanizm kalendarza (blokowanie dni na podstawie rezerwacji), formularz zamówienia (tworzenie usera). HTMX.

Sprint 4 (BLIK Worker): Moduł IMAP w tle, parsowanie maili, automatyczne dopasowywanie wpłat i event OrderPaid.

Sprint 5 (Panele & CRM): Widoki User Panel, Admin Panel. HTMX dla interakcji (ręczne dopasowywanie wpłat, edycja usera).

Sprint 6 (Deploy): Spakowanie całości w Docker, synchronizacja metod wdrażania z serwerem classy-cash. Testy.