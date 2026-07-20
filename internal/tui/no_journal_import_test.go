// Package tui holds a static, AST-based check that no file under
// internal/tui (this package plus internal/tui/components) imports
// internal/journal or internal/service. The presentation components mounted
// by the TUI porcelain must stay decoupled from the legacy journal/service
// layers: an import-graph check is exact and automatable where a
// semantic review ("is this journal.Task usage just an inert DTO?") is not,
// so this test operates purely on the parsed import spec, not on how a
// banned type might be used.
//
// This is the first .go file in the internal/tui package itself (only its
// components/ subpackage previously had any).
package tui

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// bannedImports are the two exact import paths forbidden anywhere under
// internal/tui. Only direct imports are checked: a file importing some other
// internal package that itself imports internal/journal transitively is not
// flagged (out of scope for a direct-import graph check).
var bannedImports = []string{
	"github.com/MikeBiancalana/reckon/internal/journal",
	"github.com/MikeBiancalana/reckon/internal/service",
}

// TestNoJournalOrServiceImports walks every .go file under internal/tui
// (this package's directory, ".", which recursively covers ./components too
// since filepath.Walk descends into subdirectories) and parses each file's
// import block via go/parser (ImportsOnly mode — cheap, no need to type-check
// or resolve bodies). Any file importing internal/journal or internal/service
// is an offender; the test fails listing every offender found, so a
// regression names the exact file(s) to fix rather than just "some file
// somewhere."
func TestNoJournalOrServiceImports(t *testing.T) {
	root, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}

	var offenders []string
	walkErr := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}

		fset := token.NewFileSet()
		f, perr := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if perr != nil {
			t.Fatalf("parse imports of %s: %v", path, perr)
		}

		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			rel = path
		}
		rel = filepath.ToSlash(rel)

		for _, imp := range f.Imports {
			importPath, uerr := strconv.Unquote(imp.Path.Value)
			if uerr != nil {
				t.Fatalf("unquote import spec %q in %s: %v", imp.Path.Value, path, uerr)
			}
			if bannedImport(importPath) {
				offenders = append(offenders, rel+": imports "+importPath)
			}
		}
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walk %s: %v", root, walkErr)
	}

	sort.Strings(offenders)
	if len(offenders) > 0 {
		t.Errorf("internal/tui must not import internal/journal or internal/service directly; found %d offender(s):\n%s",
			len(offenders), strings.Join(offenders, "\n"))
	}
}

func bannedImport(importPath string) bool {
	for _, b := range bannedImports {
		if importPath == b {
			return true
		}
	}
	return false
}

// TestNoJournalOrServiceImports_ParsesEveryFile is a narrow sanity check that
// the walk above actually visits more than zero files (a silently-empty walk
// — e.g. from a wrong working-directory assumption — would make
// TestNoJournalOrServiceImports vacuously pass no matter what the tree
// contains, defeating the whole point of an automatable check).
func TestNoJournalOrServiceImports_ParsesEveryFile(t *testing.T) {
	root, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	count := 0
	walkErr := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasSuffix(path, ".go") {
			count++
		}
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walk %s: %v", root, walkErr)
	}
	if count == 0 {
		t.Fatalf("walk visited zero .go files under %s — working-directory assumption is wrong", root)
	}
	// components/ alone (task_list.go, log_view.go, note_picker.go,
	// notes_pane.go, task_picker.go, date_picker.go, ...) is well over a
	// handful of files; a suspiciously low count would also indicate the
	// walk isn't reaching ./components.
	if count < 10 {
		t.Errorf("walk visited only %d .go file(s) under %s, want at least 10 (internal/tui/components alone has more) — suspect the walk is not recursing into components/", count, root)
	}
}
