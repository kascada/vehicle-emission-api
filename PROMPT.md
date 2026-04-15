# Projekt-Prompt: Vehicle Emission API

## Aufgabe

Implementiere einen Go-Backend-Service mit zwei logischen Funktionen:
1. E-Mail-Validierung (formal + Wegwerf-Check) als Auth-Gate
2. Fahrzeug-Emissionsdaten von FuelEconomy.gov abrufen und aufbereitet zurückgeben

Der Code liegt bereits als Skeleton vor (main.go, handler/, validator/, client/, testdata/).
Die Architektur-Entscheidungen und API-Analyse sind in NOTES.md dokumentiert, der Testplan in TESTPLAN.md.

---

## Architektur-Entscheidungen (bereits getroffen)

### Auth: E-Mail als Inline-Validierung, kein separates Token

Die E-Mail wird als `X-Email`-Header bei jedem Request mitgeschickt und inline validiert.
Kein POST /auth, kein JWT, kein Opaque-Token.

Begründung:
- Die Validierung ist deterministisch (gleiche Domain → gleiches Ergebnis). Ein Token kapselt keinen Mehrwert.
- Token in sync.Map sind fragil bei Server-Neustarts (Deploy, Update → alle Sessions ungültig).
- Kein Token-Diebstahl, kein Expiry-Management, keine verteilte Session-Speicherung nötig.
- Die Domain-Prüfung wird gecacht (einmal pro Domain, nicht pro Request).

### Wegwerf-E-Mail-Erkennung

Statische Domain-Liste aus Datei (disposable_domains.txt), geladen beim Server-Start als Map.
Subdomain-Erkennung eingebaut: sub.mailinator.com wird ebenfalls erkannt.
Case-insensitive (alles lowercase).

### Technologie

Go mit net/http (Go 1.22+ für Routing-Pattern `GET /vehicle/{id}`).
Keine externen Dependencies. Einzelnes Binary.

---

## API-Design

### Endpunkt

```
GET /vehicle/{id}
Header: X-Email: user@example.com
```

### Response (200 OK)

```json
{
  "make": "Toyota",
  "model": "Camry",
  "year": 2024,
  "city08": 22,
  "highway08": 33,
  "comb08": 26,
  "co2": 338.0,
  "vclass": "Midsize Cars",
  "fuelType": "Regular Gasoline"
}
```

### Fehlerfälle

| Status | Bedingung |
|---|---|
| 400 | E-Mail formal ungültig, oder ID kein positiver Integer |
| 401 | X-Email-Header fehlt |
| 403 | Wegwerf-E-Mail-Adresse |
| 404 | Fahrzeug-ID existiert nicht bei FuelEconomy.gov |
| 429 | Rate Limit überschritten |
| 502 | FuelEconomy.gov nicht erreichbar oder Fehler |

Alle Error-Responses als JSON: `{"error": "beschreibung"}`

---

## FuelEconomy.gov API — Erkenntnisse aus Vorab-Analyse

Upstream-Endpunkt: `https://www.fueleconomy.gov/ws/rest/vehicle/{id}`
Header `Accept: application/json` setzen, sonst kommt XML.
Kein API-Key nötig.

### Fünf bekannte Probleme in den Daten

1. **co2-Feld unzuverlässig:** Bei alten Fahrzeugen (z.B. Ford F150 1990, ID 7062) ist `co2 = -1`. Fallback auf `co2TailpipeGpm` nötig. Bei E-Autos ist `co2 = 0` (korrekt). Logik:
   - co2 > 0 → verwenden
   - co2 <= 0 && co2TailpipeGpm > 0 → Fallback
   - beides <= 0 → null im JSON

2. **E-Autos liefern MPGe statt MPG:** city08/highway08/comb08 sind bei Electricity-Fahrzeugen "Miles Per Gallon equivalent". Erkennbar an fuelType1 == "Electricity". Wir geben fuelType mit aus, damit der Client unterscheiden kann.

3. **Alte Fahrzeuge haben viele leere Felder:** Scores, Emissionslisten, ungerundete Werte fehlen bei Autos vor ~2000.

4. **Ungültige ID → HTML 404:** Die API gibt kein JSON zurück, sondern eine HTML-Fehlerseite. HTTP-Statuscode prüfen bevor JSON geparst wird.

5. **Modellnamen bei Tesla sehr granular:** "Model Y" allein liefert leere Ergebnisse. Für uns irrelevant (wir arbeiten mit IDs), aber gut zu wissen.

### Test-IDs

| ID | Fahrzeug | Besonderheit |
|---|---|---|
| 47085 | 2024 Toyota Camry V6 3.5L | Standardfall, alle Daten vorhanden |
| 47913 | 2024 Tesla Model Y LR AWD | E-Auto: co2=0, MPGe-Werte, kein Hubraum |
| 7062 | 1990 Ford F150 Pickup 2WD | Alt: co2=-1, co2TailpipeGpm=634.79 |
| 9999999 | (existiert nicht) | HTTP 404, HTML-Response |

JSON-Fixtures dieser Fahrzeuge liegen in testdata/.

---

## Noch zu implementieren

- [ ] Unit-Tests (validator/, client/, handler/) gemäß TESTPLAN.md
- [ ] Rate Limiting (einfaches In-Memory-Limit pro IP oder E-Mail)
- [ ] Disposable-Liste erweitern (vollständige Liste von https://github.com/disposable-email-domains/disposable-email-domains einbinden)
- [ ] Dockerfile (optional, für Cloud Run Deploy)
- [ ] go.mod Modulname anpassen wenn echtes Repo steht

---

## Projektstruktur

```
CodingChallenge/
├── main.go                        # Server-Setup, Dependency-Wiring
├── go.mod
├── disposable_domains.txt         # Wegwerf-Domains (eine pro Zeile)
├── .gitignore
├── NOTES.md                       # Architektur-Entscheidungen & API-Analyse
├── TESTPLAN.md                    # Detaillierter Testplan
├── PROMPT.md                      # Dieser Prompt
├── handler/
│   └── vehicle.go                 # HTTP-Handler mit Auth-Prüfung
├── validator/
│   ├── email.go                   # RFC-5322-Validierung
│   └── disposable.go              # Domain-Blocklist mit Subdomain-Check
├── client/
│   └── fueleconomy.go             # API-Client mit Cache und CO2-Fallback
└── testdata/
    ├── vehicle_47085.json         # Toyota Camry (Standardfall)
    ├── vehicle_47913.json         # Tesla Model Y (E-Auto)
    └── vehicle_7062.json          # Ford F150 1990 (Datenlücken)
```
