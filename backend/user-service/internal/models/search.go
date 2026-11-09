package models

import "strings"

// MaxSearchQueryLength bounds a user search query. A longer one is rejected
// rather than truncated so a caller never receives results for a query they
// did not send.
const MaxSearchQueryLength = 100

// NormalizeSearchQuery trims a search query and reports whether it is usable.
func NormalizeSearchQuery(query string) (string, bool) {
	trimmed := strings.TrimSpace(query)
	if trimmed == "" {
		return "", false
	}
	if len([]rune(trimmed)) > MaxSearchQueryLength {
		return "", false
	}
	return trimmed, true
}
