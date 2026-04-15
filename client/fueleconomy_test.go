package client_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/kamran/vehicle-emission-api/client"
)

// TestGetVehicle_CO2Fallback prüft die Fallback-Logik für das CO2-Feld.
// P1: co2 > 0 → direkt; co2 <= 0 && co2TailpipeGpm > 0 → Fallback; beides <= 0 → nil.
func TestGetVehicle_CO2Fallback(t *testing.T) {
	tests := []struct {
		name        string
		fixtureFile string
		wantCO2     *float64
	}{
		{
			name:        "Verbrenner: primäres co2-Feld vorhanden",
			fixtureFile: "../testdata/vehicle_47085.json",
			wantCO2:     ptr(338.0),
		},
		{
			name:        "Elektro: co2=0, co2TailpipeGpm=0 → nil",
			fixtureFile: "../testdata/vehicle_47913.json",
			wantCO2:     nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := os.ReadFile(tt.fixtureFile)
			if err != nil {
				t.Fatalf("Fixture nicht lesbar: %v", err)
			}
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				w.Write(data)
			}))
			defer srv.Close()

			fc := client.NewFuelEconomyClientWithBaseURL(srv.URL, nil, nil, false)
			v, err := fc.GetVehicle("47085")
			if err != nil {
				t.Fatalf("unerwarteter Fehler: %v", err)
			}

			if tt.wantCO2 == nil {
				if v.CO2 != nil {
					t.Errorf("CO2 = %.1f, want nil", *v.CO2)
				}
			} else {
				if v.CO2 == nil {
					t.Errorf("CO2 = nil, want %.1f", *tt.wantCO2)
				} else if *v.CO2 != *tt.wantCO2 {
					t.Errorf("CO2 = %.1f, want %.1f", *v.CO2, *tt.wantCO2)
				}
			}
		})
	}
}

// TestGetVehicle_HTTP prüft HTTP-Statuscodes: 200, 404, 500.
func TestGetVehicle_HTTP(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		body       string
		wantErr    bool
		notFound   bool
	}{
		{
			name:       "200 OK",
			statusCode: http.StatusOK,
			body:       mustRead(t, "../testdata/vehicle_47085.json"),
		},
		{
			name:       "404 Not Found",
			statusCode: http.StatusNotFound,
			wantErr:    true,
			notFound:   true,
		},
		{
			name:       "500 Internal Server Error",
			statusCode: http.StatusInternalServerError,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
				if tt.body != "" {
					w.Write([]byte(tt.body))
				}
			}))
			defer srv.Close()

			fc := client.NewFuelEconomyClientWithBaseURL(srv.URL, nil, nil, false)
			_, err := fc.GetVehicle("1")

			if tt.wantErr {
				if err == nil {
					t.Fatal("erwarteter Fehler, aber kein Fehler")
				}
				if tt.notFound && err != client.ErrVehicleNotFound {
					t.Errorf("err = %v, want ErrVehicleNotFound", err)
				}
			} else if err != nil {
				t.Fatalf("unerwarteter Fehler: %v", err)
			}
		})
	}
}

func ptr(f float64) *float64 { return &f }

func mustRead(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Fixture nicht lesbar: %v", err)
	}
	return string(data)
}
