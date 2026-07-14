package index

import "strings"

// deriveTitle returns body's title: the first line that is non-whitespace
// after strings.TrimSpace (which also strips a trailing \r for CRLF bodies),
// skipping any leading blank lines. Returns "" if body has no such line.
func deriveTitle(body string) string {
	for _, line := range strings.Split(body, "\n") {
		if t := strings.TrimSpace(line); t != "" {
			return t
		}
	}
	return ""
}
