package cli

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/MikeBiancalana/reckon/internal/checklist"
	"github.com/MikeBiancalana/reckon/internal/config"
	"github.com/MikeBiancalana/reckon/internal/output"
	"github.com/MikeBiancalana/reckon/internal/storage"
	"github.com/spf13/cobra"
)

// ─────────────────────────────────────────────────────────────────────────────
// Flag variables (package-global so cobra can bind them; each subcommand's
// RunE resets them, and their pflag Changed state, via defer resetChecklistFlags).
// ─────────────────────────────────────────────────────────────────────────────

var (
	checklistItemFlag      []string
	checklistItemsFileFlag string
	checklistAllFlag       bool
)

// resetChecklistFlags restores checklist flag variables to their defaults and
// clears the pflag Changed state on whichever of these flags are registered
// on cmd (create/list each register a different subset). Mirrors todo.go's
// resetTodoFlags.
func resetChecklistFlags(cmd *cobra.Command) {
	checklistItemFlag = nil
	checklistItemsFileFlag = ""
	checklistAllFlag = false
	for _, name := range []string{"item", "items-file", "all"} {
		if fl := cmd.Flags().Lookup(name); fl != nil {
			fl.Changed = false
		}
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Commands
// ─────────────────────────────────────────────────────────────────────────────

// checklistCmd is the parent for the checklist template/run family
// (create/list/start/check/status/reset/abandon).
var checklistCmd = &cobra.Command{
	Use:   "checklist",
	Short: "Manage reusable checklist templates and their runs",
	Long: "Create checklist templates and track runs through them " +
		"(create/list/start/check/status/reset/abandon).\n\n" +
		"Checklist template/run state lives only in the local operational " +
		"database (~/.reckon/reckon.db). It is not vault-native, is never " +
		"git-synced, and has no text file as its source of truth. rk index's " +
		"rebuild of the vault-derived index never touches it — a fresh clone " +
		"or a new machine starts with no checklist state at all.",
}

var checklistCreateCmd = &cobra.Command{
	Use:          "create <name>",
	Short:        "Create a new checklist template",
	SilenceUsage: true,
	Args:         cobra.ExactArgs(1),
	RunE:         runChecklistCreateE,
}

var checklistListCmd = &cobra.Command{
	Use:          "list [template]",
	Short:        "List checklist templates, or runs for one template",
	SilenceUsage: true,
	Args:         cobra.MaximumNArgs(1),
	RunE:         runChecklistListE,
}

var checklistStartCmd = &cobra.Command{
	Use:          "start <template>",
	Short:        "Start a new run of a template, or resume its active run",
	SilenceUsage: true,
	Args:         cobra.ExactArgs(1),
	RunE:         runChecklistStartE,
}

var checklistCheckCmd = &cobra.Command{
	Use:          "check <template> <position>",
	Short:        "Toggle a checklist item's checked state",
	SilenceUsage: true,
	Args:         cobra.ExactArgs(2),
	RunE:         runChecklistCheckE,
}

var checklistStatusCmd = &cobra.Command{
	Use:          "status <template>",
	Short:        "Show the active run's status for a template",
	SilenceUsage: true,
	Args:         cobra.ExactArgs(1),
	RunE:         runChecklistStatusE,
}

var checklistResetCmd = &cobra.Command{
	Use:          "reset <template>",
	Short:        "Abandon any active run and start a fresh one",
	SilenceUsage: true,
	Args:         cobra.ExactArgs(1),
	RunE:         runChecklistResetE,
}

var checklistAbandonCmd = &cobra.Command{
	Use:          "abandon <template>",
	Short:        "Abandon the active run for a template",
	SilenceUsage: true,
	Args:         cobra.ExactArgs(1),
	RunE:         runChecklistAbandonE,
}

func init() {
	cf := checklistCreateCmd.Flags()
	cf.StringArrayVar(&checklistItemFlag, "item", nil, "Checklist item text (repeatable)")
	cf.StringVar(&checklistItemsFileFlag, "items-file", "", "Path to a file with one item per line")

	lf := checklistListCmd.Flags()
	lf.BoolVar(&checklistAllFlag, "all", false, "Include completed/abandoned runs (only meaningful with a template argument)")

	checklistCmd.AddCommand(
		checklistCreateCmd,
		checklistListCmd,
		checklistStartCmd,
		checklistCheckCmd,
		checklistStatusCmd,
		checklistResetCmd,
		checklistAbandonCmd,
	)
}

// ─────────────────────────────────────────────────────────────────────────────
// DB/service wiring
// ─────────────────────────────────────────────────────────────────────────────

// openChecklistService opens the operational database and wires up a
// checklist.Service. The caller must defer db.Close().
func openChecklistService() (*checklist.Service, *storage.Database, error) {
	path, err := config.DatabasePath()
	if err != nil {
		return nil, nil, fmt.Errorf("checklist: resolve database path: %w", err)
	}
	db, err := storage.NewDatabase(path)
	if err != nil {
		return nil, nil, fmt.Errorf("checklist: open database: %w", err)
	}
	repo := checklist.NewRepository(db)
	return checklist.NewService(repo), db, nil
}

// setupChecklistRun resolves the requested output mode and opens the
// checklist service; every subcommand RunE starts with this pair. The caller
// must defer db.Close() (only reached once err is nil).
func setupChecklistRun() (output.Mode, *checklist.Service, *storage.Database, error) {
	mode, err := output.ModeFromFlags(jsonFlag, ndjsonFlag)
	if err != nil {
		return 0, nil, nil, err
	}
	svc, db, err := openChecklistService()
	if err != nil {
		return 0, nil, nil, err
	}
	return mode, svc, db, nil
}

// printChecklistResult prints res unless mode is Pretty and --quiet was set;
// mutation verbs (create/start/check/reset/abandon) treat their Pretty
// confirmation as suppressible noise.
func printChecklistResult(cmd *cobra.Command, mode output.Mode, res any) error {
	if mode == output.Pretty && quietFlag {
		return nil
	}
	return output.New(cmd.OutOrStdout(), mode).Print(res)
}

// ─────────────────────────────────────────────────────────────────────────────
// Result types
// ─────────────────────────────────────────────────────────────────────────────

// checklistTemplateResult wraps a created template so its model-tagged fields
// are promoted flat into JSON, with a human Pretty() alongside.
type checklistTemplateResult struct{ *checklist.Template }

func (r checklistTemplateResult) Pretty() string {
	return fmt.Sprintf("checklist: created template %q (%d items)", r.Name, len(r.Items))
}

// checklistRunResult wraps a run for start/check/status/reset/abandon.
// resumed is a Pretty-only discriminator (unexported fields are never
// marshaled, so JSON output stays pure checklist.Run model fields).
type checklistRunResult struct {
	*checklist.Run
	resumed bool
}

func (r checklistRunResult) Pretty() string {
	checked := 0
	for _, it := range r.Items {
		if it.Checked {
			checked++
		}
	}
	var b strings.Builder
	if r.resumed {
		b.WriteString("checklist: resuming existing run\n")
	}
	fmt.Fprintf(&b, "%s  [%d/%d]", r.TemplateName, checked, len(r.Items))
	for _, it := range r.Items {
		mark := " "
		if it.Checked {
			mark = "x"
		}
		fmt.Fprintf(&b, "\n  [%s] %d. %s", mark, it.Position, it.Text)
	}
	switch r.Status {
	case checklist.RunStatusCompleted:
		b.WriteString("\n  ✓ Complete!")
	case checklist.RunStatusAbandoned:
		fmt.Fprintf(&b, "\nchecklist: abandoned %q", r.TemplateName)
	default:
		b.WriteString("\nstatus: active")
	}
	return b.String()
}

// checklistTemplateList is `checklist list`'s bare (no-template-arg) Pretty
// rendering of all templates.
type checklistTemplateList []*checklist.Template

func (l checklistTemplateList) Pretty() string {
	if len(l) == 0 {
		return "checklist: no templates"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "checklist: %d template(s)", len(l))
	for _, t := range l {
		fmt.Fprintf(&b, "\n  %s  (%d items)", t.Name, len(t.Items))
	}
	return b.String()
}

// checklistRunList is `checklist list <template>`'s non-empty Pretty
// rendering of that template's runs. The empty-state message is printed
// directly by the caller (it needs the template name, which an empty slice
// cannot carry).
type checklistRunList []*checklist.Run

func (l checklistRunList) Pretty() string {
	var b strings.Builder
	for i, r := range l {
		checked := 0
		for _, it := range r.Items {
			if it.Checked {
				checked++
			}
		}
		if i > 0 {
			b.WriteString("\n")
		}
		fmt.Fprintf(&b, "  %s  [%d/%d]  started %s", r.Status, checked, len(r.Items), r.StartedAt.Format(time.RFC3339))
	}
	return b.String()
}

// toAny converts a typed slice to []any for output.Writer.PrintAll.
func toAny[T any](items []T) []any {
	out := make([]any, len(items))
	for i, v := range items {
		out[i] = v
	}
	return out
}

// ─────────────────────────────────────────────────────────────────────────────
// create
// ─────────────────────────────────────────────────────────────────────────────

func runChecklistCreateE(cmd *cobra.Command, args []string) error {
	defer resetChecklistFlags(cmd)

	name := args[0]
	itemFlags := checklistItemFlag
	itemsFile := checklistItemsFileFlag

	if len(itemFlags) > 0 && itemsFile != "" {
		return fmt.Errorf("checklist create: --item and --items-file are mutually exclusive")
	}

	items := itemFlags
	if itemsFile != "" {
		parsed, err := parseChecklistItemsFile(itemsFile)
		if err != nil {
			return fmt.Errorf("checklist create: %w", err)
		}
		items = parsed
	}
	if len(items) == 0 {
		return fmt.Errorf("checklist create: at least one item required")
	}

	mode, svc, db, err := setupChecklistRun()
	if err != nil {
		return err
	}
	defer db.Close()

	tpl, err := svc.CreateTemplate(name, items)
	if err != nil {
		return fmt.Errorf("checklist create: %w", err)
	}

	return printChecklistResult(cmd, mode, checklistTemplateResult{Template: tpl})
}

// parseChecklistItemsFile reads path (relative to process cwd), splitting on
// newlines, trimming whitespace, and skipping blank lines while preserving
// file order.
func parseChecklistItemsFile(path string) ([]string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read items file %s: %w", path, err)
	}
	var items []string
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		items = append(items, line)
	}
	return items, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// list
// ─────────────────────────────────────────────────────────────────────────────

func runChecklistListE(cmd *cobra.Command, args []string) error {
	defer resetChecklistFlags(cmd)

	includeCompleted := checklistAllFlag

	mode, svc, db, err := setupChecklistRun()
	if err != nil {
		return err
	}
	defer db.Close()

	if len(args) == 0 {
		templates, err := svc.ListTemplates()
		if err != nil {
			return fmt.Errorf("checklist list: %w", err)
		}
		if templates == nil {
			templates = []*checklist.Template{}
		}
		return printChecklistTemplateList(cmd, mode, templates)
	}

	name := args[0]
	tpl, err := svc.GetTemplate(name)
	if err != nil {
		return fmt.Errorf("checklist list: %w", err)
	}

	allRuns, err := svc.ListRuns(includeCompleted)
	if err != nil {
		return fmt.Errorf("checklist list: %w", err)
	}
	// ListRuns is unscoped across all templates; filter to this one.
	var runs []*checklist.Run
	for _, r := range allRuns {
		if r.TemplateID == tpl.ID {
			runs = append(runs, r)
		}
	}
	if runs == nil {
		runs = []*checklist.Run{}
	}
	return printChecklistRunList(cmd, mode, tpl.Name, runs)
}

func printChecklistTemplateList(cmd *cobra.Command, mode output.Mode, templates []*checklist.Template) error {
	w := output.New(cmd.OutOrStdout(), mode)
	if mode == output.Pretty {
		return w.Print(checklistTemplateList(templates))
	}
	return w.PrintAll(toAny(templates))
}

func printChecklistRunList(cmd *cobra.Command, mode output.Mode, templateName string, runs []*checklist.Run) error {
	w := output.New(cmd.OutOrStdout(), mode)
	if mode == output.Pretty {
		if len(runs) == 0 {
			return w.Print(fmt.Sprintf("checklist: no runs for %q", templateName))
		}
		return w.Print(checklistRunList(runs))
	}
	return w.PrintAll(toAny(runs))
}

// ─────────────────────────────────────────────────────────────────────────────
// start
// ─────────────────────────────────────────────────────────────────────────────

func runChecklistStartE(cmd *cobra.Command, args []string) error {
	defer resetChecklistFlags(cmd)

	name := args[0]

	mode, svc, db, err := setupChecklistRun()
	if err != nil {
		return err
	}
	defer db.Close()

	if _, err := svc.GetTemplate(name); err != nil {
		return fmt.Errorf("checklist start: %w", err)
	}

	resumed := false
	run, err := svc.GetActiveRun(name)
	if err != nil {
		run, err = svc.StartRun(name)
		if err != nil {
			return fmt.Errorf("checklist start: %w", err)
		}
	} else {
		resumed = true
	}

	return printChecklistResult(cmd, mode, checklistRunResult{Run: run, resumed: resumed})
}

// ─────────────────────────────────────────────────────────────────────────────
// check
// ─────────────────────────────────────────────────────────────────────────────

func runChecklistCheckE(cmd *cobra.Command, args []string) error {
	defer resetChecklistFlags(cmd)

	name := args[0]
	pos, err := strconv.Atoi(args[1])
	if err != nil {
		return fmt.Errorf("checklist check: position must be an integer, got %q", args[1])
	}

	mode, svc, db, err := setupChecklistRun()
	if err != nil {
		return err
	}
	defer db.Close()

	run, err := svc.GetActiveRun(name)
	if err != nil {
		return fmt.Errorf("checklist check: %w", err)
	}

	if err := svc.CheckItem(run.ID, pos); err != nil {
		return fmt.Errorf("checklist check: %w", err)
	}

	// Re-fetch by run ID (not GetActiveRun again): the final check on a run
	// auto-completes it, and GetActiveRun would then error not-found.
	updated, err := svc.GetRunStatus(run.ID)
	if err != nil {
		return fmt.Errorf("checklist check: %w", err)
	}

	return printChecklistResult(cmd, mode, checklistRunResult{Run: updated})
}

// ─────────────────────────────────────────────────────────────────────────────
// status
// ─────────────────────────────────────────────────────────────────────────────

func runChecklistStatusE(cmd *cobra.Command, args []string) error {
	defer resetChecklistFlags(cmd)

	name := args[0]

	mode, svc, db, err := setupChecklistRun()
	if err != nil {
		return err
	}
	defer db.Close()

	run, err := svc.GetActiveRun(name)
	if err != nil {
		return fmt.Errorf("checklist status: %w", err)
	}

	// status is a query verb: its data is the requested output, so it always
	// prints regardless of --quiet.
	return output.New(cmd.OutOrStdout(), mode).Print(checklistRunResult{Run: run})
}

// ─────────────────────────────────────────────────────────────────────────────
// reset
// ─────────────────────────────────────────────────────────────────────────────

func runChecklistResetE(cmd *cobra.Command, args []string) error {
	defer resetChecklistFlags(cmd)

	name := args[0]

	mode, svc, db, err := setupChecklistRun()
	if err != nil {
		return err
	}
	defer db.Close()

	run, err := svc.ResetRun(name)
	if err != nil {
		return fmt.Errorf("checklist reset: %w", err)
	}

	return printChecklistResult(cmd, mode, checklistRunResult{Run: run})
}

// ─────────────────────────────────────────────────────────────────────────────
// abandon
// ─────────────────────────────────────────────────────────────────────────────

func runChecklistAbandonE(cmd *cobra.Command, args []string) error {
	defer resetChecklistFlags(cmd)

	name := args[0]

	mode, svc, db, err := setupChecklistRun()
	if err != nil {
		return err
	}
	defer db.Close()

	run, err := svc.AbandonRun(name)
	if err != nil {
		return fmt.Errorf("checklist abandon: %w", err)
	}

	return printChecklistResult(cmd, mode, checklistRunResult{Run: run})
}
