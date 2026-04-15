package validator

import (
	"testing"
)

func TestNewDisposableChecker_Embedded(t *testing.T) {
	t.Parallel()

	// NewDisposableChecker nutzt go:embed — prüft bekannte Domains aus der eingebetteten Liste
	dc := NewDisposableChecker()

	if !dc.IsDisposable("mailinator.com") {
		t.Error("expected mailinator.com to be disposable")
	}
	if !dc.IsDisposable("guerrillamail.com") {
		t.Error("expected guerrillamail.com to be disposable")
	}
	if !dc.IsDisposable("yopmail.com") {
		t.Error("expected yopmail.com to be disposable")
	}
	if dc.IsDisposable("gmail.com") {
		t.Error("gmail.com should not be disposable")
	}
}

func TestIsDisposable_DirectMatch(t *testing.T) {
	t.Parallel()
	dc := NewDisposableCheckerFromMap(map[string]struct{}{
		"mailinator.com":    {},
		"throwaway.email":   {},
	})

	cases := []struct {
		domain string
		want   bool
	}{
		{"mailinator.com", true},
		{"throwaway.email", true},
		{"gmail.com", false},
		{"example.com", false},
	}
	for _, tc := range cases {
		t.Run(tc.domain, func(t *testing.T) {
			t.Parallel()
			got := dc.IsDisposable(tc.domain)
			if got != tc.want {
				t.Errorf("IsDisposable(%q) = %v, want %v", tc.domain, got, tc.want)
			}
		})
	}
}

func TestIsDisposable_CaseInsensitive(t *testing.T) {
	t.Parallel()
	dc := NewDisposableCheckerFromMap(map[string]struct{}{
		"mailinator.com": {},
	})

	cases := []string{"MAILINATOR.COM", "Mailinator.Com", "mailinator.COM"}
	for _, domain := range cases {
		t.Run(domain, func(t *testing.T) {
			t.Parallel()
			if !dc.IsDisposable(domain) {
				t.Errorf("expected %q to be disposable (case insensitive)", domain)
			}
		})
	}
}

func TestIsDisposable_SubdomainCheck(t *testing.T) {
	t.Parallel()
	dc := NewDisposableCheckerFromMap(map[string]struct{}{
		"mailinator.com": {},
	})

	cases := []struct {
		domain string
		want   bool
	}{
		{"sub.mailinator.com", true},
		{"deep.sub.mailinator.com", true},
		{"notmailinator.com", false},
		{"mailinator.org", false},
	}
	for _, tc := range cases {
		t.Run(tc.domain, func(t *testing.T) {
			t.Parallel()
			got := dc.IsDisposable(tc.domain)
			if got != tc.want {
				t.Errorf("IsDisposable(%q) = %v, want %v", tc.domain, got, tc.want)
			}
		})
	}
}
