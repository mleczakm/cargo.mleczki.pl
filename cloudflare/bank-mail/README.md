# Bank mail Worker

Wspólny kod i testy są synchronizowane z
`infrastructure-mikrus/integrations/bank-mail-worker` do wersjonowanego `shared/`.
Uruchom w repozytorium infrastruktury
`node integrations/bank-mail-worker/sync.mjs --write --test --project cargo`.
`--check` wykrywa rozbieżności; skrypt chroni lokalne zmiany przed nadpisaniem.

`npm run test:js` uruchamia dotychczasowe testy i kontrakt w rzeczywistym workerd;
CI wykonuje tę komendę. Format JSON, `X-Cargo-Bank-*`, czas ISO i kolejka
pozostają zgodne z odbiornikiem. Przekierowania nie przenoszą powiadomień na
inny adres; nieudane dostarczenie wraca do kolejki.
