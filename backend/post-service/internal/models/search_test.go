package models

import (
	"strings"
	"testing"
)

func TestExtractHashtags(t *testing.T) {
	cases := []struct {
		name    string
		content string
		want    []string
	}{
		{name: "none", content: "no tags here", want: nil},
		{name: "single", content: "shipped #golang today", want: []string{"golang"}},
		{name: "distinct lowercased", content: "#GoLang and #golang again", want: []string{"golang"}},
		{name: "multiple in order", content: "#a #b #1989 #_c", want: []string{"a", "b", "1989", "_c"}},
		{name: "unicode", content: "#日本語 for you", want: []string{"日本語"}},
		{name: "not a tag when attached to a word", content: "C# is a language", want: nil},
		{name: "empty tag body is ignored", content: "nothing # here", want: nil},
		{name: "tag longer than the cap keeps the prefix", content: "#" + strings.Repeat("x", 60), want: []string{strings.Repeat("x", 50)}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ExtractHashtags(tc.content)
			if len(got) != len(tc.want) {
				t.Fatalf("ExtractHashtags(%q) = %v, want %v", tc.content, got, tc.want)
			}
			for i := range tc.want {
				if got[i] != tc.want[i] {
					t.Fatalf("tag %d = %q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}

// The reason the result is never a nil slice: pgx encodes nil []string as SQL
// NULL, and posts.hashtags is NOT NULL. Returning nil made every post without a
// tag fail to insert with "null value in column hashtags violates not-null
// constraint" -- the whole composer, media included.
func TestExtractHashtagsNeverReturnsNil(t *testing.T) {
	for _, content := range []string{
		"",
		"no tags here",
		"C# is a language",
		"nothing # here",
		"#",
		strings.Repeat(" ", 200),
	} {
		got := ExtractHashtags(content)
		if got == nil {
			t.Errorf(
				"ExtractHashtags(%q) returned nil; CreatePost would bind NULL into a NOT NULL column",
				content,
			)
		}
		if len(got) != 0 {
			t.Errorf("ExtractHashtags(%q) = %v, want no tags", content, got)
		}
	}
}

func TestExtractHashtagsStillFindsTagsWhenSomeExist(t *testing.T) {
	got := ExtractHashtags("shipped #golang and #Testing")
	if len(got) != 2 {
		t.Fatalf("got %v, want two tags", got)
	}
	for i, want := range []string{"golang", "testing"} {
		if got[i] != want {
			t.Fatalf("tag %d = %q, want %q", i, got[i], want)
		}
	}
}

func TestNormalizeSearchQuery(t *testing.T) {
	normalized, ok := NormalizeSearchQuery("  #golang performance  ")
	if !ok || normalized != "#golang performance" {
		t.Fatalf("normalized = %q, ok = %v", normalized, ok)
	}

	if _, ok := NormalizeSearchQuery("   "); ok {
		t.Fatal("a blank query should be rejected")
	}
}
