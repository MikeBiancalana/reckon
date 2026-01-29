# Parser Package

This package provides parsing utilities for Reckon's markdown content.

## Wiki Links

The wiki links parser extracts and normalizes wiki-style links from markdown content, supporting the zettelkasten notes system.

### Supported Syntax

- `[[note-slug]]` - Simple link to another note
- `[[note-slug|Display Text]]` - Link with custom display text

### Features

- **Code Block Exclusion**: Links in fenced code blocks (` ``` `) and inline code (`` ` ``) are automatically excluded
- **Slug Normalization**: All slugs are normalized to lowercase with spaces converted to hyphens
- **Deduplication**: Duplicate links are automatically removed
- **Robust Parsing**: Handles edge cases like empty links, whitespace, and malformed syntax
- **Security Hardening**: Defense-in-depth protections including:
  - Maximum slug length limit (200 characters) to prevent DoS attacks
  - Special character sanitization (only alphanumeric, hyphens, and underscores allowed)
  - Path traversal prevention
  - Unicode safety

### Usage Example

```go
import "github.com/MikeBiancalana/reckon/internal/parser"

content := `# My Note

See [[other-note]] and [[project-plan|Project Planning]].`

links := parser.ExtractWikiLinks(content)

for _, link := range links {
    fmt.Printf("Target: %s\n", link.TargetSlug)
    if link.DisplayText != "" {
        fmt.Printf("Display: %s\n", link.DisplayText)
    }
}
```

### Slug Normalization Rules

1. Convert to lowercase
2. Sanitize: keep only alphanumeric, hyphens, and underscores
3. Replace spaces with hyphens
4. Collapse multiple consecutive hyphens
5. Trim leading/trailing hyphens and whitespace
6. Enforce maximum length (200 characters)

```go
parser.NormalizeSlug("My Important Note")     // returns "my-important-note"
parser.NormalizeSlug("UPPER-CASE")            // returns "upper-case"
parser.NormalizeSlug("  spaces---")           // returns "spaces"
parser.NormalizeSlug("note!@#$%")             // returns "note" (sanitized)
parser.NormalizeSlug(strings.Repeat("a", 300)) // returns 200 'a's (truncated)
```

### Security Considerations

The parser implements defense-in-depth security measures:

**DoS Prevention**: Slugs are limited to 200 characters to prevent resource exhaustion attacks through extremely long inputs.

**Character Sanitization**: Only safe characters (letters, digits, hyphens, underscores) are allowed in normalized slugs. This prevents:
- Path traversal attempts (`../`, `..\\`)
- Special characters that could cause issues (`/`, `\`, `.`, etc.)
- Control characters and null bytes

**Unicode Safety**: Unicode letters (like café, naïve) are preserved, but emoji and other special Unicode characters are removed.

While SQL injection is already prevented by parameterized queries in the database layer, this additional validation provides defense-in-depth protection.

## Testing

Run the tests:

```bash
go test ./internal/parser/
```

Run with coverage:

```bash
go test -cover ./internal/parser/
```

Run benchmarks:

```bash
go test -bench=. ./internal/parser/
```
