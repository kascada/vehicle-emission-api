package client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

const (
	baseURL      = "https://www.fueleconomy.gov/ws/rest/vehicle"
	cacheTTL     = 24 * time.Hour
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
type rawVehicle struct {
	Make             string  `json:"make"`
	Model            string  `json:"model"`
	Year             int     `json:"year"`
	City08           int     `json:"city08"`
	Highway08        int     `json:"highway08"`
	Comb08           int     `json:"comb08"`
	CO2              int     `json:"co2"`
	CO2TailpipeGpm   float64 `json:"co2TailpipeGpm"`
	VClass           string  `json:"VClass"`
	FuelType1        string  `json:"fuelType1"`
}

type cacheEntry struct {
	data      *VehicleData
	fetchedAt time.Time
}

// FuelEconomyClient ruft Fahrzeugdaten von der FuelEconomy.gov API ab.
type FuelEconomyClient struct {
	httpClient *http.Client
	cache      map[string]cacheEntry
	mu         sync.RWMutex
}

func NewFuelEconomyClient(httpClient *http.Client) *FuelEconomyClient {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: clientTimeout}
	}
	return &FuelEconomyClient{
		httpClient: httpClient,
		cache:      make(map[string]cacheEntry),
	}
}

// ErrVehicleNotFound wird zurückgegeben wenn die ID nicht existiert.
var ErrVehicleNotFound = fmt.Errorf("vehicle not found")

// GetVehicle ruft die Fahrzeugdaten für eine ID ab.
// Nutzt einen In-Memory-Cache mit TTL.
func (c *FuelEconomyClient) GetVehicle(id string) (*VehicleData, error) {
	// Cache prüfen
	c.mu.RLock()
	if entry, ok := c.cache[id]; ok && time.Since(entry.fetchedAt) < cacheTTL {
		c.mu.RUnlock()
		return entry.data, nil
	}
	c.mu.RUnlock()

	// Extern abrufen
	url := fmt.Sprintf("%s/%s", baseURL, id)
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

	// Cache speichern
	c.mu.Lock()
	c.cache[id] = cacheEntry{data: data, fetchedAt: time.Now()}
	c.mu.Unlock()

	return data, nil
}

// mapToVehicleData wandelt die Rohdaten in unser sauberes Format um.
// Enthält die Fallback-Logik für das CO2-Feld.
func mapToVehicleData(raw *rawVehicle) *VehicleData {
	v := &VehicleData{
		Make:      raw.Make,
		Model:     raw.Model,
		Year:      raw.Year,
		City08:    raw.City08,
		Highway08: raw.Highway08,
		Comb08:    raw.Comb08,
		VClass:    raw.VClass,
		FuelType:  raw.FuelType1,
	}

	// CO2 Fallback-Logik (siehe NOTES.md, Problem 1):
	// 1. co2 > 0 → direkt verwenden
	// 2. co2 <= 0 && co2TailpipeGpm > 0 → Fallback
	// 3. beides <= 0 → null (Pointer bleibt nil)
	switch {
	case raw.CO2 > 0:
		co2 := float64(raw.CO2)
		v.CO2 = &co2
	case raw.CO2TailpipeGpm > 0:
		v.CO2 = &raw.CO2TailpipeGpm
	}
	// Sonst bleibt v.CO2 == nil → JSON: "co2": null

	return v
}
