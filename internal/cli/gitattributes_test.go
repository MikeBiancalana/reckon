package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// findRepoRoot walks up from the test's working directory until it finds the
// directory containing go.mod (the module root, which is the repo root here).
func findRepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("could not locate repo root (go.mod) from working dir")
		}
		dir = parent
	}
}

// AC-4 / T-10: .gitattributes exists at repo root and scopes merge=union to
// log/*.md ONLY — no broader markdown or other-directory union glob.
func TestGitattributes_ScopedToLogOnly(t *testing.T) {
	root := findRepoRoot(t)
	data, err := os.ReadFile(filepath.Join(root, ".gitattributes"))
	if err != nil {
		t.Fatalf("reading .gitattributes: %v", err)
	}
	content := string(data)

	// Must contain the exact log-only union directive.
	if !strings.Contains(content, "log/*.md merge=union") {
		t.Errorf("expected .gitattributes to contain %q, got:\n%s", "log/*.md merge=union", content)
	}

	// Must NOT contain a broader union glob that would catch non-log markdown
	// or other content directories.
	for _, forbidden := range []string{
		"*.md merge=union",
		"**/*.md merge=union",
		"todo/*.md merge=union",
		"notes/*.md merge=union",
		"note/*.md merge=union",
	} {
		// "*.md merge=union" is a substring of "log/*.md merge=union", so check
		// it only when it is NOT part of the allowed log/ directive.
		for _, line := range strings.Split(content, "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "#") || line == "" {
				continue
			}
			if line == forbidden {
				t.Errorf("found forbidden broad union glob %q in .gitattributes", forbidden)
			}
		}
	}
}

// AC-4 / T-11 (integration): git applies merge=union to log/*.md but not to
// other markdown. Skipped when git is unavailable or this is not a work tree.
func TestGitattributes_CheckAttr(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH; skipping check-attr integration test")
	}
	root := findRepoRoot(t)

	run := func(path string) string {
		t.Helper()
		cmd := exec.Command("git", "check-attr", "merge", "--", path)
		cmd.Dir = root
		out, err := cmd.Output()
		if err != nil {
			t.Skipf("git check-attr unavailable (%v); not a work tree?", err)
		}
		return string(out)
	}

	logOut := run("log/2026-01-01.md")
	if !strings.Contains(logOut, "merge: union") {
		t.Errorf("expected log/*.md to have merge: union, got %q", logOut)
	}

	for _, p := range []string{"todo/2026-01-01.md", "notes/foo.md", "README.md"} {
		out := run(p)
		if strings.Contains(out, "merge: union") {
			t.Errorf("expected %s to NOT have merge: union, got %q", p, out)
		}
	}
}
