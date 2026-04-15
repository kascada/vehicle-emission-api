# Gedankengänge

## Analyse der Datenquelle

Die geforderte API ist recht einfach, daher lasse ich als erstes die Datenquelle und die API von
https://www.fueleconomy.gov/feg/ws/ von Claude anschauen und analysieren.

Sie ist tatsächlich auch einfach und funktional. Habe Claude gebeten, etwas tiefer zu schauen und einige Beispiele auszuprobieren. Dabei sind ein paar Inkonsistenzen aufgetaucht, die in eine separate Datei notiert wurden.


## Design-Entscheidungen

### Authentifizierung

Die entscheidende Frage ist das Design der API, und dabei ist eigentlich nur gegeben, dass zuerst die
E-Mail-Adresse geprüft werden soll.
Naheliegend ist es, nach erfolgreicher Prüfung ein Token zu übergeben, das jedesmal zur Authentifizierung mitgegeben werden muss.

Das Handling ist zwar einfach, aber für diesen Fall führt es für den Anwender zu unnötigem Aufwand.
Zusätzlich müsste das Token und die E-Mail in einem Cache gehalten werden. Startet das Programm aber neu,
wäre es weg und der Anwender würde eine Fehlermeldung bekommen — das ist inakzeptabel, weswegen wir eine
externe Datenbank (Key-Value) bräuchten. Diese zusätzliche Komplexität lässt sich leicht umgehen:

Wir prüfen beim ersten Aufruf die E-Mail und legen das Ergebnis in den Cache.
Nur bei einem Neustart oder Ablauf des Caches wird neu geprüft.

Damit haben wir das optimale Ergebnis:

- Als erstes wird die E-Mail geprüft, wie gefordert
- Der User braucht kein Token
- Das Programm kann jederzeit neu gestartet werden
- Keine zusätzliche Datenbank nötig
- Deutlich effizienter als die Token-Variante

### Technologie-Stack

Benötigt wird nur ein Webserver für den API-Endpoint.
Daher werde ich es in Go schreiben, so dass keine weiteren Libraries nötig sind.


---

## Umsetzung

### Projektsetup

Verzeichnis anlegen, Git vorbereiten, Go-Basis erstellen.

Für Claude grundsätzliches Vorgehen vorgeben, etwas Schritt für Schritt:
Alles in PROMPT.md

1. Erstellung des `main` für CLI
2. Alle Funktionen werden der Reihe nach als CLI-Funktion jeweils mit Unit-Tests erstellt
3. API-Integration zum Schluss

### Datenabruf

Als erstes der Abruf der Daten, CLI gibt es lesbar aus.

```
go run main.go vehicle -text -verbose 47913
```


Ausgabe lesbar, Inkonsistenzen werden erkannt.

Unit-Test integriert


### Authentifizierung

Statt einem Token wird die E-Mail-Adresse geprüft und wenn positiv in einen flüchtigen Cache geschrieben.
Der Cache hat pro Eintrag eine Verfallszeit.

```
go run main.go check-email test@gmail.com
```

### API

*(in Arbeit)*

---

## Offene Punkte

- Cache-Größe und Verfall prüfen
- Prüfen, ob alle Vorgaben erfüllt
 ohne Netzwerkverbindung:

```
go test -v ./...
```

### E-Mail-Verifikation

Domains der Wegwerf-Adressen werden einmalig geladen und können jederzeit aktualisiert werden.
Diese werden per `embed` direkt in den Code kompiliert, daher kein Container o. Ä. nötig.
Download bei Programmstart wäre eine Alternative, aber die Liste ändert sich zu selten —
würde den Start nur unnötig verzögern.

Parsen der E-Mail, Syntax wird geprüft und mit der Liste abgeglichen.

Verifikation per CLI:

```
go run main.go validate-email notvalid
go run main.go validate-email wegwerf@10mail.org
```

Unit-Test erweitert, wird automatisch nach dem Build ausgeführt.




### Authentifizierung

Statt einem Token wird die E-Mail-Adresse geprüft und wenn positiv in einen flüchtigen Cache geschrieben.
Der Cache hat pro Eintrag eine Verfallszeit.

```
go run main.go check-email test@gmail.com
```

### API als CLI-Test

Für die API haben wir zwei Aufrufarten:

/vehicle/47085?email=user@gmail.com
 Übergabe der email per POST oder GET

Sicherer ist natürlich:
curl -H "email: user@gmail.com" http://localhost:8080/vehicle/47085

API-Test per CLI eingebaut:
go run main.go fetch -verbose -text 47085 user@gmail.com


### API 

API mit Port 8081 oder Parameter -port aufrufbar

go run main.go serve -verbose

TTL vom Cache wird alle 10min geprüft und altes entfernt, das älter als 6h ist


## Cache für Daten

Die Abfargen werden gecached, so kann eine eventuelle Nichterreichbarkeit der Datenquelle eventuell abgefangen werden

## Deployment auf meinen Server

Build mit für binary, Deployment auf meinen Server, Subdomain eingerichtet, SSL aktiviert.
Proxy für Apache. 

Wenn fehlerhafter Aufruf, wird eine 404 generiert mit Hinweis:
 {"error":"not found","usage":"GET /vehicle/{id}?email=user@example.com"}



### Ergebnis

Cache auch für Anfragen

---

## Offene Punkte

- Prüfen, ob alle Vorgaben erfüllt

- short -Guard
- Testen, wenn Quelle nicht erreichbar
- Geschwindigkeit testen