package cli

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/MikeBiancalana/reckon/internal/config"
	"github.com/MikeBiancalana/reckon/internal/index"
	"github.com/MikeBiancalana/reckon/internal/node"
	"github.com/MikeBiancalana/reckon/internal/output"
	"github.com/spf13/cobra"
)

// adoptCmd stamps a freshly minted ULID into id-less markdown files. It is a
// pure filesystem operation on truth files: it never opens, reads, or writes
// the SQLite index (Annotations requiresDB=false) — the index picks up an
// adopted file's new id through its own normal reconcile/rebuild pass, same
// as any other file edit. See docs/design/composable-redesign.md invariant
// #7 for the policy this command implements.
var adoptCmd = &cobra.Command{
	Use:   "adopt <path...>",
	Short: "Stamp a minted ULID into id-less markdown files",
	Long: "Walk the given file/directory paths and, for each id-less markdown file, " +
		"insert a freshly minted `id:` frontmatter line via a span-safe write (every " +
		"other byte of the file is preserved exactly). Files that already have an " +
		"`id:` are left untouched (idempotent no-op). Paths must be inside the vault " +
		"root. This command never touches the index.",
	Annotations:  map[string]string{"requiresDB": "false"},
	SilenceUsage: true,
	Args:         cobra.MinimumNArgs(1),
	RunE:         runAdoptE,
}

// adoptResult is the structured summary of an adopt run, printed via
// internal/output. Paths are reported vault-relative (forward-slash
// separated) so output is stable across machines/temp dirs.
type adoptResult struct {
	Adopted []adoptAdoptedEntry `json:"adopted"`
	Skipped []adoptSkippedEntry `json:"skipped"`
	Errored []adoptErroredEntry `json:"errored"`
}

type adoptAdoptedEntry struct {
	Path string `json:"path"`
	ULID string `json:"ulid"`
}

type adoptSkippedEntry struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

type adoptErroredEntry struct {
	Path  string `json:"path"`
	Error string `json:"error"`
}

func (r *adoptResult) addAdopted(path, ulid string) {
	r.Adopted = append(r.Adopted, adoptAdoptedEntry{Path: path, ULID: ulid})
}

func (r *adoptResult) addSkipped(path, reason string) {
	r.Skipped = append(r.Skipped, adoptSkippedEntry{Path: path, Reason: reason})
}

func (r *adoptResult) addErrored(path string, err error) {
	r.Errored = append(r.Errored, adoptErroredEntry{Path: path, Error: err.Error()})
}

// Pretty renders a human-readable summary line, followed by one indented
// line per adopted/skipped/errored file.
func (r adoptResult) Pretty() string {
	var b strings.Builder
	fmt.Fprintf(&b, "adopt: %d adopted, %d skipped, %d errored", len(r.Adopted), len(r.Skipped), len(r.Errored))
	for _, a := range r.Adopted {
		fmt.Fprintf(&b, "\n  adopted %s -> id %s", a.Path, a.ULID)
	}
	for _, s := range r.Skipped {
		fmt.Fprintf(&b, "\n  skipped %s (%s)", s.Path, s.Reason)
	}
	for _, e := range r.Errored {
		fmt.Fprintf(&b, "\n  error %s: %s", e.Path, e.Error)
	}
	return b.String()
}

func runAdoptE(cmd *cobra.Command, args []string) error {
	mode, err := output.ModeFromFlags(jsonFlag, ndjsonFlag)
	if err != nil {
		return err
	}

	cfg, err := config.LoadWithOverrides(vaultFlag, "")
	if err != nil {
		return fmt.Errorf("adopt: load config: %w", err)
	}

	absVault, err := filepath.Abs(cfg.VaultDir)
	if err != nil {
		return fmt.Errorf("adopt: resolve vault dir: %w", err)
	}

	res := adoptResult{}
	for _, argPath := range args {
		adoptPathArg(absVault, argPath, &res)
	}

	if !(mode == output.Pretty && quietFlag) {
		if err := output.New(cmd.OutOrStdout(), mode).Print(res); err != nil {
			return err
		}
	}

	if len(res.Errored) > 0 {
		return fmt.Errorf("adopt: %d of %d file(s) failed", len(res.Errored), len(res.Adopted)+len(res.Skipped)+len(res.Errored))
	}
	return nil
}

// adoptPathArg resolves one positional argument (file or directory), enforces
// vault containment, and dispatches to either a single-file adopt or a
// recursive directory walk. Outcomes are appended to res; one bad path never
// blocks the others.
func adoptPathArg(absVault, argPath string, res *adoptResult) {
	abs, err := filepath.Abs(argPath)
	if err != nil {
		res.addErrored(argPath, fmt.Errorf("resolve path: %w", err))
		return
	}

	rel, err := filepath.Rel(absVault, abs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		res.addErrored(argPath, fmt.Errorf("path %q is outside the vault root %q", argPath, absVault))
		return
	}
	relDisplay := filepath.ToSlash(rel)

	info, err := os.Stat(abs)
	if err != nil {
		res.addErrored(relDisplay, fmt.Errorf("stat: %w", err))
		return
	}

	if info.IsDir() {
		walkAdoptDir(absVault, abs, res)
		return
	}

	if !strings.HasSuffix(abs, ".md") {
		res.addErrored(relDisplay, fmt.Errorf("not a markdown file (explicit path must end in .md)"))
		return
	}
	adoptOneFile(abs, relDisplay, res)
}

// walkAdoptDir recursively adopts eligible markdown files under dir, applying
// the exact same ignore rules the index uses (index.ShouldSkipDir /
// index.Indexable) so a directory-argument adopt never touches a file the
// index would itself ignore.
func walkAdoptDir(absVault, dir string, res *adoptResult) {
	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			res.addErrored(path, fmt.Errorf("walk: %w", err))
			return nil
		}
		if d.IsDir() {
			if path != dir && index.ShouldSkipDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !index.Indexable(d.Name()) {
			return nil
		}
		rel, err := filepath.Rel(absVault, path)
		if err != nil {
			res.addErrored(path, fmt.Errorf("relativize: %w", err))
			return nil
		}
		adoptOneFile(path, filepath.ToSlash(rel), res)
		return nil
	})
}

// adoptOneFile reads, parses, and (if id-less) stamps a single markdown file
// in place, reporting the outcome into res under relDisplay.
func adoptOneFile(absPath, relDisplay string, res *adoptResult) {
	raw, err := os.ReadFile(absPath)
	if err != nil {
		res.addErrored(relDisplay, fmt.Errorf("read: %w", err))
		return
	}

	// CRLF files parse as body-only today (known parser-scope gap,
	// reckon-vj55); refuse rather than silently mishandle or mix line
	// endings (plan.md D8).
	if bytes.Contains(raw, []byte("\r\n")) {
		res.addErrored(relDisplay, fmt.Errorf("CRLF line endings are not supported (reckon-vj55)"))
		return
	}

	n, err := node.Parse(raw)
	if err != nil {
		res.addErrored(relDisplay, fmt.Errorf("parse: %w", err))
		return
	}

	if n.ULID != "" {
		res.addSkipped(relDisplay, "already has an id")
		return
	}

	id := node.Mint()
	if err := n.InsertField("id", id); err != nil {
		res.addErrored(relDisplay, fmt.Errorf("insert id: %w", err))
		return
	}

	if err := writeFileAtomic(absPath, n.Serialize()); err != nil {
		res.addErrored(relDisplay, fmt.Errorf("write: %w", err))
		return
	}

	res.addAdopted(relDisplay, id)
}

// writeFileAtomic writes data to path via a temp file in the same directory
// (so the rename is same-filesystem, hence atomic) followed by os.Rename over
// the original. On any error the temp file is removed and the original is
// left untouched — a half-written frontmatter block must never corrupt a
// hand-authored truth file (plan.md D4).
func writeFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	mode := os.FileMode(0o644)
	if fi, err := os.Stat(path); err == nil {
		mode = fi.Mode()
	}

	tmp, err := os.CreateTemp(dir, ".adopt-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	ok := false
	defer func() {
		if !ok {
			os.Remove(tmpPath)
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}
	if err := os.Chmod(tmpPath, mode); err != nil {
		return fmt.Errorf("chmod temp file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("rename temp file: %w", err)
	}
	ok = true
	return nil
}
