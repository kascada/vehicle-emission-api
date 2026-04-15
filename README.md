# Vehicle Emission API

CLI-Tool zum Abrufen von Fahrzeug- und Emissionsdaten über die [fueleconomy.gov API](https://www.fueleconomy.gov/feg/ws/index.shtml).

## Voraussetzungen

- Go 1.22+
- Keine externen Dependencies

## Befehle

### `vehicle` — Fahrzeugdaten abrufen

```bash
go run main.go vehicle <id>
go run main.go vehicle -text <id>
```

| Flag    | Beschreibung                        |
|---------|-------------------------------------|
| `-text` | Lesbare Textausgabe statt rohem JSON |

**Beispiele:**

```bash
# JSON-Ausgabe (Rohdaten)
go run main.go vehicle 47085

# Lesbare Textausgabe
go run main.go vehicle -text 47085
```

**Beispiel-Ausgabe (`-text`):**

```
Fahrzeug:       2024 Toyota Camry
Klasse:         Midsize Cars
Antrieb:        Front-Wheel Drive, 3.5 L, 6 Zyl.
Getriebe:       Automatic (S8)
Verbrauch:      Stadt 22 / Autobahn 33 / Kombi 26 mpg
CO₂:            338.0 g/mi
Umwelt-Score:   5/10
```

### `validate-email` — E-Mail-Adresse prüfen

Validiert Format und prüft gegen eine eingebettete Liste von ~5.300 Wegwerf-Domains.

```bash
go run main.go validate-email <email>
```

| Ergebnis           | Ausgabe (Kanal)                                    | Exit |
|--------------------|----------------------------------------------------|------|
| Gültig             | `OK: user@example.com` (stdout)                    | 0    |
| Ungültiges Format  | `fehler: invalid email format: ...` (stderr)       | 1    |
| Wegwerf-Domain     | `BLOCKED: disposable email (domain: mailinator.com)` (stderr) | 1 |
| Kein Argument      | `usage: validate-email <email>` (stderr)           | 1    |

**Beispiele:**

```bash
go run main.go validate-email user@example.com
# OK: user@example.com

go run main.go validate-email user@mailinator.com
# BLOCKED: disposable email (domain: mailinator.com)

go run main.go validate-email notanemail
# fehler: invalid email format: mail: missing '@' or angle-addr
```

### `check-email` — E-Mail prüfen (mit Cache)

Wie `validate-email`, speichert gültige Adressen jedoch in einem In-Memory-Cache (TTL 1 Stunde). Bereits geprüfte Adressen werden ohne erneute Validierung direkt bestätigt.

```bash
go run main.go check-email <email>
```

| Ergebnis               | Ausgabe (Kanal)                             | Exit |
|------------------------|---------------------------------------------|------|
| Gültig (neu)           | `OK: user@example.com (added to cache)` (stdout) | 0 |
| Gültig (gecacht)       | `CACHED: user@example.com` (stdout)         | 0    |
| Ungültiges Format      | `fehler: invalid email format: ...` (stderr) | 1   |
| Wegwerf-Domain         | `BLOCKED: disposable email (domain: ...)` (stderr) | 1 |

> Hinweis: Der Cache lebt nur für die Dauer des Prozesses. Bei jedem neuen `go run` startet er leer.

## API-Endpunkt (HTTP-Server)

```
GET /vehicle/{id}
```

Die E-Mail-Adresse zur Authentifizierung kann auf zwei Arten übergeben werden:

**Variante 1 — Header (empfohlen für Produktion):**

```bash
curl -H "Email: user@gmail.com" http://localhost:8080/vehicle/47085
```

Die E-Mail ist nicht in der URL sichtbar — keine Logs, keine Browser-History, kein Referrer-Leak.

**Variante 2 — Query-Parameter (zum Testen):**

```bash
curl http://localhost:8080/vehicle/47085?email=user@gmail.com
```

Einfacher zum Testen im Browser oder auf der Kommandozeile. Die E-Mail landet allerdings in Server-Logs und Browser-History — daher nur für die Entwicklung gedacht.

Header hat Vorrang. Wenn beide angegeben sind, wird der Header verwendet.

## Fehlerbehandlung

| Situation              | Verhalten                                          |
|------------------------|----------------------------------------------------|
| Fehlende ID            | Fehlermeldung + Exit 1                             |
| Unbekannte Fahrzeug-ID | `Fahrzeug mit ID "x" nicht gefunden` + Exit 1      |
| Netzwerkfehler         | Fehlermeldung + Exit 1                             |
| Unbekannter Befehl     | usage + Exit 1                                     |
