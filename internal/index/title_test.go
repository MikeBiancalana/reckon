// Package index — TDD red tests for reckon-fnqs.3 (subject/body node
// convention: a derived `_nodes.title` column, computed at reconcile time).
//
// deriveTitle does not exist yet — internal/index/title.go (plan.md D3/D4) is
// the implementation phase's job. Referencing it here means the whole index
// package fails to COMPILE until title.go defines it, mirroring the existing
// red-state pattern documented in internal/cli/todo_test.go's header comment
// ("the package does not build until todo.go exists"): not "tests run and
// fail," but "the package does not build until deriveTitle exists." Once
// title.go lands, the package builds and TestDeriveTitle starts driving real
// (pass/fail) behavior.
package index

import "testing"

// TestDeriveTitle covers every edge case in acceptance-criteria.md §3 for the
// title-derivation algorithm (plan.md D4): the first line that is non-empty
// after strings.TrimSpace (which also strips a trailing \r for CRLF bodies),
// skipping any leading blank/whitespace-only lines; "" when no such line
// exists; no blank-line separator required; no truncation.
func TestDeriveTitle(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{"empty body", "", ""},
		{"whitespace-only body", "   \n\n  ", ""},
		{"single line, no trailing newline", "Buy milk", "Buy milk"},
		{"single line, with trailing newline", "Buy milk\n", "Buy milk"},
		{"git-commit shape: subject + blank + body", "Ship it.\n\nDetails here.\n", "Ship it."},
		{"subject with no blank-line separator", "Ship it.\nDetails here.\n", "Ship it."},
		{"leading blank line(s) before subject", "\n\nShip it.\n\nDetails.\n", "Ship it."},
		{"multiple blank lines between subject and body", "Ship it.\n\n\n\nDetails.\n", "Ship it."},
		{"leading/trailing whitespace on the subject line", "  Ship it.  \n\nDetails.\n", "Ship it."},
		{"CRLF body: no trailing \\r on title", "Ship it.\r\n\r\nDetails.\r\n", "Ship it."},
		{
			"5-line blockquote/fence fixture (today_test.go shape)",
			"Ship the report.\n\n> keep this blockquote\n\n```text\nfenced content\n```\n",
			"Ship the report.",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := deriveTitle(tt.body); got != tt.want {
				t.Errorf("deriveTitle(%q) = %q, want %q", tt.body, got, tt.want)
			}
		})
	}
}
