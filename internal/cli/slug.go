package cli

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// maxSlugBytes caps a generated slug's length, matching the legacy
// internal/parser.NormalizeSlug precedent (MaxSlugLength = 200) without
// importing across the legacy/v1 package boundary (plan.md "Where the
// slugify helper lives").
const maxSlugBytes = 200

// reservedSlugs are slugs that would collide with tool-generated artifacts.
// "index" collides with `rk note index`'s generated notes/index.md (or
// notes/<dir>/index.md).
var reservedSlugs = map[string]bool{"index": true}

// slugify converts a title into an Obsidian-filename-safe, lowercase slug
// (plan.md "Slug algorithm + Obsidian compatibility"): trim -> lowercase ->
// keep unicode.IsLetter/IsDigit/-/_ verbatim (no transliteration -- Obsidian
// does none either, so "café" stays "café") -> map runs of whitespace to a
// single '-' -> drop everything else (incidentally stripping both the
// OS-illegal filename set \/:*?"<>| and A#6's forbidden link-control chars
// # | [ ] ^) -> collapse repeated '-' -> trim leading/trailing '-' -> cap at
// ~200 bytes on a rune boundary.
func slugify(title string) string {
	title = strings.ToLower(strings.TrimSpace(title))

	var b strings.Builder
	lastWasSpace := false
	for _, r := range title {
		switch {
		case unicode.IsSpace(r):
			if !lastWasSpace {
				b.WriteByte('-')
				lastWasSpace = true
			}
		case unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_':
			b.WriteRune(r)
			lastWasSpace = false
		default:
			// dropped: punctuation, control chars, wikilink-control chars, etc.
		}
	}

	slug := strings.Trim(collapseHyphens(b.String()), "-")
	if len(slug) > maxSlugBytes {
		slug = strings.Trim(truncateBytes(slug, maxSlugBytes), "-")
	}
	return slug
}

// collapseHyphens replaces runs of consecutive '-' with a single '-'.
func collapseHyphens(s string) string {
	var b strings.Builder
	prevHyphen := false
	for _, r := range s {
		if r == '-' {
			if prevHyphen {
				continue
			}
			prevHyphen = true
		} else {
			prevHyphen = false
		}
		b.WriteRune(r)
	}
	return b.String()
}

// truncateBytes cuts s to at most max bytes without splitting a multi-byte
// rune.
func truncateBytes(s string, max int) string {
	if len(s) <= max {
		return s
	}
	var b strings.Builder
	for _, r := range s {
		if b.Len()+utf8.RuneLen(r) > max {
			break
		}
		b.WriteRune(r)
	}
	return b.String()
}

// validateSlug rejects an empty slug (EC-9: a title that slugifies to
// nothing, e.g. "!!!" or "") or a reserved slug (EC-10: "index", which would
// collide with rk note index's generated catalog file).
func validateSlug(slug string) error {
	if slug == "" {
		return fmt.Errorf("slug: title produced an empty/invalid slug (no letters, digits, '-', or '_' survived)")
	}
	if reservedSlugs[slug] {
		return fmt.Errorf("slug: %q is a reserved slug (collides with the generated index.md)", slug)
	}
	return nil
}
