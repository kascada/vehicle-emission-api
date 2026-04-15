package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
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

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: vehicle-emission-api vehicle [flags] <id>")
		os.Exit(1)
	}

	if os.Args[1] != "vehicle" {
		fmt.Fprintf(os.Stderr, "unbekannter Befehl: %q\n", os.Args[1])
		os.Exit(1)
	}

	if err := cmdVehicle(os.Args[2:]); err != nil {
		fmt.Fprintf(os.Stderr, "fehler: %v\n", err)
		os.Exit(1)
	}
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
