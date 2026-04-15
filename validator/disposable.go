package validator

import (
	_ "embed"
	"strings"
	"sync"
)

//go:embed disposable_domains.txt
var disposableDomainsTxt string

// DisposableChecker prüft E-Mail-Domains gegen eine Liste
// bekannter Wegwerf-E-Mail-Anbieter.
type DisposableChecker struct {
	domains map[string]struct{}
	mu      sync.RWMutex
}

// NewDisposableChecker parst die eingebettete disposable_domains.txt.
// Kommentare (#) und Leerzeilen werden übersprungen, alle Domains lowercase gespeichert.
func NewDisposableChecker() *DisposableChecker {
	domains := make(map[string]struct{})
	for _, line := range strings.Split(disposableDomainsTxt, "\n") {
		line = strings.TrimSpace(strings.ToLower(line))
		if line != "" && !strings.HasPrefix(line, "#") {
			domains[line] = struct{}{}
		}
	}
	return &DisposableChecker{domains: domains}
}

// NewDisposableCheckerFromMap erstellt einen Checker aus einer vorgegebenen Map.
// Nützlich für Tests.
func NewDisposableCheckerFromMap(domains map[string]struct{}) *DisposableChecker {
	return &DisposableChecker{domains: domains}
}

// IsDisposable prüft ob die Domain (lowercase) in der Liste ist.
// Prüft auch Subdomains: sub.mailinator.com → mailinator.com.
func (c *DisposableChecker) IsDisposable(domain string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	domain = strings.ToLower(domain)

	// Direkter Match
	if _, ok := c.domains[domain]; ok {
		return true
	}

	// Subdomain-Check: Schrittweise kürzen
	for {
		idx := strings.Index(domain, ".")
		if idx < 0 {
			break
		}
		domain = domain[idx+1:]
		if _, ok := c.domains[domain]; ok {
			return true
		}
	}

	return false
}
