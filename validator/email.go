package validator

import (
	"fmt"
	"net/mail"
	"strings"
)

// EmailValidator prüft E-Mail-Adressen auf formale Gültigkeit
// und gegen eine Disposable-Domain-Liste.
type EmailValidator struct {
	disposable *DisposableChecker
}

func NewEmailValidator(disposable *DisposableChecker) *EmailValidator {
	return &EmailValidator{disposable: disposable}
}

// Validate prüft die E-Mail-Adresse.
// Gibt nil zurück wenn gültig, sonst einen beschreibenden Fehler.
func (v *EmailValidator) Validate(email string) error {
	if email == "" {
		return fmt.Errorf("e-mail address is required")
	}

	// RFC 5322 Parsing
	addr, err := mail.ParseAddress(email)
	if err != nil {
		return fmt.Errorf("invalid email format: %w", err)
	}

	// Domain extrahieren
	parts := strings.SplitN(addr.Address, "@", 2)
	if len(parts) != 2 || parts[1] == "" {
		return fmt.Errorf("invalid email format: missing domain")
	}

	domain := strings.ToLower(parts[1])

	// Mindestens eine TLD prüfen (domain muss einen Punkt enthalten)
	if !strings.Contains(domain, ".") {
		return fmt.Errorf("invalid email format: domain must contain a TLD")
	}

	// Disposable-Check
	if v.disposable.IsDisposable(domain) {
		return fmt.Errorf("disposable email addresses are not allowed")
	}

	return nil
}
