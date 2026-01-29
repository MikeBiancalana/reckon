package parser

import (
	"regexp"
	"strings"
	"unicode"
)

const (
	// MaxSlugLength is the maximum allowed length for a normalized slug.
	// This prevents DoS attacks via extremely long slugs and ensures
	// reasonable database constraints.
	MaxSlugLength = 200
)

// WikiLink represents a parsed wiki-style link from markdown content.
type WikiLink struct {
	TargetSlug  string // The normalized slug extracted from the link
	DisplayText string // The display text (empty if not provided)
	RawLink     string // The original link text for debugging
}

// codeBlockPattern matches fenced code blocks with optional language specifier.
var codeBlockPattern = regexp.MustCompile("(?s)```[^`]*```")

// inlineCodePattern matches inline code spans.
var inlineCodePattern = regexp.MustCompile("`[^`]+`")

// wikiLinkPattern matches wiki-style links with optional display text.
// Matches: [[target-slug]] or [[target-slug|display text]]
var wikiLinkPattern = regexp.MustCompile(`\[\[([^|\]]+)(?:\|([^\]]+))?\]\]`)

// ExtractWikiLinks parses markdown content and extracts all wiki-style links,
// excluding links found within code blocks (both fenced and inline).
//
// The function supports two syntaxes:
//   - [[note-slug]] - simple link with no display text
//   - [[note-slug|display text]] - link with custom display text
//
// Slugs are normalized to lowercase with spaces replaced by hyphens.
// Empty or whitespace-only links are skipped.
func ExtractWikiLinks(content string) []WikiLink {
	// First, remove all code blocks to prevent extracting links from code
	contentWithoutCode := removeCodeBlocks(content)

	// Find all wiki-link matches
	matches := wikiLinkPattern.FindAllStringSubmatch(contentWithoutCode, -1)
	if matches == nil {
		return nil
	}

	links := make([]WikiLink, 0, len(matches))
	seen := make(map[string]bool) // Track unique slugs

	for _, match := range matches {
		if len(match) < 2 {
			continue
		}

		targetSlug := strings.TrimSpace(match[1])
		if targetSlug == "" {
			continue // Skip empty links
		}

		// Normalize the slug
		normalizedSlug := NormalizeSlug(targetSlug)
		if normalizedSlug == "" {
			continue // Skip if normalization results in empty string
		}

		// Skip duplicates
		if seen[normalizedSlug] {
			continue
		}
		seen[normalizedSlug] = true

		displayText := ""
		if len(match) > 2 {
			displayText = strings.TrimSpace(match[2])
		}

		links = append(links, WikiLink{
			TargetSlug:  normalizedSlug,
			DisplayText: displayText,
			RawLink:     match[0],
		})
	}

	return links
}

// NormalizeSlug converts a slug to lowercase and replaces spaces with hyphens.
// It also trims leading/trailing whitespace and hyphens, collapses multiple
// consecutive hyphens into a single hyphen, and enforces the maximum slug length.
//
// For defense-in-depth, this function also sanitizes special characters by
// keeping only alphanumeric characters, hyphens, and underscores.
//
// If the normalized slug exceeds MaxSlugLength, it is truncated and any
// trailing hyphen is removed.
func NormalizeSlug(slug string) string {
	// Trim whitespace
	slug = strings.TrimSpace(slug)
	if slug == "" {
		return ""
	}

	// Convert to lowercase
	slug = strings.ToLower(slug)

	// Sanitize: keep only alphanumeric, hyphens, underscores, and spaces
	// Spaces will be converted to hyphens in the next step
	var sanitized strings.Builder
	sanitized.Grow(len(slug))
	for _, r := range slug {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' || r == ' ' {
			sanitized.WriteRune(r)
		}
	}
	slug = sanitized.String()

	// Replace spaces with hyphens
	slug = strings.ReplaceAll(slug, " ", "-")

	// Collapse multiple consecutive hyphens
	for strings.Contains(slug, "--") {
		slug = strings.ReplaceAll(slug, "--", "-")
	}

	// Trim leading/trailing hyphens
	slug = strings.Trim(slug, "-")

	// Enforce maximum length
	if len(slug) > MaxSlugLength {
		slug = slug[:MaxSlugLength]
		// Trim any trailing hyphen after truncation
		slug = strings.TrimRight(slug, "-")
	}

	return slug
}

// removeCodeBlocks removes both fenced code blocks (```) and inline code (`)
// from the content to prevent extracting links from code.
func removeCodeBlocks(content string) string {
	// First remove fenced code blocks
	content = codeBlockPattern.ReplaceAllString(content, "")

	// Then remove inline code
	content = inlineCodePattern.ReplaceAllString(content, "")

	return content
}
