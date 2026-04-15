package handler_test

import (
	"encoding/json"
	"net/http"
	"testing"
)

const baseURL = "https://vehicle.akte.de"

// TestLiveAPI testet die deployed API. Übersprungen mit go test -short.
func TestLiveAPI(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}

	t.Run("valid request returns vehicle data", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/vehicle/47085?email=user@gmail.com")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != 200 {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}

		var body map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatalf("invalid JSON: %v", err)
		}

		// Pflichtfelder prüfen
		for _, field := range []string{"make", "model", "year", "city08", "highway08", "comb08"} {
			if _, ok := body[field]; !ok {
				t.Errorf("missing field %q in response", field)
			}
		}

		if body["make"] != "Toyota" {
			t.Errorf("expected make=Toyota, got %v", body["make"])
		}
	})

	t.Run("missing email returns 401", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/vehicle/47085")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != 401 {
			t.Errorf("expected 401, got %d", resp.StatusCode)
		}
	})

	t.Run("disposable email returns 403", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/vehicle/47085?email=user@mailinator.com")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != 403 {
			t.Errorf("expected 403, got %d", resp.StatusCode)
		}
	})

	t.Run("invalid ID returns 400", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/vehicle/abc?email=user@gmail.com")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != 400 {
			t.Errorf("expected 400, got %d", resp.StatusCode)
		}
	})

	t.Run("root path returns 404 with usage hint", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != 404 {
			t.Errorf("expected 404, got %d", resp.StatusCode)
		}

		var body map[string]string
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatalf("invalid JSON: %v", err)
		}

		if _, ok := body["usage"]; !ok {
			t.Error("expected 'usage' field in 404 response")
		}
	})

	t.Run("rate limit returns 429", func(t *testing.T) {
		// 1001 Requests um globales Limit von 1000/min auszulösen
		email := "ratelimit-test@gmail.com"
		for i := 0; i < 1001; i++ {
			resp, err := http.Get(baseURL + "/vehicle/47085?email=" + email)
			if err != nil {
				t.Fatalf("request %d failed: %v", i, err)
			}
			resp.Body.Close()

			if resp.StatusCode == 429 {
				return // Erfolg: Rate Limit greift
			}
		}
		t.Error("expected 429 after 1000 requests, but all succeeded")
	})

	t.Run("email via header works", func(t *testing.T) {
		req, _ := http.NewRequest("GET", baseURL+"/vehicle/47085", nil)
		req.Header.Set("Email", "user@gmail.com")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != 200 {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
	})
}
