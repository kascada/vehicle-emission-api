package client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/kamran/vehicle-emission-api/cache"
)

const (
	baseURL       = "https://www.fueleconomy.gov/ws/rest/vehicle"
	clientTimeout = 10 * time.Second
)

// VehicleData enthält die aufbereiteten Fahrzeug-Emissionsdaten.
type VehicleData struct {
	Make      string   `json:"make"`
	Model     string   `json:"model"`
	Year      int      `json:"year"`
	City08    int      `json:"city08"`
	Highway08 int      `json:"highway08"`
	Comb08    int      `json:"comb08"`
	CO2       *float64 `json:"co2"`       // Pointer: null wenn nicht verfügbar (z.B. alte Fahrzeuge)
	VClass    string   `json:"vclass"`
	FuelType  string   `json:"fuelType"`
}

// rawVehicle bildet die relevanten Felder der FuelEconomy.gov JSON-Response ab.
// Die API liefert alle Felder als JSON-Strings, auch numerische Werte.
type rawVehicle struct {
	Make           string `json:"make"`
	Model          string `json:"model"`
	Year           string `json:"year"`
	City08         string `json:"city08"`
	Highway08      string `json:"highway08"`
	Comb08         string `json:"comb08"`
	CO2            string `json:"co2"`
	CO2TailpipeGpm string `json:"co2TailpipeGpm"`
	VClass         string `json:"VClass"`
	FuelType1      string `json:"fuelType1"`
}

// FuelEconomyClient ruft Fahrzeugdaten von der FuelEconomy.gov API ab.
// cache ist optional (nil = kein Caching).
type FuelEconomyClient struct {
	baseURL    string
	httpClient *http.Client
	cache      *cache.VehicleCache
	verbose    bool
}

func NewFuelEconomyClient(httpClient *http.Client, vc *cache.VehicleCache, verbose bool) *FuelEconomyClient {
	return NewFuelEconomyClientWithBaseURL(baseURL, httpClient, vc, verbose)
}

// NewFuelEconomyClientWithBaseURL erstellt einen Client mit überschriebener Base-URL (für Tests).
func NewFuelEconomyClientWithBaseURL(url string, httpClient *http.Client, vc *cache.VehicleCache, verbose bool) *FuelEconomyClient {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: clientTimeout}
	}
	return &FuelEconomyClient{
		baseURL:    url,
		httpClient: httpClient,
		cache:      vc,
		verbose:    verbose,
	}
}

func (c *FuelEconomyClient) vlog(format string, args ...any) {
	if c.verbose {
		fmt.Fprintf(os.Stderr, format+"\n", args...)
	}
}

// ErrVehicleNotFound wird zurückgegeben wenn die ID nicht existiert.
var ErrVehicleNotFound = fmt.Errorf("vehicle not found")

// GetVehicle ruft die Fahrzeugdaten für eine ID ab.
// Bei gesetztem Cache wird erst dort nachgeschaut (HIT/MISS geloggt wenn verbose).
func (c *FuelEconomyClient) GetVehicle(id string) (*VehicleData, error) {
	if c.cache != nil {
		if data, ok := c.cache.Get(id); ok {
			c.vlog("[vcache] HIT:  %s", id)
			var v VehicleData
			if err := json.Unmarshal(data, &v); err == nil {
				return &v, nil
			}
		} else {
			c.vlog("[vcache] MISS: %s — fetching from API...", id)
		}
	}

	url := fmt.Sprintf("%s/%s", c.baseURL, id)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching vehicle data: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrVehicleNotFound
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status from fueleconomy.gov: %d", resp.StatusCode)
	}

	var raw rawVehicle
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decoding vehicle data: %w", err)
	}

	data := mapToVehicleData(&raw)

	if c.cache != nil {
		if b, err := json.Marshal(data); err == nil {
			c.cache.Set(id, b)
			c.vlog("[vcache] SET:  %s (%d bytes)", id, len(b))
		}
	}

	return data, nil
}

// mapToVehicleData wandelt die Rohdaten in unser sauberes Format um.
// Enthält die Fallback-Logik für das CO2-Feld.
func mapToVehicleData(raw *rawVehicle) *VehicleData {
	v := &VehicleData{
		Make:      raw.Make,
		Model:     raw.Model,
		Year:      parseInt(raw.Year),
		City08:    parseInt(raw.City08),
		Highway08: parseInt(raw.Highway08),
		Comb08:    parseInt(raw.Comb08),
		VClass:    raw.VClass,
		FuelType:  raw.FuelType1,
	}

	// CO2 Fallback-Logik:
	// 1. co2 > 0 → direkt verwenden
	// 2. co2 <= 0 && co2TailpipeGpm > 0 → Fallback
	// 3. beides <= 0 → null (Pointer bleibt nil)
	if co2 := parseFloat(raw.CO2); co2 > 0 {
		v.CO2 = &co2
	} else if pipe := parseFloat(raw.CO2TailpipeGpm); pipe > 0 {
		v.CO2 = &pipe
	}

	return v
}

func parseInt(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

func parseFloat(s string) float64 {
	f, _ := strconv.ParseFloat(s, 64)
	return f
}
