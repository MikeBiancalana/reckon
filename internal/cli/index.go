package cli

import (
	"fmt"
	"strings"

	"github.com/MikeBiancalana/reckon/internal/config"
	"github.com/MikeBiancalana/reckon/internal/index"
	"github.com/MikeBiancalana/reckon/internal/output"
	"github.com/spf13/cobra"
)

// indexCmd builds/rebuilds the per-device property-graph index from the vault.
// It is requiresDB=false: the index is its own SQLite store in the cache dir,
// independent of the legacy operational database.
var indexCmd = &cobra.Command{
	Use:   "index",
	Short: "Build or rebuild the vault index",
	Long: "Rebuild the per-device property-graph index cache from the vault text. " +
		"The index is derived and disposable; this performs a full, deterministic rebuild.",
	Annotations: map[string]string{"requiresDB": "false"},
	RunE: func(cmd *cobra.Command, args []string) error {
		mode, err := output.ModeFromFlags(jsonFlag, ndjsonFlag)
		if err != nil {
			return err
		}

		cfg, err := config.LoadWithOverrides(vaultFlag, "")
		if err != nil {
			return fmt.Errorf("index: load config: %w", err)
		}

		ix, err := index.Open(cfg)
		if err != nil {
			return fmt.Errorf("index: open: %w", err)
		}
		defer ix.Close()

		st, err := ix.Rebuild()
		if err != nil {
			return fmt.Errorf("index: rebuild: %w", err)
		}

		res, err := indexSummary(ix, cfg)
		if err != nil {
			return err
		}
		res.Warnings = st.Warnings
		if res.Warnings == nil {
			res.Warnings = []index.Warning{}
		}

		// In pretty mode --quiet suppresses the status line; structured output is
		// the requested data, so --json/--ndjson always emit.
		if mode == output.Pretty && quietFlag {
			return nil
		}
		return output.New(cmd.OutOrStdout(), mode).Print(res)
	},
}

// indexResult is the structured summary of a rebuild.
type indexResult struct {
	VaultID  string          `json:"vault_id"`
	Nodes    int             `json:"nodes"`
	Edges    int             `json:"edges"`
	Aliases  int             `json:"aliases"`
	Warnings []index.Warning `json:"warnings"`
}

// Pretty renders the human-readable status line (output.Writer prefers this),
// followed by one indented line per warning when the pass found any.
func (r indexResult) Pretty() string {
	line := fmt.Sprintf("Rebuilt index %s: %d nodes, %d edges, %d aliases",
		r.VaultID, r.Nodes, r.Edges, r.Aliases)
	if len(r.Warnings) == 0 {
		return line
	}

	var b strings.Builder
	b.WriteString(line)
	fmt.Fprintf(&b, "\nwarnings (%d):", len(r.Warnings))
	for _, w := range r.Warnings {
		b.WriteString("\n  ")
		switch w.Kind {
		case "duplicate_ulid":
			fmt.Fprintf(&b, "duplicate ULID %s: %s", w.ULID, strings.Join(w.Files, ", "))
		case "alias_collision":
			fmt.Fprintf(&b, "alias %q on %d nodes: %s", w.Alias, len(w.NodeKeys), strings.Join(w.Files, ", "))
		default:
			b.WriteString(w.Kind)
		}
	}
	return b.String()
}

func indexSummary(ix *index.Index, cfg *config.Config) (indexResult, error) {
	res := indexResult{}
	vid, err := ix.Meta("vault_id")
	if err != nil {
		return res, err
	}
	res.VaultID = vid
	for view, dst := range map[string]*int{"nodes": &res.Nodes, "edges": &res.Edges, "aliases": &res.Aliases} {
		if err := ix.DB().QueryRow("SELECT count(*) FROM " + view).Scan(dst); err != nil {
			return res, fmt.Errorf("index: count %s: %w", view, err)
		}
	}
	return res, nil
}
