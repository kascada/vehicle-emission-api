package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// --- Vehicle.co2() ---

func TestCO2(t *testing.T) {
	tests := []struct {
		name       string
		co2Raw     string
		co2Pipe    string
		wantVal    string
		wantSource string
	}{
		{
			name:       "Verbrenner: primäres Feld vorhanden",
			co2Raw:     "338",
			co2Pipe:    "338.0",
			wantVal:    "338",
			wantSource: "co2",
		},
		{
			name:       "Alt-Fahrzeug: co2=-1, Fallback auf co2TailpipeGpm",
			co2Raw:     "-1",
			co2Pipe:    "634.7857142857143",
			wantVal:    "634.7857142857143",
			wantSource: "co2TailpipeGpm",
		},
		{
			name:       "Elektro: beide Felder 0 → kein Wert",
			co2Raw:     "0",
			co2Pipe:    "0.0",
			wantVal:    "",
			wantSource: "",
		},
		{
			name:       "Beide Felder leer",
			co2Raw:     "",
			co2Pipe:    "",
			wantVal:    "",
			wantSource: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := Vehicle{CO2Raw: tt.co2Raw, CO2Pipe: tt.co2Pipe}
			gotVal, gotSrc := v.co2()
			if gotVal != tt.wantVal {
				t.Errorf("co2() value = %q, want %q", gotVal, tt.wantVal)
			}
			if gotSrc != tt.wantSource {
				t.Errorf("co2() source = %q, want %q", gotSrc, tt.wantSource)
			}
		})
	}
}

// --- Vehicle.fuelUnit() ---

func TestFuelUnit(t *testing.T) {
	tests := []struct {
		name     string
		fuelType string
		atvType  string
		want     string
	}{
		{"Verbrenner Benzin", "Regular Gasoline", "", "MPG"},
		{"Verbrenner Diesel", "Diesel", "", "MPG"},
		{"Elektro (fuelType)", "Electricity", "", "MPGe"},
		{"Elektro (atvType EV)", "Electricity", "EV", "MPGe"},
		{"Plug-in Hybrid", "Regular Gasoline", "PHEV", "MPGe"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := Vehicle{FuelType: tt.fuelType, AtvType: tt.atvType}
			if got := v.fuelUnit(); got != tt.want {
				t.Errorf("fuelUnit() = %q, want %q", got, tt.want)
			}
		})
	}
}

// --- checkDataQuality() Warnungen ---

func TestCheckDataQuality(t *testing.T) {
	tests := []struct {
		name         string
		vehicle      Vehicle
		wantWarnings []string // Teilstrings die in der Ausgabe erscheinen müssen
		wantAbsent   []string // Teilstrings die NICHT erscheinen dürfen
	}{
		{
			name: "Toyota Camry — alles ok",
			vehicle: Vehicle{
				Make: "Toyota", Model: "Camry", Year: "2024",
				Drive: "Front-Wheel Drive", Trany: "Automatic (S8)",
				FuelType: "Regular Gasoline", Displ: "3.5",
				CO2Raw: "338", CO2Pipe: "338.0",
				GHGScore: "5", FEScore: "5",
			},
			wantWarnings: nil,
			wantAbsent:   []string{"[warn]"},
		},
		{
			name: "Ford F150 1990 — co2 Fallback + Scores fehlen",
			vehicle: Vehicle{
				Make: "Ford", Model: "F150 Pickup 2WD", Year: "1990",
				Drive: "Rear-Wheel Drive", Trany: "Automatic 3-spd",
				FuelType: "Regular Gasoline", Displ: "4.9",
				CO2Raw: "-1", CO2Pipe: "634.7857142857143",
				GHGScore: "-1", FEScore: "-1",
			},
			wantWarnings: []string{
				"P1 CO₂: primäres Feld co2",
				"P3 GHG-Score",
				"P3 FE-Score",
			},
		},
		{
			name: "Tesla Model Y — Elektro, kein CO₂",
			vehicle: Vehicle{
				Make: "Tesla", Model: "Model Y Long Range AWD", Year: "2024",
				Drive: "All-Wheel Drive", Trany: "Automatic (A1)",
				FuelType: "Electricity", AtvType: "EV",
				CO2Raw: "0", CO2Pipe: "0.0",
				GHGScore: "10", FEScore: "10",
			},
			wantWarnings: []string{
				"P1 CO₂: kein verwertbarer Wert",
				"P2 Einheit",
				"MPGe",
			},
		},
		{
			name: "Leere Pflichtfelder werden gemeldet",
			vehicle: Vehicle{
				Make: "", Model: "X", Year: "2020",
				Drive: "", Trany: "Manual",
				FuelType: "Regular Gasoline",
				CO2Raw:   "200", CO2Pipe: "200.0",
				GHGScore: "7", FEScore: "7",
			},
			wantWarnings: []string{
				`LEER: Feld "make"`,
				`LEER: Feld "drive"`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			verbose = true
			logWriter = &buf
			defer func() {
				verbose = false
				logWriter = os.Stderr
			}()

			checkDataQuality(tt.vehicle)

			out := buf.String()
			for _, want := range tt.wantWarnings {
				if !strings.Contains(out, want) {
					t.Errorf("erwartete Warnung %q nicht in Ausgabe:\n%s", want, out)
				}
			}
			for _, absent := range tt.wantAbsent {
				if strings.Contains(out, absent) {
					t.Errorf("unerwarteter Text %q in Ausgabe:\n%s", absent, out)
				}
			}
		})
	}
}

// --- fetchVehicle() mit httptest.Server ---

func TestFetchVehicle(t *testing.T) {
	tests := []struct {
		name        string
		id          string
		statusCode  int
		fixtureFile string
		wantErr     string
	}{
		{
			name:        "Toyota Camry — 200 OK",
			id:          "47085",
			statusCode:  http.StatusOK,
			fixtureFile: "testdata/vehicle_47085.json",
		},
		{
			name:        "Tesla Model Y — 200 OK",
			id:          "47913",
			statusCode:  http.StatusOK,
			fixtureFile: "testdata/vehicle_47913.json",
		},
		{
			name:       "Unbekannte ID — 404",
			id:         "9999999",
			statusCode: http.StatusNotFound,
			wantErr:    `"9999999" nicht gefunden`,
		},
		{
			name:       "Server-Fehler — 500",
			id:         "1",
			statusCode: http.StatusInternalServerError,
			wantErr:    "unerwarteter HTTP-Status 500",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
				if tt.fixtureFile != "" {
					data, err := os.ReadFile(tt.fixtureFile)
					if err != nil {
						t.Fatalf("Fixture nicht lesbar: %v", err)
					}
					w.Write(data)
				}
			}))
			defer srv.Close()

			// baseURL auf Testserver umbiegen
			orig := baseURL
			baseURL = srv.URL
			defer func() { baseURL = orig }()

			body, err := fetchVehicle(tt.id)

			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("erwarteter Fehler %q, aber kein Fehler", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("Fehler = %q, soll %q enthalten", err.Error(), tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unerwarteter Fehler: %v", err)
			}
			if len(body) == 0 {
				t.Error("leere Response")
			}
		})
	}
}
