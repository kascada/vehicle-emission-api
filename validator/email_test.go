package validator

import (
	"strings"
	"testing"
)

// testDisposableChecker erstellt einen DisposableChecker mit einer kleinen Testliste.
func testDisposableChecker() *DisposableChecker {
	return NewDisposableCheckerFromMap(map[string]struct{}{
		"mailinator.com":     {},
		"guerrillamail.com":  {},
		"tempmail.com":       {},
		"throwaway.email":    {},
		"yopmail.com":        {},
	})
}

// --- TESTPLAN 1.1: Gültige E-Mail-Adressen ---

func TestValidate_ValidEmails(t *testing.T) {
	t.Parallel()
	v := NewEmailValidator(testDisposableChecker())

	cases := []struct {
		name  string
		email string
	}{
		{"standard", "user@gmail.com"},
		{"dot in local part, german TLD", "user.name@company.de"},
		{"plus addressing", "user+tag@example.com"},
		{"subdomain", "user@subdomain.example.com"},
		{"uppercase domain", "user@EXAMPLE.COM"},
		{"numeric local part", "123@example.com"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if err := v.Validate(tc.email); err != nil {
				t.Errorf("expected valid, got error: %v", err)
			}
		})
	}
}

// --- TESTPLAN 1.2: Formal ungültige E-Mail-Adressen ---

func TestValidate_InvalidFormat(t *testing.T) {
	t.Parallel()
	v := NewEmailValidator(testDisposableChecker())

	cases := []struct {
		name  string
		email string
	}{
		{"empty string", ""},
		{"no at sign", "user"},
		{"no local part", "@domain.com"},
		{"no domain", "user@"},
		{"domain starts with dot", "user@.com"},
		{"space in local part", "user space@domain.com"},
		{"no TLD", "user@domain"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := v.Validate(tc.email)
			if err == nil {
				t.Errorf("expected error for %q, got nil", tc.email)
			}
		})
	}
}

// --- TESTPLAN 1.3: Wegwerf-E-Mail-Adressen ---

func TestValidate_DisposableEmails(t *testing.T) {
	t.Parallel()
	v := NewEmailValidator(testDisposableChecker())

	cases := []struct {
		name  string
		email string
	}{
		{"mailinator", "user@mailinator.com"},
		{"guerrillamail", "user@guerrillamail.com"},
		{"tempmail", "user@tempmail.com"},
		{"throwaway", "user@throwaway.email"},
		{"yopmail", "user@yopmail.com"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := v.Validate(tc.email)
			if err == nil {
				t.Fatalf("expected disposable error for %q, got nil", tc.email)
			}
			if !strings.Contains(err.Error(), "disposable") {
				t.Errorf("expected disposable error, got: %v", err)
			}
		})
	}
}

// --- TESTPLAN 1.5: Integration mit eingebetteter Domain-Liste ---

func TestValidate_WithEmbeddedChecker(t *testing.T) {
	t.Parallel()
	v := NewEmailValidator(NewDisposableChecker())

	cases := []struct {
		name        string
		email       string
		wantErr     bool
		errContains string
	}{
		{"gültige Adresse", "user@example.com", false, ""},
		{"mailinator geblockt", "user@mailinator.com", true, "disposable"},
		{"guerrillamail geblockt", "user@guerrillamail.com", true, "disposable"},
		{"yopmail geblockt", "user@yopmail.com", true, "disposable"},
		{"subdomain geblockt", "user@sub.mailinator.com", true, "disposable"},
		{"ungültiges Format", "notanemail", true, "invalid email format"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := v.Validate(tc.email)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error for %q, got nil", tc.email)
				}
				if tc.errContains != "" && !strings.Contains(err.Error(), tc.errContains) {
					t.Errorf("error = %q, soll %q enthalten", err.Error(), tc.errContains)
				}
			} else {
				if err != nil {
					t.Errorf("expected nil error for %q, got: %v", tc.email, err)
				}
			}
		})
	}
}

// --- TESTPLAN 1.4: Grenzfälle ---

func TestValidate_EdgeCases(t *testing.T) {
	t.Parallel()
	v := NewEmailValidator(testDisposableChecker())

	t.Run("case insensitive disposable", func(t *testing.T) {
		t.Parallel()
		err := v.Validate("user@MAILINATOR.COM")
		if err == nil {
			t.Fatal("expected disposable error for uppercase domain, got nil")
		}
		if !strings.Contains(err.Error(), "disposable") {
			t.Errorf("expected disposable error, got: %v", err)
		}
	})

	t.Run("subdomain of disposable", func(t *testing.T) {
		t.Parallel()
		err := v.Validate("user@subdomain.mailinator.com")
		if err == nil {
			t.Fatal("expected disposable error for subdomain, got nil")
		}
		if !strings.Contains(err.Error(), "disposable") {
			t.Errorf("expected disposable error, got: %v", err)
		}
	})

	t.Run("similar but valid domain", func(t *testing.T) {
		t.Parallel()
		if err := v.Validate("user@gmail.co"); err != nil {
			t.Errorf("expected valid for gmail.co, got: %v", err)
		}
	})

	t.Run("max length email (254 chars)", func(t *testing.T) {
		t.Parallel()
		// RFC 5321: max 254 chars total
		// local part max 64, domain max 253
		local := strings.Repeat("a", 64)
		domain := strings.Repeat("a", 63) + "." + strings.Repeat("b", 63) + ".com"
		email := local + "@" + domain
		// Should be formally valid (net/mail should parse it)
		// Not disposable — should pass
		err := v.Validate(email)
		if err != nil {
			t.Errorf("expected valid for max-length email, got: %v", err)
		}
	})
}
