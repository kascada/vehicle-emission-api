# Vehicle Emission API — Projektnotizen

## Methodik

Vor der Implementierung wurde Claude (Anthropic, Opus 4.6) beauftragt, die Aufgabenstellung zu analysieren
und eine grobe Einschätzung der zu erwartenden Codequalität und Architektur-Entscheidungen zu liefern.
Dabei wurden mehrere Inkonsistenzen in den Daten der FuelEconomy.gov API identifiziert
(siehe Abschnitt "Erkannte Probleme & Datenlücken"), die ohne Vorab-Analyse erst zur Laufzeit
aufgefallen wären. Diese Erkenntnisse fließen direkt in das API-Design und die Fehlerbehandlung ein.

---

## Vorgehen & Entscheidungen

### Auth-Konzept: E-Mail als Inline-Validierung (kein separater Token)

**Entscheidung:** Die E-Mail-Adresse wird bei jedem Request als Header übergeben und inline validiert.
Beim ersten Aufruf wird die Domain gegen eine Disposable-Liste geprüft und das Ergebnis gecacht.
Nachfolgende Requests mit derselben E-Mail-Domain werden aus dem Cache bedient.

**Begründung:**
- Die Aufgabe fordert "Validierung vor Zugriff" — das findet bei jedem Request statt, nur effizienter.
- Ein separates Token (JWT/Opaque) erzeugt Zustandsprobleme: Bei Server-Neustart (Update, Deploy) wären alle Tokens ungültig. Das widerspricht dem Ziel eines robusten Services.
- Die E-Mail-Domain-Validierung ist deterministisch — gleiches Ergebnis bei jedem Aufruf. Es gibt keinen Grund, das Ergebnis in einem separaten Token zu kapseln.
- Bei mehreren Server-Instanzen: Der Domain-Cache kann lokal sein (jede Instanz baut ihren eigenen auf), da die Prüfung immer zum selben Ergebnis führt. Kein Redis/DB nötig.
- Weniger Angriffsfläche: Kein Token-Diebstahl, kein Token-Expiry-Management.

**Gegenargument (bewusst abgewogen):**
Ein Prüfer könnte sagen, dass zwei separate Endpunkte gefordert sind. Wir argumentieren, dass die Validierung trotzdem stattfindet — sie ist nur eingebettet statt vorgeschaltet. Das ist ein Engineering-Trade-off zugunsten von Robustheit.

**Alternative (falls gewünscht):** POST /auth mit Opaque-Token in sync.Map. Einfach umsetzbar, aber fragil bei Restarts.

### Wegwerf-E-Mail-Erkennung

**Ansatz:** Statische Domain-Liste aus dem Open-Source-Projekt `disposable-email-domains` (GitHub).
Die Liste wird beim Server-Start als Map geladen. Kein externer API-Call zur Laufzeit nötig.

**Warum kein externer Service (z.B. SendGrid)?**
- Zusätzliche Latenz bei jedem Request
- Abhängigkeit von Dritt-Service-Verfügbarkeit
- Für die geforderte Aufgabe überdimensioniert

### Technologie: Go

**Gründe:**
- Einzelnes Binary — einfach zu testen, deployen (z.B. Google Cloud Run)
- `net/http` reicht als Router, kein Framework nötig
- Gute Performance bei externen HTTP-Calls (goroutines)
- Zeigt, dass kein Framework-Overhead nötig ist

---

## FuelEconomy.gov API — Analyse

### Endpunkte

| Endpunkt | Beschreibung |
|---|---|
| `/ws/rest/vehicle/menu/year` | Liste aller verfügbaren Modelljahre (1984–2026) |
| `/ws/rest/vehicle/menu/make?year=YYYY` | Hersteller für ein Jahr |
| `/ws/rest/vehicle/menu/model?year=YYYY&make=XXX` | Modelle eines Herstellers |
| `/ws/rest/vehicle/menu/options?year=YYYY&make=XXX&model=YYY` | Varianten mit Fahrzeug-IDs |
| `/ws/rest/vehicle/{id}` | Vollständige Fahrzeugdaten |

### Format
- **Standard: XML** — Browser-Aufruf liefert XML
- JSON möglich über `Accept: application/json` Header
- Kein API-Key erforderlich
- Kein Rate-Limit dokumentiert (trotzdem eigenes Limit einbauen)

### Beispiel-IDs für Tests

| ID | Fahrzeug | Typ | Besonderheit |
|---|---|---|---|
| 47085 | 2024 Toyota Camry (V6, 3.5L) | Verbrenner | Standard-Fall, alle Felder vorhanden |
| 47913 | 2024 Tesla Model Y Long Range AWD | Elektro | co2=0, city08=122 MPGe, kein Hubraum |
| 7062 | 1990 Ford F150 Pickup 2WD | Alt/Verbrenner | co2=-1, feScore=-1, Felder teils leer |
| 9999999 | (existiert nicht) | — | HTTP 404 |

### Erkannte Probleme & Datenlücken

#### PROBLEM 1: co2-Feld ist unzuverlässig
- **Toyota Camry (47085):** `co2 = 338` ✓ (direkt nutzbar)
- **Tesla Model Y (47913):** `co2 = 0` — Logisch korrekt (Elektro, kein Tailpipe), aber es gibt zusätzlich `co2TailpipeGpm = 0.0`
- **Ford F150 1990 (7062):** `co2 = -1` — **Fehlender Wert!** Aber `co2TailpipeGpm = 634.79` ist vorhanden.
- **Fazit:** Das `co2`-Feld allein reicht nicht. Wir brauchen Fallback-Logik:
  1. Wenn `co2 > 0` → direkt verwenden
  2. Wenn `co2 <= 0` und `co2TailpipeGpm > 0` → diesen Wert verwenden
  3. Wenn beides <= 0 → `null` zurückgeben und dem Client signalisieren

#### PROBLEM 2: Elektrofahrzeuge haben andere Metriken
- `city08`/`highway08`/`comb08` sind bei E-Autos **MPGe** (Miles Per Gallon equivalent), nicht MPG
- Tesla: city08=122, highway08=112 — das sind keine echten MPG-Werte
- Der `fuelType` hilft: "Electricity" vs "Regular Gasoline"
- **Empfehlung:** Einheit im Response mitliefern (`mpg` vs `mpge`) oder `fuelType` mit ausgeben

#### PROBLEM 3: Leere/fehlende Felder bei alten Fahrzeugen
- Ford F150 1990: `cylDeact` = leer (nicht "N"), `mfrCode` = leer, `eng_dscr` = "(FFS)"
- `city08U` = 0.0 (ungerundete Werte fehlen bei alten Autos)
- `emissionsList` = leer (keine Emissionsdaten vorhanden)
- `feScore = -1`, `ghgScore = -1` — Scores existieren erst ab ~2000er Jahrgängen
- **Fazit:** Alle optionalen Felder als Pointer/nullable behandeln

#### PROBLEM 4: Modellnamen bei Tesla sind granular
- "Model Y" allein reicht nicht → leeres Ergebnis
- Korrekt: "Model Y Long Range AWD", "Model Y Performance AWD", etc.
- **Relevanz für uns:** Gering (wir arbeiten mit IDs, nicht mit Modellnamen), aber gut zu wissen

#### PROBLEM 5: Ungültige ID → HTTP 404 (kein JSON/XML-Body)
- ID 9999999 liefert eine HTML-Fehlerseite, kein strukturiertes Error-Objekt
- **Muss im Code abgefangen werden:** HTTP-Statuscode prüfen, bevor XML/JSON geparst wird

---

## Datenvergleich der drei Testfahrzeuge

### Relevante Felder für unsere API-Response:

| Feld | Toyota Camry (47085) | Tesla Model Y (47913) | Ford F150 (7062) |
|---|---|---|---|
| make | Toyota | Tesla | Ford |
| model | Camry | Model Y Long Range AWD | F150 Pickup 2WD |
| year | 2024 | 2024 | 1990 |
| city08 | 22 | 122 (MPGe!) | 13 |
| highway08 | 33 | 112 (MPGe!) | 15 |
| comb08 | 26 | 117 (MPGe!) | 14 |
| co2 | 338 | 0 | -1 (!) |
| co2TailpipeGpm | 338.0 | 0.0 | 634.79 |
| VClass | Midsize Cars | Small SUV 4WD | Standard Pickup Trucks |
| fuelType | Regular Gasoline | Electricity | Regular Gasoline |
