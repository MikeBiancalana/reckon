package cli

import (
	"fmt"

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

		if _, err := ix.Rebuild(); err != nil {
			return fmt.Errorf("index: rebuild: %w", err)
		}

		res, err := indexSummary(ix, cfg)
		if err != nil {
			return err
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

// Pretty renders the human-readable status line (output.Writer prefers this).
func (r indexResult) Pretty() string {
	return fmt.Sprintf("Rebuilt index %s: %d nodes, %d edges, %d aliases",
		r.VaultID, r.Nodes, r.Edges, r.Aliases)
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
