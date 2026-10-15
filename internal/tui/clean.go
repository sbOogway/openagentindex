package tui

import (
	"strings"
	"unicode"

	"github.com/charmbracelet/x/ansi"
)

// clean makes untrusted agent text safe for the terminal. It removes escape
// sequences, control characters and bidirectional overrides, and puts the text
// on one line.
func clean(s string) string {
	s = ansi.Strip(s)
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == '\n' || r == '\t' || r == '\r':
			b.WriteRune(' ')
		case unicode.IsControl(r) || isBidiControl(r):
		default:
			b.WriteRune(r)
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

// cleanLines is clean for text with more lines, for example chat replies. It
// keeps the line breaks.
func cleanLines(s string) []string {
	lines := strings.Split(strings.ReplaceAll(ansi.Strip(s), "\r\n", "\n"), "\n")
	for i, l := range lines {
		lines[i] = clean(l)
	}
	return lines
}

// isBidiControl reports the characters that can make text look different from
// its content (Trojan Source, CVE-2021-42574).
func isBidiControl(r rune) bool {
	return (r >= 0x202A && r <= 0x202E) || (r >= 0x2066 && r <= 0x2069) || r == 0x200E || r == 0x200F || r == 0x061C
}
