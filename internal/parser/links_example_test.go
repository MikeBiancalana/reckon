package parser_test

import (
	"fmt"

	"github.com/MikeBiancalana/reckon/internal/parser"
)

// Example demonstrates basic wiki link extraction from markdown content.
func Example() {
	content := `# My Note

This note references [[other-note]] for more details.

See also [[project-plan|Project Planning]] and [[architecture]].`

	links := parser.ExtractWikiLinks(content)

	for _, link := range links {
		if link.DisplayText != "" {
			fmt.Printf("Link to %s (display: %s)\n", link.TargetSlug, link.DisplayText)
		} else {
			fmt.Printf("Link to %s\n", link.TargetSlug)
		}
	}

	// Output:
	// Link to other-note
	// Link to project-plan (display: Project Planning)
	// Link to architecture
}

// ExampleExtractWikiLinks_codeBlocks demonstrates that links in code blocks are excluded.
func ExampleExtractWikiLinks_codeBlocks() {
	content := "# Documentation\n\n" +
		"Use the syntax `[[link]]` to create links.\n\n" +
		"```markdown\n" +
		"[[example-link]]\n" +
		"```\n\n" +
		"Real link: [[actual-link]]"

	links := parser.ExtractWikiLinks(content)

	for _, link := range links {
		fmt.Println(link.TargetSlug)
	}

	// Output:
	// actual-link
}

// ExampleNormalizeSlug demonstrates slug normalization.
func ExampleNormalizeSlug() {
	slugs := []string{
		"My Important Note",
		"UPPER-CASE-SLUG",
		"  spaces  and  hyphens---",
	}

	for _, slug := range slugs {
		normalized := parser.NormalizeSlug(slug)
		fmt.Printf("%q -> %q\n", slug, normalized)
	}

	// Output:
	// "My Important Note" -> "my-important-note"
	// "UPPER-CASE-SLUG" -> "upper-case-slug"
	// "  spaces  and  hyphens---" -> "spaces-and-hyphens"
}
