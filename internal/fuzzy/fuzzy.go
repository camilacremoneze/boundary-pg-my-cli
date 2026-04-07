// Package fuzzy provides simple fuzzy string matching for filtering lists.
package fuzzy

import "strings"

// Match returns true if every space-separated word in pattern appears as a
// contiguous substring in s (case-insensitively), and each word's match begins
// at or after the position where the previous word's match ended.
//
// Examples:
//
//	Match("cards read",  "cards-read-write") → true
//	Match("read cards",  "cards-read-write") → false  (order matters)
//	Match("bus read",    "business-read")    → true
//	Match("cards write", "cards-read-write") → true
func Match(pattern, s string) bool {
	s = strings.ToLower(s)
	words := strings.Fields(strings.ToLower(pattern))
	if len(words) == 0 {
		return true
	}
	pos := 0 // search position in s; advances after each word match
	for _, word := range words {
		idx := strings.Index(s[pos:], word)
		if idx < 0 {
			return false
		}
		pos += idx + len(word) // next word must start at or after where this one ended
	}
	return true
}

// Filter returns the subset of items where Match(pattern, item) is true.
func Filter(pattern string, items []string) []string {
	if pattern == "" {
		return items
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		if Match(pattern, item) {
			out = append(out, item)
		}
	}
	return out
}
