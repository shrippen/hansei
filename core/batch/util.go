package batch

import "strings"

func join(lines []string) string { return strings.Join(lines, "\n") }

// contains compares after trimming, so the AI's snippet may differ in surrounding whitespace.
func contains(haystack, needle string) bool {
	n := strings.TrimSpace(needle)
	return n != "" && strings.Contains(haystack, n)
}
