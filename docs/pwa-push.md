# Aplikacja (PWA) i powiadomienia push

Strona instaluje się jak aplikacja na Androidzie, iOS i komputerze, a zalogowani klienci i administratorzy
mogą dostawać powiadomienia web push.

## Instalacja jako aplikacja

- `web/static/manifest.json` – nazwa, kolory, ikony (PNG 192/512, maskable) i skróty (Moje zamówienia, Koszyk).
- `web/static/sw.js` – service worker, serwowany spod `/sw.js` (zasięg: cała strona). Nie cache'uje stron
  (koszyk, sesje, zamówienia); przy braku sieci pokazuje `web/static/offline.html`.
- `web/static/pwa.js` – rejestruje service worker i obsługuje instalację:
  - Android / Chrome / Edge: przechwytuje `beforeinstallprompt`; przycisk „Zainstaluj aplikację” w stopce,
    w panelach i dolny baner (tylko telefon, można odrzucić na 30 dni).
  - iOS (Safari): brak API instalacji, więc pokazuje instrukcję „Udostępnij → Do ekranu początkowego”.
- Ikony generuje `scripts/generate-icons.sh` (wymaga `rsvg-convert`) z logo roweru; wyniki są w `web/static/icons/`.

## Powiadomienia push

Przełącznik „Powiadomienia” jest w panelu klienta (`/user`) i administratora (`/admin`). Każde urządzenie
zapisuje się osobno w tabeli `push_subscriptions` (baza read models). Na iPhonie/iPadzie (iOS 16.4+) działa to
dopiero po dodaniu strony do ekranu początkowego.

| Zdarzenie | Klient | Admin |
|---|---|---|
| Nowe zamówienie (płatność z góry) | – | tak |
| Nowe zamówienie z odbiorem osobistym | – | tak (przy składaniu, jak e-mail) |
| Płatność przyjęta | tak (poza gotówką potwierdzaną przez klienta) | tylko przelew z banku |
| Zamówienie potwierdzone / zrealizowane / anulowane | tak | – |

Powiadomienia wysyła `Server.observeProjectedEvent` (`cmd/server/push.go`), wywoływane przez projektor po
zaktualizowaniu read models, więc obejmują każde źródło zmiany (panel admina, worker poczty bankowej, klient).
Zdarzenia starsze niż 30 minut są pomijane (np. przy odtwarzaniu zdarzeń).

### Konfiguracja

Klucze VAPID: `VAPID_PRIVATE_KEY`, `VAPID_PUBLIC_KEY`, `VAPID_SUBJECT` (sekrety GitHub ustawia
`scripts/setup-vapid-secrets.sh`). Bez nich serwer generuje parę kluczy raz i trzyma ją w bazie
(`business_settings`) – wystarczy na development. **Zmiana kluczy unieważnia istniejące subskrypcje**
(użytkownicy muszą włączyć powiadomienia ponownie).

### Bezpieczeństwo

- Endpointy `/api/webpush/*` wymagają zalogowania (401 bez sesji).
- Serwer wysyła żądania pod adres podany przez klienta, więc przyjmuje tylko `https` i hosty znanych usług push
  (FCM, Mozilla, Apple, Windows) – ochrona przed SSRF.
- Jedno konto może mieć do 10 urządzeń; urządzenie przechodzi na konto aktualnie zalogowanego, a przy wylogowaniu
  jest odpinane, żeby powiadomienia admina nie trafiały do następnej osoby na tym telefonie.
- Subskrypcje wygasłe (HTTP 404/410) są usuwane automatycznie.
