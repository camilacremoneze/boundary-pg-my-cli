package fuzzy_test

import (
	"testing"

	"github.com/camilacremoneze/pgcli-boundary-vault-integration/internal/fuzzy"
)

func TestMatch(t *testing.T) {
	tests := []struct {
		pattern string
		s       string
		want    bool
	}{
		// Empty pattern always matches.
		{"", "anything", true},
		{"", "", true},

		// Single word – contiguous substring match.
		{"cards", "cards-read-write", true},
		{"read", "cards-read-write", true},
		{"write", "cards-read-write", true},
		{"xyz", "cards-read-write", false},
		// Subsequence that is NOT a contiguous substring must not match.
		{"crw", "cards-read-write", false},

		// Case insensitivity.
		{"CARDS", "cards-read-write", true},
		{"Cards", "CARDS-READ-WRITE", true},

		// Two words – order is enforced.
		{"cards read", "cards-read-write", true},
		{"cards write", "cards-read-write", true},
		{"read write", "cards-read-write", true},
		{"write read", "cards-read-write", false}, // wrong order
		{"read cards", "cards-read-write", false}, // wrong order
		{"bus read", "business-read", true},
		{"read bus", "business-read", false},

		// Three words – all must appear in order.
		{"prod data read", "production-database-readonly", true},
		{"data prod read", "production-database-readonly", false},

		// Words must not overlap: second word must start after first ended.
		{"ab ab", "ab", false},   // only one "ab" present
		{"ab ab", "ab ab", true}, // two separate "ab"s

		// Partial prefix match.
		{"prod", "production", true},
		{"prod data", "production database", true},

		// No match at all.
		{"zzz", "abcdef", false},
	}

	for _, tt := range tests {
		got := fuzzy.Match(tt.pattern, tt.s)
		if got != tt.want {
			t.Errorf("Match(%q, %q) = %v, want %v", tt.pattern, tt.s, got, tt.want)
		}
	}
}

func TestFilter(t *testing.T) {
	items := []string{
		"cards-read-write",
		"cards-readonly",
		"business-read",
		"production-database",
	}

	t.Run("empty pattern returns all", func(t *testing.T) {
		got := fuzzy.Filter("", items)
		if len(got) != len(items) {
			t.Errorf("Filter(%q) returned %d items, want %d", "", len(got), len(items))
		}
	})

	t.Run("single word match", func(t *testing.T) {
		got := fuzzy.Filter("cards", items)
		want := []string{"cards-read-write", "cards-readonly"}
		if !equalSlices(got, want) {
			t.Errorf("Filter(%q) = %v, want %v", "cards", got, want)
		}
	})

	t.Run("two words in order", func(t *testing.T) {
		got := fuzzy.Filter("cards write", items)
		want := []string{"cards-read-write"}
		if !equalSlices(got, want) {
			t.Errorf("Filter(%q) = %v, want %v", "cards write", got, want)
		}
	})

	t.Run("two words match multiple", func(t *testing.T) {
		got := fuzzy.Filter("cards read", items)
		want := []string{"cards-read-write", "cards-readonly"}
		if !equalSlices(got, want) {
			t.Errorf("Filter(%q) = %v, want %v", "cards read", got, want)
		}
	})

	t.Run("no match", func(t *testing.T) {
		got := fuzzy.Filter("zzz", items)
		if len(got) != 0 {
			t.Errorf("Filter(%q) = %v, want empty", "zzz", got)
		}
	})
}

func equalSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
