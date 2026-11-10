package models

import (
	"regexp"
	"strings"
)

// hashtagPattern matches a hashtag body without the leading '#'. Letters from
// any script are allowed so non-English tags are indexed as written; the cap
// keeps a pathological tag from becoming an unbounded array element.
var hashtagPattern = regexp.MustCompile(`#([\p{L}\p{N}_]{1,50})`)

// ExtractHashtags returns the distinct, lower-cased hashtags in content in
// first-seen order. Storing them separately lets a hashtag search use an exact
// array overlap instead of a substring scan.
// ExtractHashtags returns the distinct tags in content, always as a non-nil
// slice.
//
// Non-nil matters: pgx encodes a nil []string as SQL NULL, and posts.hashtags is
// NOT NULL, so returning nil made every post without a tag fail to insert with
// "null value in column hashtags violates not-null constraint". Callers that
// only need the count -- search -- behave identically either way.
func ExtractHashtags(content string) []string {
	matches := hashtagPattern.FindAllStringSubmatch(content, -1)
	if len(matches) == 0 {
		return []string{}
	}

	seen := make(map[string]struct{}, len(matches))
	tags := make([]string, 0, len(matches))
	for _, match := range matches {
		tag := strings.ToLower(match[1])
		if _, exists := seen[tag]; exists {
			continue
		}
		seen[tag] = struct{}{}
		tags = append(tags, tag)
	}
	return tags
}

// MaxSearchQueryLength bounds a search query. Longer input is rejected rather
// than truncated so a caller never receives results for a query they did not
// send.
const MaxSearchQueryLength = 100

// NormalizeSearchQuery trims a query and reports whether it is usable.
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
