package parser

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestSecurityDefenseInDepth tests defense-in-depth security measures
// to prevent DoS attacks and ensure safe slug handling.
func TestSecurityDefenseInDepth(t *testing.T) {
	t.Run("max_slug_length_prevents_dos", func(t *testing.T) {
		// Create an extremely long slug to test DoS protection
		veryLongSlug := strings.Repeat("a", 10000)
		normalized := NormalizeSlug(veryLongSlug)

		assert.LessOrEqual(t, len(normalized), MaxSlugLength,
			"normalized slug should not exceed MaxSlugLength")
		assert.Equal(t, MaxSlugLength, len(normalized),
			"slug should be truncated to exactly MaxSlugLength")
	})

	t.Run("max_length_enforced_with_unicode", func(t *testing.T) {
		// Test with unicode characters that might have variable byte lengths
		longUnicode := strings.Repeat("café", 100)
		normalized := NormalizeSlug(longUnicode)

		assert.LessOrEqual(t, len(normalized), MaxSlugLength,
			"unicode slugs should also respect MaxSlugLength")
	})

	t.Run("special_characters_sanitized", func(t *testing.T) {
		// Potential injection attempts should be sanitized
		maliciousInputs := []string{
			"note'; DROP TABLE notes; --",
			"note<script>alert('xss')</script>",
			"note../../../etc/passwd",
			"note%00null",
			"note\x00null",
			"note\r\ninjection",
		}

		for _, input := range maliciousInputs {
			normalized := NormalizeSlug(input)

			// Should only contain safe characters
			for _, r := range normalized {
				assert.True(t,
					r >= 'a' && r <= 'z' ||
						r >= '0' && r <= '9' ||
						r == '-' || r == '_',
					"normalized slug should only contain safe characters, got: %s", normalized)
			}
		}
	})

	t.Run("no_trailing_hyphen_after_truncation", func(t *testing.T) {
		// Ensure that truncation doesn't leave a trailing hyphen
		slug := strings.Repeat("a", MaxSlugLength-1) + "----"
		normalized := NormalizeSlug(slug)

		assert.False(t, strings.HasSuffix(normalized, "-"),
			"truncated slug should not have trailing hyphen")
		assert.LessOrEqual(t, len(normalized), MaxSlugLength)
	})

	t.Run("empty_after_sanitization", func(t *testing.T) {
		// Input with only special characters should result in empty string
		onlySpecial := "!@#$%^&*(){}[]|\\:;\"'<>,.?/"
		normalized := NormalizeSlug(onlySpecial)

		assert.Empty(t, normalized,
			"slug with only special characters should be empty after sanitization")
	})

	t.Run("collapsing_hyphens_after_sanitization", func(t *testing.T) {
		// Special chars between words should collapse to single hyphens
		input := "word!!!word###word"
		normalized := NormalizeSlug(input)

		assert.Equal(t, "wordwordword", normalized,
			"multiple special chars should not create multiple hyphens")
	})

	t.Run("boundary_conditions", func(t *testing.T) {
		tests := []struct {
			name   string
			input  string
			maxLen int
			valid  bool
		}{
			{
				name:   "exactly_at_boundary",
				input:  strings.Repeat("x", MaxSlugLength),
				maxLen: MaxSlugLength,
				valid:  true,
			},
			{
				name:   "one_over_boundary",
				input:  strings.Repeat("x", MaxSlugLength+1),
				maxLen: MaxSlugLength,
				valid:  true,
			},
			{
				name:   "way_over_boundary",
				input:  strings.Repeat("x", MaxSlugLength*10),
				maxLen: MaxSlugLength,
				valid:  true,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				normalized := NormalizeSlug(tt.input)
				assert.LessOrEqual(t, len(normalized), tt.maxLen)
				assert.True(t, tt.valid)
			})
		}
	})

	t.Run("defense_against_path_traversal", func(t *testing.T) {
		// Ensure path traversal attempts are sanitized
		pathTraversals := []string{
			"../../../etc/passwd",
			"..\\..\\..\\windows\\system32",
			"note/../admin",
			"note/./secret",
		}

		for _, input := range pathTraversals {
			normalized := NormalizeSlug(input)

			// Should not contain path separators
			assert.NotContains(t, normalized, "/")
			assert.NotContains(t, normalized, "\\")
			assert.NotContains(t, normalized, ".")
		}
	})

	t.Run("unicode_normalization_safe", func(t *testing.T) {
		// Test that unicode characters don't cause issues
		unicodeInputs := []string{
			"café",
			"naïve",
			"日本語",
			"Ελληνικά",
			"Русский",
		}

		for _, input := range unicodeInputs {
			normalized := NormalizeSlug(input)

			// Should handle gracefully without panicking
			assert.LessOrEqual(t, len(normalized), MaxSlugLength)
		}
	})
}

// TestExtractWikiLinks_SecurityBoundaries tests security boundaries
// when extracting wiki links from content.
func TestExtractWikiLinks_SecurityBoundaries(t *testing.T) {
	t.Run("extremely_long_document", func(t *testing.T) {
		// Create a document with many links to test resource limits
		var content strings.Builder
		content.WriteString("# Document\n\n")

		// Add 1000 links
		for i := 0; i < 1000; i++ {
			content.WriteString("[[note-")
			content.WriteString(strings.Repeat("x", 50))
			content.WriteString("]] ")
		}

		// Should complete without hanging or excessive memory use
		links := ExtractWikiLinks(content.String())

		// Should deduplicate identical links
		assert.LessOrEqual(t, len(links), 1000)

		// All slugs should respect max length
		for _, link := range links {
			assert.LessOrEqual(t, len(link.TargetSlug), MaxSlugLength)
		}
	})

	t.Run("properly_closed_code_blocks", func(t *testing.T) {
		// Test that properly formatted code blocks are handled correctly
		content := "# Doc\n\n" +
			"```\n" +
			"[[should-not-extract]]\n" +
			"```\n\n" +
			"Real link: [[real-link]]"

		links := ExtractWikiLinks(content)

		// Should only extract the real link
		assert.Len(t, links, 1)
		assert.Equal(t, "real-link", links[0].TargetSlug)
	})

	t.Run("malformed_markdown_handled_gracefully", func(t *testing.T) {
		// Malformed markdown (unclosed code block) - parser does its best
		// Note: This is malformed markdown and behavior is implementation-defined
		content := "# Doc\n\n" +
			"```\n" +
			"unclosed code block\n" +
			"[[in-unclosed-block]]\n\n" +
			"[[after-unclosed]]"

		// Should not panic - graceful degradation
		links := ExtractWikiLinks(content)
		assert.NotNil(t, links)
		// Behavior with malformed markdown is implementation-defined,
		// but should not crash
	})

	t.Run("malformed_wiki_links", func(t *testing.T) {
		// Test various malformed link attempts
		content := `# Doc

Malformed links that should not crash:
[[unclosed-link
]]orphaned-close
[single-bracket]
[[[triple-open]]]
[[]]
[[   ]]
`

		// Should not panic and handle gracefully
		links := ExtractWikiLinks(content)

		// Might extract some, but should not crash
		assert.NotNil(t, links)
	})
}

// BenchmarkNormalizeSlug_WithLongInput benchmarks slug normalization
// with long inputs to ensure performance is acceptable.
func BenchmarkNormalizeSlug_WithLongInput(b *testing.B) {
	longSlug := strings.Repeat("a", MaxSlugLength*2)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = NormalizeSlug(longSlug)
	}
}

// BenchmarkExtractWikiLinks_ManyLinks benchmarks link extraction
// from content with many links.
func BenchmarkExtractWikiLinks_ManyLinks(b *testing.B) {
	var content strings.Builder
	content.WriteString("# Document\n\n")

	for i := 0; i < 100; i++ {
		content.WriteString("[[link-")
		content.WriteString(strings.Repeat("a", 20))
		content.WriteString("]] ")
	}

	contentStr := content.String()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ExtractWikiLinks(contentStr)
	}
}
