package repository

import "testing"

func TestPreviewTrimsRunesSafely(t *testing.T) {
	cases := []struct {
		name    string
		content string
		limit   int
		want    string
	}{
		{name: "short", content: "hello world", limit: 20, want: "hello world"},
		{name: "trimmed", content: "  hello  ", limit: 20, want: "hello"},
		{name: "truncated", content: "abcdef", limit: 3, want: "abc…"},
		{name: "unicode", content: "héllo wörld", limit: 4, want: "héll…"},
		{name: "disabled", content: "hello", limit: 0, want: ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := preview(tc.content, tc.limit); got != tc.want {
				t.Fatalf("preview(%q, %d) = %q, want %q", tc.content, tc.limit, got, tc.want)
			}
		})
	}
}
