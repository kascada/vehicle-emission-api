package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/kamran/vehicle-emission-api/client"
	"github.com/kamran/vehicle-emission-api/validator"
)

// Handler enthält die HTTP-Handler für die API.
type Handler struct {
	emailValidator *validator.EmailValidator
	fuelClient     *client.FuelEconomyClient
}

func New(ev *validator.EmailValidator, fc *client.FuelEconomyClient) *Handler {
	return &Handler{emailValidator: ev, fuelClient: fc}
}

// GetVehicle behandelt GET /vehicle/{id}
func (h *Handler) GetVehicle(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// 1. E-Mail aus Header lesen und validieren
	email := r.Header.Get("X-Email")
	if email == "" {
		writeError(w, http.StatusUnauthorized, "missing X-Email header")
		return
	}

	if err := h.emailValidator.Validate(email); err != nil {
		// Unterscheide: ungültiges Format (400) vs. Wegwerf-Adresse (403)
		if isDisposableError(err) {
			writeError(w, http.StatusForbidden, err.Error())
		} else {
			writeError(w, http.StatusBadRequest, err.Error())
		}
		return
	}

	// 2. Fahrzeug-ID validieren
	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid vehicle ID: must be a positive integer")
		return
	}

	// 3. Fahrzeugdaten abrufen
	vehicle, err := h.fuelClient.GetVehicle(idStr)
	if err != nil {
		if errors.Is(err, client.ErrVehicleNotFound) {
			writeError(w, http.StatusNotFound, "vehicle not found")
			return
		}
		writeError(w, http.StatusBadGateway, "error fetching vehicle data from upstream")
		return
	}

	// 4. Erfolg
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(vehicle)
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
