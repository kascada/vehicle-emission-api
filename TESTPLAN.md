# Unit-Test-Plan — Vehicle Emission API

## Übersicht

Tests sind in drei Bereiche gegliedert:
1. E-Mail-Validierung & Auth
2. FuelEconomy-API-Integration
3. HTTP-Endpunkte (Integration Tests)

---

## 1. E-Mail-Validierung

### 1.1 Gültige E-Mail-Adressen (erwarte: OK)
- `user@gmail.com` — Standard
- `user.name@company.de` — Punkt im Local-Part, deutsche TLD
- `user+tag@example.com` — Plus-Adressierung
- `user@subdomain.example.com` — Subdomain

### 1.2 Formal ungültige E-Mail-Adressen (erwarte: Fehler 400)
- `""` — Leer
- `"user"` — Kein @-Zeichen
- `"@domain.com"` — Kein Local-Part
- `"user@"` — Keine Domain
- `"user@.com"` — Domain beginnt mit Punkt
- `"user space@domain.com"` — Leerzeichen
- `"user@domain"` — Keine TLD (je nach Strenge)

### 1.3 Wegwerf-E-Mail-Adressen (erwarte: Fehler 403 oder 422)
- `user@mailinator.com`
- `user@guerrillamail.com`
- `user@tempmail.com`
- `user@throwaway.email`
- `user@yopmail.com`

### 1.4 Grenzfälle
- `user@MAILINATOR.COM` — Groß-/Kleinschreibung (muss trotzdem blockiert werden!)
- `user@subdomain.mailinator.com` — Subdomain einer Wegwerf-Domain
- `user@gmail.co` — Ähnlich wie bekannte Domain, aber gültig
- Sehr lange E-Mail-Adresse (254 Zeichen max laut RFC)

---

## 2. FuelEconomy-API-Integration

### 2.1 Erfolgreicher Abruf — Verbrenner (ID: 47085, Toyota Camry 2024)
- Erwarte: make="Toyota", model="Camry", year=2024
- Erwarte: city08=22, highway08=33, comb08=26
- Erwarte: co2=338 (oder Fallback co2TailpipeGpm=338.0)
- Erwarte: VClass="Midsize Cars"

### 2.2 Erfolgreicher Abruf — Elektrofahrzeug (ID: 47913, Tesla Model Y 2024)
- Erwarte: make="Tesla", model="Model Y Long Range AWD", year=2024
- Erwarte: city08=122, highway08=112, comb08=117 (MPGe-Werte!)
- Erwarte: co2=0 (kein Tailpipe-Ausstoß)
- Erwarte: fuelType="Electricity"
- **Prüfe:** Wird dem Client signalisiert, dass es sich um MPGe handelt?

### 2.3 Erfolgreicher Abruf — Altes Fahrzeug mit Datenlücken (ID: 7062, Ford F150 1990)
- Erwarte: make="Ford", model="F150 Pickup 2WD", year=1990
- Erwarte: city08=13, highway08=15, comb08=14
- **Erwarte: co2=-1 im Rohdatensatz → muss als null/nicht verfügbar behandelt werden**
- Prüfe: co2TailpipeGpm=634.79 als Fallback?
- Erwarte: VClass="Standard Pickup Trucks"

### 2.4 Ungültige Fahrzeug-ID (ID: 9999999)
- Erwarte: HTTP 404 von der Upstream-API
- Unser Service muss: sauberen JSON-Error zurückgeben (nicht die HTML-Seite durchreichen!)
- Erwarte: `{"error": "Vehicle not found", "id": 9999999}` oder ähnlich

### 2.5 Ungültiges ID-Format
- `/vehicle/abc` — String statt Zahl
- `/vehicle/-1` — Negative Zahl
- `/vehicle/0` — Null
- Erwarte: HTTP 400 Bad Request, bevor überhaupt die externe API aufgerufen wird

### 2.6 Externe API nicht erreichbar (Mock)
- Simuliere: Timeout der FuelEconomy-API
- Simuliere: 500 Internal Server Error von der API
- Erwarte: Unser Service gibt HTTP 502 Bad Gateway oder 503 zurück
- Erwarte: Sinnvolle Fehlermeldung, kein Stacktrace

---

## 3. HTTP-Endpunkte (Integration)

### 3.1 Zugriff ohne Auth
- `GET /vehicle/47085` ohne E-Mail-Header
- Erwarte: HTTP 401 Unauthorized

### 3.2 Zugriff mit ungültiger E-Mail
- `GET /vehicle/47085` mit `X-Email: not-an-email`
- Erwarte: HTTP 400 Bad Request

### 3.3 Zugriff mit Wegwerf-E-Mail
- `GET /vehicle/47085` mit `X-Email: user@mailinator.com`
- Erwarte: HTTP 403 Forbidden

### 3.4 Zugriff mit gültiger E-Mail
- `GET /vehicle/47085` mit `X-Email: user@gmail.com`
- Erwarte: HTTP 200 mit korrektem JSON-Body

### 3.5 Response-Format prüfen
- Erwarte exakt diese Felder im JSON:
```json
{
  "make": "Toyota",
  "model": "Camry",
  "year": 2024,
  "city08": 22,
  "highway08": 33,
  "comb08": 26,
  "co2": 338,
  "vclass": "Midsize Cars",
  "fuelType": "Regular Gasoline"
}
```
- Keine zusätzlichen Felder (kein Data-Leak aus der Upstream-API)

### 3.6 Rate Limiting
- 100+ Requests in schneller Folge von derselben IP/E-Mail
- Erwarte: HTTP 429 Too Many Requests ab Schwellenwert

### 3.7 Content-Type
- Erwarte: `Content-Type: application/json` in allen Responses
- Erwarte: Auch Error-Responses sind JSON (nie HTML/Text)

---

## 4. Caching-Verhalten

### 4.1 Wiederholter Abruf derselben ID
- Erster Aufruf: Externe API wird aufgerufen
- Zweiter Aufruf (innerhalb TTL): Aus Cache, kein externer Call
- **Wie testen:** Mock der externen API, Zähler auf Aufrufe prüfen

### 4.2 Cache-Invalidierung
- Nach TTL-Ablauf: Externer Call findet wieder statt
- Cache darf nicht unbegrenzt wachsen (Memory-Leak-Prüfung)

---

## 5. Testinfrastruktur

### Go-spezifisch:
- `net/http/httptest` für HTTP-Tests
- Interface für den FuelEconomy-Client → Mock in Tests
- `testing.T` mit Table-Driven Tests für E-Mail-Validierung
- `t.Parallel()` wo möglich

### Mocks:
- FuelEconomy-API: `httptest.NewServer` mit vorgefertigten XML-Responses
- Disposable-Liste: Kleine Test-Liste statt der vollständigen

### Beispiel-Testdaten (als Dateien):
- `testdata/vehicle_47085.xml` — Toyota Camry Response
- `testdata/vehicle_47913.xml` — Tesla Model Y Response
- `testdata/vehicle_7062.xml` — Ford F150 Response
- `testdata/vehicle_404.html` — 404-Response für ungültige ID
