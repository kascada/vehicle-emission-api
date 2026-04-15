package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/kamran/vehicle-emission-api/cache"
	"github.com/kamran/vehicle-emission-api/client"
	"github.com/kamran/vehicle-emission-api/handler"
	"github.com/kamran/vehicle-emission-api/validator"
)

var (
	baseURL    = "https://www.fueleconomy.gov/ws/rest"
	httpClient = &http.Client{Timeout: 10 * time.Second}
	verbose    bool
	logWriter  io.Writer = os.Stderr
)

func logf(format string, args ...any) {
	if verbose {
		fmt.Fprintf(logWriter, "[verbose] "+format+"\n", args...)
	}
}

func warnf(format string, args ...any) {
	if verbose {
		fmt.Fprintf(logWriter, "[warn]    "+format+"\n", args...)
	}
}

// Vehicle enthält die relevanten Felder aus der fueleconomy.gov API.
// Die API liefert alle Felder als JSON-Strings, auch numerische Werte.
type Vehicle struct {
	ID        string `json:"id"`
	Make      string `json:"make"`
	Model     string `json:"model"`
	Year      string `json:"year"`
	Class     string `json:"VClass"`
	Drive     string `json:"drive"`
	Trany     string `json:"trany"`
	Cylinders string `json:"cylinders"`
	Displ     string `json:"displ"`
	FuelType  string `json:"fuelType1"`
	City      string `json:"city08"`
	Highway   string `json:"highway08"`
	Combined  string `json:"comb08"`
	CO2Raw    string `json:"co2"`
	CO2Pipe   string `json:"co2TailpipeGpm"`
	GHGScore  string `json:"ghgScore"`
	FEScore   string `json:"feScore"`
	AtvType   string `json:"atvType"`
}

// co2 gibt den besten verfügbaren CO₂-Wert zurück (P1: Fallback-Logik).
func (v Vehicle) co2() (value string, source string) {
	if f, err := strconv.ParseFloat(v.CO2Raw, 64); err == nil && f > 0 {
		return v.CO2Raw, "co2"
	}
	if f, err := strconv.ParseFloat(v.CO2Pipe, 64); err == nil && f > 0 {
		return v.CO2Pipe, "co2TailpipeGpm"
	}
	return "", ""
}

// fuelUnit gibt "MPGe" für Elektro/Plug-in zurück, sonst "MPG" (P2).
func (v Vehicle) fuelUnit() string {
	ft := strings.ToLower(v.FuelType)
	if strings.Contains(ft, "electricity") || v.AtvType == "EV" || v.AtvType == "PHEV" {
		return "MPGe"
	}
	return "MPG"
}

func (v Vehicle) FormatText() string {
	co2val, _ := v.co2()
	if co2val == "" {
		co2val = "n/a"
	}

	ghg := v.GHGScore
	if ghg == "-1" || ghg == "" {
		ghg = "n/a"
	}

	return fmt.Sprintf(
		"%-15s %s %s %s\n"+
			"%-15s %s\n"+
			"%-15s %s, %s L, %s Zyl.\n"+
			"%-15s %s\n"+
			"%-15s Stadt %s / Autobahn %s / Kombi %s %s\n"+
			"%-15s %s g/mi\n"+
			"%-15s %s/10\n",
		"Fahrzeug:", v.Year, v.Make, v.Model,
		"Klasse:", v.Class,
		"Antrieb:", v.Drive, v.Displ, v.Cylinders,
		"Getriebe:", v.Trany,
		"Verbrauch:", v.City, v.Highway, v.Combined, v.fuelUnit(),
		"CO₂:", co2val,
		"Umwelt-Score:", ghg,
	)
}

// checkDataQuality loggt bekannte und generische Datenprobleme (nur bei -verbose).
func checkDataQuality(v Vehicle) {
	// P1: CO₂-Fallback
	_, src := v.co2()
	switch src {
	case "":
		warnf("P1 CO₂: kein verwertbarer Wert (co2=%q, co2TailpipeGpm=%q) → wird als n/a ausgegeben", v.CO2Raw, v.CO2Pipe)
	case "co2TailpipeGpm":
		warnf("P1 CO₂: primäres Feld co2=%q unbrauchbar → Fallback auf co2TailpipeGpm=%q", v.CO2Raw, v.CO2Pipe)
	default:
		logf("P1 CO₂: ok (Quelle: %s = %s g/mi)", src, v.CO2Raw)
	}

	// P2: Einheit MPG vs MPGe
	unit := v.fuelUnit()
	if unit == "MPGe" {
		warnf("P2 Einheit: fuelType=%q → Verbrauchswerte sind %s, nicht MPG", v.FuelType, unit)
	} else {
		logf("P2 Einheit: %s (fuelType=%q)", unit, v.FuelType)
	}

	// P3: Sentinel-Werte bei Scores
	if v.GHGScore == "-1" || v.GHGScore == "" {
		warnf("P3 GHG-Score: Wert %q → nicht verfügbar (vermutlich Fahrzeug vor ~2000)", v.GHGScore)
	}
	if v.FEScore == "-1" || v.FEScore == "" {
		warnf("P3 FE-Score: Wert %q → nicht verfügbar", v.FEScore)
	}

	// Generisch: wichtige Felder leer
	checks := map[string]string{
		"make":  v.Make,
		"model": v.Model,
		"year":  v.Year,
		"drive": v.Drive,
		"trany": v.Trany,
	}
	for field, val := range checks {
		if strings.TrimSpace(val) == "" {
			warnf("LEER: Feld %q ist leer — möglicherweise unvollständige API-Daten", field)
		}
	}

	// Generisch: Hubraum leer bei Verbrennern
	if v.fuelUnit() == "MPG" && strings.TrimSpace(v.Displ) == "" {
		warnf("LEER: displ (Hubraum) ist leer bei Verbrenner — unerwartete Datenlücke")
	}
}

// fetchVehicle ruft die Fahrzeugdaten für die gegebene ID ab.
// Gibt rohe JSON-Bytes zurück oder einen beschreibenden Fehler.
func fetchVehicle(id string) ([]byte, error) {
	url := baseURL + "/vehicle/" + id
	logf("GET %s", url)
	logf("Header: Accept: application/json")

	start := time.Now()
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("request konnte nicht erstellt werden: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("API nicht erreichbar: %w", err)
	}
	defer resp.Body.Close()

	logf("Status: %d %s (%.0fms)", resp.StatusCode, http.StatusText(resp.StatusCode), float64(time.Since(start).Milliseconds()))

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("Fahrzeug mit ID %q nicht gefunden", id)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unerwarteter HTTP-Status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("Antwort konnte nicht gelesen werden: %w", err)
	}
	logf("Response: %d bytes", len(body))
	return body, nil
}

// --- CLI dispatcher ---

func usage() {
	fmt.Fprintln(os.Stderr, "usage:")
	fmt.Fprintln(os.Stderr, "  vehicle-emission-api vehicle [flags] <id>   Fahrzeugdaten abrufen")
	fmt.Fprintln(os.Stderr, "  vehicle-emission-api validate-email <email> E-Mail-Adresse prüfen")
	fmt.Fprintln(os.Stderr, "  vehicle-emission-api check-email <email>    E-Mail prüfen (mit Cache)")
	fmt.Fprintln(os.Stderr, "  vehicle-emission-api fetch <id> <email>     Fahrzeug + E-Mail-Check")
	fmt.Fprintln(os.Stderr, "  vehicle-emission-api serve                  HTTP-Server starten")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "flags (vehicle, fetch):")
	fmt.Fprintln(os.Stderr, "  -text     Lesbare Textausgabe statt JSON")
	fmt.Fprintln(os.Stderr, "  -verbose  Aufgerufene URLs, Status-Codes, Datenwarnungen")
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	cmd := os.Args[1]
	args := os.Args[2:]

	var err error
	switch cmd {
	case "vehicle":
		err = cmdVehicle(args)
	case "validate-email":
		err = cmdValidateEmail(args)
	case "check-email":
		err = cmdCheckEmail(args)
	case "fetch":
		err = cmdFetch(args)
	case "serve":
		err = cmdServe(args)
	default:
		fmt.Fprintf(os.Stderr, "unbekannter Befehl: %q\n\n", cmd)
		usage()
		os.Exit(1)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "fehler: %v\n", err)
		os.Exit(1)
	}
}

func cmdValidateEmail(args []string) error {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "usage: validate-email <email>")
		os.Exit(1)
	}
	email := args[0]
	v := validator.NewEmailValidator(validator.NewDisposableChecker())
	if err := v.Validate(email); err != nil {
		if strings.Contains(err.Error(), "disposable") {
			fmt.Fprintf(os.Stderr, "BLOCKED: disposable email (domain: %s)\n", extractDomain(email))
		} else {
			fmt.Fprintf(os.Stderr, "fehler: %v\n", err)
		}
		os.Exit(1)
	}
	fmt.Printf("OK: %s\n", email)
	return nil
}

func extractDomain(email string) string {
	parts := strings.SplitN(email, "@", 2)
	if len(parts) == 2 {
		return parts[1]
	}
	return email
}

func cmdVehicle(args []string) error {
	fs := flag.NewFlagSet("vehicle", flag.ContinueOnError)
	textMode := fs.Bool("text", false, "Lesbare Textausgabe statt JSON")
	verboseFlag := fs.Bool("verbose", false, "Aufgerufene URLs, Status-Codes, Datenwarnungen")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 1 {
		return fmt.Errorf("verwendung: vehicle [-text] [-verbose] <id>")
	}

	verbose = *verboseFlag

	data, err := fetchVehicle(fs.Arg(0))
	if err != nil {
		return err
	}

	if !*textMode {
		fmt.Println(string(data))
		return nil
	}

	var v Vehicle
	if err := json.Unmarshal(data, &v); err != nil {
		return fmt.Errorf("JSON konnte nicht geparst werden: %w", err)
	}

	checkDataQuality(v)
	fmt.Print(v.FormatText())
	return nil
}

func cmdServe(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	port := fs.String("port", "8081", "Port auf dem der Server lauscht")
	verboseMode := fs.Bool("verbose", false, "Cache-Treffer und -Misses auf stderr ausgeben")
	if err := fs.Parse(args); err != nil {
		return err
	}

	fc := client.NewFuelEconomyClient(nil)
	ev := validator.NewEmailValidator(validator.NewDisposableChecker())
	ec := cache.NewEmailCache(6*time.Hour, context.Background())
	h := handler.New(ev, ec, fc, *verboseMode)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /vehicle/{id}", h.GetVehicle)

	addr := ":" + *port
	fmt.Fprintf(os.Stderr, "[serve] listening on %s\n", addr)
	return http.ListenAndServe(addr, mux)
}

func cmdFetch(args []string) error {
	fs := flag.NewFlagSet("fetch", flag.ContinueOnError)
	textMode := fs.Bool("text", false, "Lesbare Textausgabe statt JSON")
	verboseMode := fs.Bool("verbose", false, "Verbose-Ausgabe")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 2 {
		fmt.Fprintln(os.Stderr, "usage: fetch [-text] [-verbose] <vehicle-id> <email>")
		os.Exit(1)
	}
	vehicleID := fs.Arg(0)
	email := fs.Arg(1)

	vlog := func(prefix, msg string) {
		if *verboseMode {
			fmt.Fprintf(logWriter, "[%s] %s\n", prefix, msg)
		}
	}

	// E-Mail: Cache prüfen, ggf. validieren
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c := cache.NewEmailCache(time.Hour, ctx)

	if c.IsVerified(email) {
		vlog("cache", "HIT: "+email)
	} else {
		vlog("cache", "MISS: "+email+" — validating...")
		ev := validator.NewEmailValidator(validator.NewDisposableChecker())
		if err := ev.Validate(email); err != nil {
			if strings.Contains(err.Error(), "disposable") {
				fmt.Fprintf(os.Stderr, "BLOCKED: disposable email (domain: %s)\n", extractDomain(email))
			} else {
				fmt.Fprintf(os.Stderr, "fehler: %v\n", err)
			}
			os.Exit(1)
		}
		c.Add(email)
		vlog("cache", "ADDED: "+email)
	}

	// Vehicle-ID: positiver Integer
	id, err := strconv.Atoi(vehicleID)
	if err != nil || id <= 0 {
		return fmt.Errorf("vehicle-id muss ein positiver Integer sein, got %q", vehicleID)
	}

	// API-Aufruf
	vlog("api", "fetching vehicle "+vehicleID+"...")
	data, err := fetchVehicle(vehicleID)
	if err != nil {
		return err
	}
	vlog("api", "OK (200)")

	var v Vehicle
	if err := json.Unmarshal(data, &v); err != nil {
		return fmt.Errorf("JSON konnte nicht geparst werden: %w", err)
	}

	// CO2-Fallback
	var co2 *float64
	if f, err := strconv.ParseFloat(v.CO2Raw, 64); err == nil && f > 0 {
		co2 = &f
	} else if f, err := strconv.ParseFloat(v.CO2Pipe, 64); err == nil && f > 0 {
		co2 = &f
	}

	if *textMode {
		co2Str := "n/a"
		if co2 != nil {
			co2Str = fmt.Sprintf("%.1f g/mi", *co2)
		}
		fmt.Printf("%s %s (%s)\n", v.Make, v.Model, v.Year)
		fmt.Printf("Fuel: %s\n", v.FuelType)
		fmt.Printf("City: %s | Highway: %s | Combined: %s\n", v.City, v.Highway, v.Combined)
		fmt.Printf("CO2: %s\n", co2Str)
		fmt.Printf("Class: %s\n", v.Class)
		return nil
	}

	resp := struct {
		Make     string   `json:"make"`
		Model    string   `json:"model"`
		Year     string   `json:"year"`
		City     string   `json:"city08"`
		Highway  string   `json:"highway08"`
		Combined string   `json:"comb08"`
		CO2      *float64 `json:"co2"`
		Class    string   `json:"vclass"`
		FuelType string   `json:"fuelType"`
	}{
		Make:     v.Make,
		Model:    v.Model,
		Year:     v.Year,
		City:     v.City,
		Highway:  v.Highway,
		Combined: v.Combined,
		CO2:      co2,
		Class:    v.Class,
		FuelType: v.FuelType,
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(resp)
}

func cmdCheckEmail(args []string) error {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "usage: check-email <email>")
		os.Exit(1)
	}
	email := args[0]

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c := cache.NewEmailCache(time.Hour, ctx)

	if c.IsVerified(email) {
		fmt.Printf("CACHED: %s\n", email)
		return nil
	}

	v := validator.NewEmailValidator(validator.NewDisposableChecker())
	if err := v.Validate(email); err != nil {
		if strings.Contains(err.Error(), "disposable") {
			fmt.Fprintf(os.Stderr, "BLOCKED: disposable email (domain: %s)\n", extractDomain(email))
		} else {
			fmt.Fprintf(os.Stderr, "fehler: %v\n", err)
		}
		os.Exit(1)
	}

	c.Add(email)
	fmt.Printf("OK: %s (added to cache)\n", email)
	return nil
}
