package parser

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractWikiLinks(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected []WikiLink
	}{
		{
			name:     "no links",
			content:  "This is just plain text with no links.",
			expected: nil,
		},
		{
			name:    "simple link",
			content: "Check out [[my-note]] for details.",
			expected: []WikiLink{
				{TargetSlug: "my-note", DisplayText: "", RawLink: "[[my-note]]"},
			},
		},
		{
			name:    "link with display text",
			content: "Read more about [[project-alpha|Project Alpha]].",
			expected: []WikiLink{
				{TargetSlug: "project-alpha", DisplayText: "Project Alpha", RawLink: "[[project-alpha|Project Alpha]]"},
			},
		},
		{
			name:    "multiple links",
			content: "See [[note-one]] and [[note-two|Note Two]] for context.",
			expected: []WikiLink{
				{TargetSlug: "note-one", DisplayText: "", RawLink: "[[note-one]]"},
				{TargetSlug: "note-two", DisplayText: "Note Two", RawLink: "[[note-two|Note Two]]"},
			},
		},
		{
			name:    "link with spaces - normalized to hyphens",
			content: "Check [[My Important Note]] here.",
			expected: []WikiLink{
				{TargetSlug: "my-important-note", DisplayText: "", RawLink: "[[My Important Note]]"},
			},
		},
		{
			name:    "link with uppercase - normalized to lowercase",
			content: "See [[IMPORTANT-NOTE]] for info.",
			expected: []WikiLink{
				{TargetSlug: "important-note", DisplayText: "", RawLink: "[[IMPORTANT-NOTE]]"},
			},
		},
		{
			name:     "empty link - should be skipped",
			content:  "This has an [[ ]] empty link.",
			expected: []WikiLink{},
		},
		{
			name:     "whitespace-only link - should be skipped",
			content:  "This has [[   ]] whitespace link.",
			expected: []WikiLink{},
		},
		{
			name:    "duplicate links - should deduplicate",
			content: "Multiple refs to [[note-a]] and [[note-a]] again.",
			expected: []WikiLink{
				{TargetSlug: "note-a", DisplayText: "", RawLink: "[[note-a]]"},
			},
		},
		{
			name:    "duplicate normalized slugs - should deduplicate",
			content: "Refs to [[Note A]] and [[note-a]] (same after normalization).",
			expected: []WikiLink{
				{TargetSlug: "note-a", DisplayText: "", RawLink: "[[Note A]]"},
			},
		},
		{
			name: "links in fenced code block - should be excluded",
			content: `Here's some code:
` + "```" + `
[[not-a-real-link]]
` + "```" + `
But [[real-link]] is outside code.`,
			expected: []WikiLink{
				{TargetSlug: "real-link", DisplayText: "", RawLink: "[[real-link]]"},
			},
		},
		{
			name: "links in fenced code block with language - should be excluded",
			content: `Example code:
` + "```go" + `
// This [[link-in-code]] should not be extracted
func example() {}
` + "```" + `
But [[actual-link]] should be extracted.`,
			expected: []WikiLink{
				{TargetSlug: "actual-link", DisplayText: "", RawLink: "[[actual-link]]"},
			},
		},
		{
			name:    "links in inline code - should be excluded",
			content: "Use `[[inline-code-link]]` syntax, but [[real-link]] works.",
			expected: []WikiLink{
				{TargetSlug: "real-link", DisplayText: "", RawLink: "[[real-link]]"},
			},
		},
		{
			name: "mixed code blocks and inline code",
			content: `Some text with ` + "`[[inline]]`" + ` code.

` + "```" + `
[[fenced-link]]
` + "```" + `

Normal [[valid-link]] here.`,
			expected: []WikiLink{
				{TargetSlug: "valid-link", DisplayText: "", RawLink: "[[valid-link]]"},
			},
		},
		{
			name:    "link with extra whitespace",
			content: "Check [[  spaced-note  ]] out.",
			expected: []WikiLink{
				{TargetSlug: "spaced-note", DisplayText: "", RawLink: "[[  spaced-note  ]]"},
			},
		},
		{
			name:    "link with display text containing spaces",
			content: "Read [[api-docs|API Documentation]] for details.",
			expected: []WikiLink{
				{TargetSlug: "api-docs", DisplayText: "API Documentation", RawLink: "[[api-docs|API Documentation]]"},
			},
		},
		{
			name: "multiple links on different lines",
			content: `Line 1 has [[link-one]]
Line 2 has [[link-two]]
Line 3 has [[link-three|Display Three]]`,
			expected: []WikiLink{
				{TargetSlug: "link-one", DisplayText: "", RawLink: "[[link-one]]"},
				{TargetSlug: "link-two", DisplayText: "", RawLink: "[[link-two]]"},
				{TargetSlug: "link-three", DisplayText: "Display Three", RawLink: "[[link-three|Display Three]]"},
			},
		},
		{
			name:    "links at start and end of content",
			content: "[[start-link]] content in middle [[end-link]]",
			expected: []WikiLink{
				{TargetSlug: "start-link", DisplayText: "", RawLink: "[[start-link]]"},
				{TargetSlug: "end-link", DisplayText: "", RawLink: "[[end-link]]"},
			},
		},
		{
			name:    "multiple consecutive hyphens - should be collapsed",
			content: "Check [[my---note---slug]] here.",
			expected: []WikiLink{
				{TargetSlug: "my-note-slug", DisplayText: "", RawLink: "[[my---note---slug]]"},
			},
		},
		{
			name:    "leading and trailing hyphens - should be trimmed",
			content: "See [[--my-note--]] for info.",
			expected: []WikiLink{
				{TargetSlug: "my-note", DisplayText: "", RawLink: "[[--my-note--]]"},
			},
		},
		{
			name: "real-world zettelkasten example",
			content: `# Project Planning

This connects to [[project-charter|Project Charter]] and [[stakeholder-analysis]].

Implementation notes:
- See [[technical-architecture]] for system design
- Reference [[api-design|API Design]] for interface specs

` + "```python" + `
# This [[not-extracted]] is in code
def get_link():
    return "[[also-not-extracted]]"
` + "```" + `

Related: [[project-retrospective]]`,
			expected: []WikiLink{
				{TargetSlug: "project-charter", DisplayText: "Project Charter", RawLink: "[[project-charter|Project Charter]]"},
				{TargetSlug: "stakeholder-analysis", DisplayText: "", RawLink: "[[stakeholder-analysis]]"},
				{TargetSlug: "technical-architecture", DisplayText: "", RawLink: "[[technical-architecture]]"},
				{TargetSlug: "api-design", DisplayText: "API Design", RawLink: "[[api-design|API Design]]"},
				{TargetSlug: "project-retrospective", DisplayText: "", RawLink: "[[project-retrospective]]"},
			},
		},
		{
			name:    "very long slug is truncated",
			content: "See [[" + strings.Repeat("a", MaxSlugLength+50) + "]] for details.",
			expected: []WikiLink{
				{TargetSlug: strings.Repeat("a", MaxSlugLength), DisplayText: "", RawLink: "[[" + strings.Repeat("a", MaxSlugLength+50) + "]]"},
			},
		},
		{
			name:    "link with special characters sanitized",
			content: "Check [[my-note!@#$]] here.",
			expected: []WikiLink{
				{TargetSlug: "my-note", DisplayText: "", RawLink: "[[my-note!@#$]]"},
			},
		},
		{
			name:    "link with emoji removed",
			content: "See [[my-note-😀-test]] and [[another-🎉-note]].",
			expected: []WikiLink{
				{TargetSlug: "my-note-test", DisplayText: "", RawLink: "[[my-note-😀-test]]"},
				{TargetSlug: "another-note", DisplayText: "", RawLink: "[[another-🎉-note]]"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractWikiLinks(tt.content)

			require.Equal(t, len(tt.expected), len(result), "link count mismatch")

			for i, expected := range tt.expected {
				assert.Equal(t, expected.TargetSlug, result[i].TargetSlug, "slug mismatch at index %d", i)
				assert.Equal(t, expected.DisplayText, result[i].DisplayText, "display text mismatch at index %d", i)
				assert.Equal(t, expected.RawLink, result[i].RawLink, "raw link mismatch at index %d", i)
			}
		})
	}
}

func TestNormalizeSlug(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "already normalized",
			input:    "my-note-slug",
			expected: "my-note-slug",
		},
		{
			name:     "uppercase to lowercase",
			input:    "MY-NOTE-SLUG",
			expected: "my-note-slug",
		},
		{
			name:     "spaces to hyphens",
			input:    "my note slug",
			expected: "my-note-slug",
		},
		{
			name:     "mixed case with spaces",
			input:    "My Important Note",
			expected: "my-important-note",
		},
		{
			name:     "leading and trailing spaces",
			input:    "  my-note  ",
			expected: "my-note",
		},
		{
			name:     "multiple consecutive spaces",
			input:    "my    note    slug",
			expected: "my-note-slug",
		},
		{
			name:     "multiple consecutive hyphens",
			input:    "my---note---slug",
			expected: "my-note-slug",
		},
		{
			name:     "leading hyphens",
			input:    "---my-note",
			expected: "my-note",
		},
		{
			name:     "trailing hyphens",
			input:    "my-note---",
			expected: "my-note",
		},
		{
			name:     "leading and trailing hyphens",
			input:    "---my-note---",
			expected: "my-note",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "whitespace only",
			input:    "   ",
			expected: "",
		},
		{
			name:     "hyphens only",
			input:    "---",
			expected: "",
		},
		{
			name:     "complex mixed input",
			input:    "  My---Complex    NOTE  Slug  ",
			expected: "my-complex-note-slug",
		},
		{
			name:     "special characters removed",
			input:    "my-note!@#$%^&*()",
			expected: "my-note",
		},
		{
			name:     "unicode letters preserved",
			input:    "café-résumé",
			expected: "café-résumé",
		},
		{
			name:     "special chars with spaces",
			input:    "my @ note # with $ special",
			expected: "my-note-with-special",
		},
		{
			name:     "underscores preserved",
			input:    "my_note_with_underscores",
			expected: "my_note_with_underscores",
		},
		{
			name:     "mixed underscores and hyphens",
			input:    "my_note-slug_test",
			expected: "my_note-slug_test",
		},
		{
			name:     "parentheses and brackets removed",
			input:    "note(with)[brackets]{and}parens",
			expected: "notewithbracketsandparens",
		},
		{
			name:     "dots and commas removed",
			input:    "note.with.dots,and,commas",
			expected: "notewithdotsandcommas",
		},
		{
			name:     "exactly max length",
			input:    strings.Repeat("a", MaxSlugLength),
			expected: strings.Repeat("a", MaxSlugLength),
		},
		{
			name:     "exceeds max length by 1",
			input:    strings.Repeat("a", MaxSlugLength+1),
			expected: strings.Repeat("a", MaxSlugLength),
		},
		{
			name:     "exceeds max length by 50",
			input:    strings.Repeat("a", MaxSlugLength+50),
			expected: strings.Repeat("a", MaxSlugLength),
		},
		{
			name:     "long slug with hyphens at end",
			input:    strings.Repeat("a", MaxSlugLength-3) + "---",
			expected: strings.Repeat("a", MaxSlugLength-3),
		},
		{
			name:     "very long slug truncated",
			input:    strings.Repeat("abcd-", 100), // 500 chars
			expected: strings.TrimRight(strings.Repeat("abcd-", 100)[:MaxSlugLength], "-"),
		},
		{
			name:     "long slug with trailing hyphen after truncation",
			input:    strings.Repeat("abc-", 100), // Will be truncated
			expected: strings.TrimRight(strings.Repeat("abc-", 100)[:MaxSlugLength], "-"),
		},
		{
			name:     "all special characters",
			input:    "!@#$%^&*(){}[]|\\:;\"'<>,.?/",
			expected: "",
		},
		{
			name:     "emoji and special unicode removed",
			input:    "my-note-😀-emoji-🎉",
			expected: "my-note-emoji",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NormalizeSlug(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestRemoveCodeBlocks(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected string
	}{
		{
			name:     "no code blocks",
			content:  "Just plain text",
			expected: "Just plain text",
		},
		{
			name:     "fenced code block",
			content:  "Before\n" + "```\ncode here\n```" + "\nAfter",
			expected: "Before\n\nAfter",
		},
		{
			name:     "fenced code block with language",
			content:  "Before\n" + "```go\nfunc main() {}\n```" + "\nAfter",
			expected: "Before\n\nAfter",
		},
		{
			name:     "inline code",
			content:  "Use `inline code` here",
			expected: "Use  here",
		},
		{
			name:     "mixed fenced and inline",
			content:  "Text with `inline` code\n" + "```\nfenced\n```" + "\nMore text",
			expected: "Text with  code\n\nMore text",
		},
		{
			name:     "multiple fenced blocks",
			content:  "```\nblock1\n```\ntext\n```\nblock2\n```",
			expected: "\ntext\n",
		},
		{
			name:     "multiple inline code spans",
			content:  "Use `code1` and `code2` here",
			expected: "Use  and  here",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := removeCodeBlocks(tt.content)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// Benchmark tests
func BenchmarkExtractWikiLinks(b *testing.B) {
	content := `# Test Document

This is a test with [[link-one]] and [[link-two|Link Two]].

` + "```go" + `
// [[code-link]] should not be extracted
func example() {}
` + "```" + `

More links: [[link-three]] and [[link-four|Link Four]].

Inline ` + "`[[inline-link]]`" + ` should be ignored.

Final links: [[link-five]] and [[link-six]].`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ExtractWikiLinks(content)
	}
}

func BenchmarkNormalizeSlug(b *testing.B) {
	slug := "  My---Complex    NOTE  Slug  "

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = NormalizeSlug(slug)
	}
}
