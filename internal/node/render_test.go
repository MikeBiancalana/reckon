package node

import (
	"bytes"
	"reflect"
	"testing"
)

// sameView asserts two nodes have the same canonical typed view (not Raw — Raw
// differs between a hand-authored file and its rendered form, which is fine).
func sameView(t *testing.T, label string, a, b *Node) {
	t.Helper()
	if a.ULID != b.ULID || a.Type != b.Type || a.Time != b.Time || a.Author != b.Author {
		t.Fatalf("%s: scalar fields differ\n a=%+v\n b=%+v", label, a, b)
	}
	if a.Body != b.Body {
		t.Fatalf("%s: body differs\n a=%q\n b=%q", label, a.Body, b.Body)
	}
	if !reflect.DeepEqual(a.Aliases, b.Aliases) {
		t.Fatalf("%s: aliases differ: %v vs %v", label, a.Aliases, b.Aliases)
	}
	if !reflect.DeepEqual(a.Props, b.Props) {
		t.Fatalf("%s: props differ: %v vs %v", label, a.Props, b.Props)
	}
	if !reflect.DeepEqual(a.Links, b.Links) {
		t.Fatalf("%s: links differ: %v vs %v", label, a.Links, b.Links)
	}
}

// THE H1 GATE: a node built from typed fields (the create path) renders to
// canonical markdown whose parse reproduces the node, AND the rendered text is
// itself round-trip-stable (so the created node's first EDIT cannot misbehave).
func TestCreateRoundTrip(t *testing.T) {
	n := &Node{
		ULID:    "01J9Z3K7Q2W8XR4M6N0V5BYHED",
		Type:    "todo",
		Time:    "2026-06-24T09:14:03-05:00",
		Author:  "mike",
		Aliases: []string{"buy-milk", "groceries"},
		Props:   map[string]string{"state": "open", "scheduled": "2026-06-25"},
		Links: []Link{
			{Rel: "depends-on", To: "01J9Z2QH8M"},
			{Rel: "references", To: "grocery-plan"}, // body link — must NOT hit frontmatter
		},
		Body: "Buy milk. See [[grocery-plan]] for brands.\n",
	}

	rendered := n.Render()

	// 1. Rendered text parses, and reproduces the typed view (minus the body
	//    "references" link, which the parser re-derives from the body text).
	parsed, err := Parse(rendered)
	if err != nil {
		t.Fatalf("parse(render): %v", err)
	}
	// The body link is re-derived on parse, so compare against the full set.
	want := *n
	if !reflect.DeepEqual(linkSet(parsed.Links), linkSet(n.Links)) {
		t.Fatalf("links not reproduced:\n got %v\n want %v", parsed.Links, n.Links)
	}
	want.Links = parsed.Links // already asserted equal as a set
	sameView(t, "parse(render)", parsed, &want)

	// 2. Rendered text is round-trip stable: serialize(parse(render)) == render.
	if got := parsed.Serialize(); !bytes.Equal(got, rendered) {
		t.Fatalf("rendered text not round-trip stable\n--- render ---\n%q\n--- reparse ---\n%q", rendered, got)
	}

	// 3. Render is idempotent: render(parse(render(n))) == render(n).
	if got := parsed.Render(); !bytes.Equal(got, rendered) {
		t.Fatalf("render not idempotent\n--- 1 ---\n%q\n--- 2 ---\n%q", rendered, got)
	}

	// 4. The rendered frontmatter is Obsidian-shaped: a typed ref is quoted.
	if !bytes.Contains(rendered, []byte(`depends-on: "[[01J9Z2QH8M]]"`)) {
		t.Errorf("typed ref not rendered as a quoted wikilink:\n%s", rendered)
	}
}

// A minted node (no id supplied) round-trips and carries a stable ULID.
func TestNewNodeMintsAndRoundTrips(t *testing.T) {
	n := NewNode("note", "agent", "A fresh note linking [[other]].\n")
	if n.ULID == "" {
		t.Fatal("NewNode did not mint a ULID")
	}
	parsed, err := Parse(n.Render())
	if err != nil {
		t.Fatalf("parse(render): %v", err)
	}
	if parsed.ULID != n.ULID || parsed.Type != "note" || parsed.Author != "agent" {
		t.Fatalf("minted node not reproduced: %+v", parsed)
	}
	if got := parsed.Serialize(); !bytes.Equal(got, n.Render()) {
		t.Fatal("minted node render not round-trip stable")
	}
}

// Bodyless / minimal node still renders and reparses cleanly.
func TestRenderMinimal(t *testing.T) {
	n := &Node{ULID: "01J9Z3K7Q2W8XR4M6N0V5BYHED", Type: "todo", Body: ""}
	parsed, err := Parse(n.Render())
	if err != nil {
		t.Fatalf("parse(render): %v", err)
	}
	if parsed.ULID != n.ULID || parsed.Type != "todo" || parsed.Body != "" {
		t.Fatalf("minimal node not reproduced: %+v", parsed)
	}
	if got := parsed.Render(); !bytes.Equal(got, n.Render()) {
		t.Fatal("minimal render not idempotent")
	}
}

// Round-trip the parse->render->parse loop for a hand-authored file: the second
// parse must match the first parse's typed view, and a further render is stable.
func TestRenderStableFromParsedFile(t *testing.T) {
	src := []byte("---\n" +
		"id: 01J9Z3K7Q2W8XR4M6N0V5BYHEE\n" +
		"type: note\n" +
		"aliases: [my-note]\n" +
		"tags: zettel\n" +
		"depends: \"[[01J9Z2QH8M]]\"\n" +
		"---\n" +
		"Body with [[a-link]] and prose.\n")
	n1, err := Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	r1 := n1.Render()
	n2, err := Parse(r1)
	if err != nil {
		t.Fatalf("parse(render): %v", err)
	}
	sameView(t, "reparse", n2, n1)
	if got := n2.Render(); !bytes.Equal(got, r1) {
		t.Fatalf("render not idempotent across a parsed file\n--- r1 ---\n%q\n--- r2 ---\n%q", r1, got)
	}
}

// TestMultiTargetRenderRoundTrip guards the render-side fix for AC(e): two
// Links sharing one Rel must survive a Render -> Parse round trip as two
// distinct links, not silently collapse into one via a duplicate frontmatter
// key (today, Render emits one `rel: "[[to]]"` line per link, so two
// same-rel links produce two duplicate-keyed lines; parseFrontmatter's
// per-line loop overwrites n.frontmatter[key] on each occurrence, so only the
// LAST link survives reparse).
func TestMultiTargetRenderRoundTrip(t *testing.T) {
	t.Run("two_same_rel_links_survive_round_trip", func(t *testing.T) {
		n := &Node{
			ULID: "01J9Z3K7Q2W8XR4M6N0V5BYHEQ",
			Type: "todo",
			Links: []Link{
				{Rel: "depends", To: "01J9Z2QH8M"},
				{Rel: "depends", To: "01J9Z2QH9N"},
			},
			Body: "Body.\n",
		}

		rendered := n.Render()
		parsed, err := Parse(rendered)
		if err != nil {
			t.Fatalf("parse(render): %v", err)
		}

		var got []Link
		for _, l := range parsed.Links {
			if l.Rel == "depends" {
				got = append(got, l)
			}
		}
		if len(got) != 2 {
			t.Fatalf("want 2 depends links after render+reparse, got %d: %v\nrendered:\n%s", len(got), got, rendered)
		}
		targets := map[string]bool{got[0].To: true, got[1].To: true}
		if !targets["01J9Z2QH8M"] || !targets["01J9Z2QH9N"] {
			t.Errorf("targets not both preserved: %v (rendered:\n%s)", got, rendered)
		}
	})

	// Regression: a single-link rel must keep rendering in the exact current
	// single-line form.
	t.Run("single_link_rel_unchanged", func(t *testing.T) {
		single := &Node{
			ULID:  "01J9Z3K7Q2W8XR4M6N0V5BYHER",
			Type:  "todo",
			Links: []Link{{Rel: "depends-on", To: "01J9Z2QH8M"}},
			Body:  "Body.\n",
		}
		singleRendered := single.Render()
		if !bytes.Contains(singleRendered, []byte(`depends-on: "[[01J9Z2QH8M]]"`)) {
			t.Errorf("single-target rel rendering regressed:\n%s", singleRendered)
		}
	})
}

func linkSet(ls []Link) map[Link]bool {
	m := map[Link]bool{}
	for _, l := range ls {
		m[l] = true
	}
	return m
}
