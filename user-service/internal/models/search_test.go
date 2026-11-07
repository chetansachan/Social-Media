package models

import (
	"strings"
	"testing"
)

func TestNormalizeSearchQuery(t *testing.T) {
	normalized, ok := NormalizeSearchQuery("  alice  ")
	if !ok || normalized != "alice" {
		t.Fatalf("normalized = %q, ok = %v", normalized, ok)
	}

	if _, ok := NormalizeSearchQuery("   "); ok {
		t.Fatal("a blank query should be rejected")
	}
	if _, ok := NormalizeSearchQuery(strings.Repeat("a", MaxSearchQueryLength+1)); ok {
		t.Fatalf("a %d rune query should be rejected", MaxSearchQueryLength+1)
	}

	exact := strings.Repeat("a", MaxSearchQueryLength)
	normalized, ok = NormalizeSearchQuery(exact)
	if !ok || normalized != exact {
		t.Fatalf("a max-length query should be accepted: %q, %v", normalized, ok)
	}

	// Runes, not bytes: a multi-byte script is not penalised for being encoded
	// as more than one byte per character.
	multi := strings.Repeat("日", MaxSearchQueryLength)
	if _, ok := NormalizeSearchQuery(multi); !ok {
		t.Fatal("a multi-byte query at the character limit should be accepted")
	}
}
