package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"

	"github.com/kamran/vehicle-emission-api/cache"
	"github.com/kamran/vehicle-emission-api/client"
	"github.com/kamran/vehicle-emission-api/validator"
)

// Handler enthält die HTTP-Handler für die API.
type Handler struct {
	emailValidator *validator.EmailValidator
	emailCache     *cache.EmailCache
	rateLimiter    *cache.RateLimiter
	fuelClient     *client.FuelEconomyClient
	verbose        bool
}

func New(ev *validator.EmailValidator, ec *cache.EmailCache, rl *cache.RateLimiter, fc *client.FuelEconomyClient, verbose bool) *Handler {
	return &Handler{emailValidator: ev, emailCache: ec, rateLimiter: rl, fuelClient: fc, verbose: verbose}
}

// GetVehicle behandelt GET /vehicle/{id}
func (h *Handler) GetVehicle(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// 1. E-Mail lesen: X-Email-Header hat Vorrang, dann Query-Param
	email := r.Header.Get("Email")
	if email == "" {
		email = r.URL.Query().Get("email")
	}
	if email == "" {
		writeError(w, http.StatusUnauthorized, "missing email: provide Email header or ?email= query param")
		return
	}

	// 2. E-Mail validieren (Cache-Check zuerst)
	if h.emailCache.IsVerified(email) {
		h.vlog("[cache] HIT:   " + email)
	} else {
		h.vlog("[cache] MISS:  " + email + " — validating...")
		if err := h.emailValidator.Validate(email); err != nil {
			if isDisposableError(err) {
				writeError(w, http.StatusForbidden, err.Error())
			} else {
				writeError(w, http.StatusBadRequest, err.Error())
			}
			return
		}
		h.emailCache.Add(email)
		h.vlog("[cache] ADDED: " + email)
	}

	// 3. Rate Limiting (global)
	if !h.rateLimiter.Allow("global") {
		writeError(w, http.StatusTooManyRequests, "rate limit exceeded")
		return
	}

	// 4. Fahrzeug-ID validieren
	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid vehicle ID: must be a positive integer")
		return
	}

	// 4. Fahrzeugdaten abrufen
	vehicle, err := h.fuelClient.GetVehicle(idStr)
	if err != nil {
		if errors.Is(err, client.ErrVehicleNotFound) {
			writeError(w, http.StatusNotFound, "vehicle not found")
			return
		}
		writeError(w, http.StatusBadGateway, "error fetching vehicle data from upstream")
		return
	}

	// 5. Erfolg
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(vehicle)
}

func (h *Handler) vlog(msg string) {
	if h.verbose {
		fmt.Fprintln(os.Stderr, msg)
	}
}

// isDisposableError prüft ob der Fehler von der Disposable-Prüfung kommt.
func isDisposableError(err error) bool {
	return err.Error() == "disposable email addresses are not allowed"
}

type errorResponse struct {
	Error string `json:"error"`
}

func writeError(w http.ResponseWriter, status int, msg string) {
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(errorResponse{Error: msg})
}
