package cli

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/MikeBiancalana/reckon/internal/checklist"
	"github.com/spf13/cobra"
)

var checklistTemplateFlag string

// GetChecklistCommand returns the checklist command tree.
func GetChecklistCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "checklist",
		Aliases: []string{"cl"},
		Short:   "Manage checklists for recurring procedures",
		Long:    "Create and run checklists for repeatable procedures like morning routines or deployment steps.",
	}

	// template subcommand tree
	templateCmd := &cobra.Command{
		Use:     "template",
		Aliases: []string{"tpl"},
		Short:   "Manage checklist templates",
	}

	templateItemCmd := &cobra.Command{
		Use:   "item",
		Short: "Manage items in a checklist template",
	}

	templateItemCmd.AddCommand(checklistTemplateItemAddCmd())
	templateItemCmd.AddCommand(checklistTemplateItemRemoveCmd())

	templateCmd.AddCommand(checklistTemplateListCmd())
	templateCmd.AddCommand(checklistTemplateAddCmd())
	templateCmd.AddCommand(checklistTemplateShowCmd())
	templateCmd.AddCommand(checklistTemplateDeleteCmd())
	templateCmd.AddCommand(templateItemCmd)

	cmd.AddCommand(templateCmd)
	cmd.AddCommand(checklistStartCmd())
	cmd.AddCommand(checklistCheckCmd())
	cmd.AddCommand(checklistStatusCmd())
	cmd.AddCommand(checklistResetCmd())
	cmd.AddCommand(checklistHistoryCmd())

	return cmd
}

func checklistTemplateListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all checklist templates",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if checklistService == nil {
				return fmt.Errorf("checklist service not initialized")
			}
			templates, err := checklistService.ListTemplates()
			if err != nil {
				return fmt.Errorf("failed to list templates: %w", err)
			}
			if len(templates) == 0 {
				if !quietFlag {
					fmt.Println("No checklist templates. Use 'rk checklist template add <name>' to create one.")
				}
				return nil
			}
			for _, t := range templates {
				fmt.Printf("%-20s  %d items\n", t.Name, len(t.Items))
			}
			return nil
		},
	}
}

func checklistTemplateAddCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "add <name> [item...]",
		Short: "Create a new checklist template",
		Long:  "Create a new checklist template. Optionally provide items as additional arguments.",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if checklistService == nil {
				return fmt.Errorf("checklist service not initialized")
			}
			name := args[0]
			items := args[1:]
			tpl, err := checklistService.CreateTemplate(name, items)
			if err != nil {
				return fmt.Errorf("failed to create template: %w", err)
			}
			if !quietFlag {
				fmt.Printf("✓ Created checklist template %q with %d items\n", tpl.Name, len(tpl.Items))
			} else {
				fmt.Println(tpl.ID)
			}
			return nil
		},
	}
}

func checklistTemplateShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <name>",
		Short: "Show a checklist template and its items",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if checklistService == nil {
				return fmt.Errorf("checklist service not initialized")
			}
			tpl, err := checklistService.GetTemplate(args[0])
			if err != nil {
				return fmt.Errorf("failed to get template: %w", err)
			}
			fmt.Printf("Template: %s\n", tpl.Name)
			fmt.Printf("Items: %d\n", len(tpl.Items))
			if len(tpl.Items) == 0 {
				fmt.Println("  (no items)")
			}
			for _, item := range tpl.Items {
				fmt.Printf("  %d. %s\n", item.Position+1, item.Text)
			}
			return nil
		},
	}
}

func checklistTemplateDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <name>",
		Short: "Delete a checklist template",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if checklistService == nil {
				return fmt.Errorf("checklist service not initialized")
			}
			name := args[0]
			if err := checklistService.DeleteTemplate(name); err != nil {
				return fmt.Errorf("failed to delete template: %w", err)
			}
			if !quietFlag {
				fmt.Printf("✓ Deleted checklist template %q\n", name)
			}
			return nil
		},
	}
}

func checklistTemplateItemAddCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "add <template> <text>",
		Short: "Add an item to a checklist template",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if checklistService == nil {
				return fmt.Errorf("checklist service not initialized")
			}
			templateName := args[0]
			text := strings.Join(args[1:], " ")
			if err := checklistService.AddTemplateItem(templateName, text); err != nil {
				return fmt.Errorf("failed to add item: %w", err)
			}
			if !quietFlag {
				fmt.Printf("✓ Added item %q to template %q\n", text, templateName)
			}
			return nil
		},
	}
}

func checklistTemplateItemRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "remove <template> <position>",
		Short: "Remove an item from a checklist template by 1-based position",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if checklistService == nil {
				return fmt.Errorf("checklist service not initialized")
			}
			templateName := args[0]
			pos1based, err := strconv.Atoi(args[1])
			if err != nil {
				return fmt.Errorf("invalid position %q: must be a number", args[1])
			}
			if pos1based < 1 {
				return fmt.Errorf("position must be 1 or greater")
			}
			if err := checklistService.RemoveTemplateItem(templateName, pos1based-1); err != nil {
				return fmt.Errorf("failed to remove item: %w", err)
			}
			if !quietFlag {
				fmt.Printf("✓ Removed item %d from template %q\n", pos1based, templateName)
			}
			return nil
		},
	}
}

func checklistStartCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "start <template>",
		Short: "Start a new run of a checklist template",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if checklistService == nil {
				return fmt.Errorf("checklist service not initialized")
			}
			run, err := checklistService.StartRun(args[0])
			if err != nil {
				return fmt.Errorf("failed to start run: %w", err)
			}
			if !quietFlag {
				printRunStatus(run)
			} else {
				fmt.Println(run.ID)
			}
			return nil
		},
	}
}

func checklistCheckCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "check <position>",
		Short: "Toggle a checklist item in the active run",
		Long:  "Toggle the checked state of an item by its 1-based position. Use --template if multiple checklists are active.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if checklistService == nil {
				return fmt.Errorf("checklist service not initialized")
			}
			pos1based, err := strconv.Atoi(args[0])
			if err != nil {
				return fmt.Errorf("invalid position %q: must be a number", args[0])
			}
			if pos1based < 1 {
				return fmt.Errorf("position must be 1 or greater")
			}

			if checklistTemplateFlag == "" {
				return fmt.Errorf("--template is required to identify the active run")
			}

			run, err := checklistService.GetActiveRun(checklistTemplateFlag)
			if err != nil {
				return fmt.Errorf("failed to get active run: %w", err)
			}

			if err := checklistService.CheckItem(run.ID, pos1based-1); err != nil {
				return fmt.Errorf("failed to check item: %w", err)
			}

			if !quietFlag {
				updated, err := checklistService.GetRunStatus(run.ID)
				if err != nil {
					return fmt.Errorf("failed to get updated run: %w", err)
				}
				printRunStatus(updated)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&checklistTemplateFlag, "template", "", "Checklist template name (required)")
	return cmd
}

func checklistStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status [template]",
		Short: "Show the status of active checklist runs",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if checklistService == nil {
				return fmt.Errorf("checklist service not initialized")
			}

			if len(args) == 1 {
				run, err := checklistService.GetActiveRun(args[0])
				if err != nil {
					return fmt.Errorf("failed to get run: %w", err)
				}
				printRunStatus(run)
				return nil
			}

			runs, err := checklistService.ListRuns(false)
			if err != nil {
				return fmt.Errorf("failed to list runs: %w", err)
			}
			if len(runs) == 0 {
				if !quietFlag {
					fmt.Println("No active checklist runs.")
				}
				return nil
			}
			for _, run := range runs {
				printRunStatus(run)
				fmt.Println()
			}
			return nil
		},
	}
}

func checklistResetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "reset <template>",
		Short: "Reset the active run of a checklist template",
		Long:  "Abandons any in-progress run and starts a fresh one.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if checklistService == nil {
				return fmt.Errorf("checklist service not initialized")
			}
			run, err := checklistService.ResetRun(args[0])
			if err != nil {
				return fmt.Errorf("failed to reset checklist: %w", err)
			}
			if !quietFlag {
				fmt.Printf("✓ Reset %q — starting fresh\n", args[0])
				printRunStatus(run)
			} else {
				fmt.Println(run.ID)
			}
			return nil
		},
	}
}

func checklistHistoryCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "history",
		Short: "Show completed and abandoned checklist runs",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if checklistService == nil {
				return fmt.Errorf("checklist service not initialized")
			}
			runs, err := checklistService.ListRuns(true)
			if err != nil {
				return fmt.Errorf("failed to list runs: %w", err)
			}
			if len(runs) == 0 {
				if !quietFlag {
					fmt.Println("No checklist run history.")
				}
				return nil
			}
			for _, run := range runs {
				checkedCount := 0
				for _, item := range run.Items {
					if item.Checked {
						checkedCount++
					}
				}
				completedStr := "-"
				if run.CompletedAt != nil {
					completedStr = run.CompletedAt.Format("2006-01-02 15:04")
				}
				fmt.Printf("%-20s  %-10s  %d/%d  started:%s  completed:%s\n",
					run.TemplateName, string(run.Status),
					checkedCount, len(run.Items),
					run.StartedAt.Format("2006-01-02 15:04"),
					completedStr,
				)
			}
			return nil
		},
	}
}

// printRunStatus prints a human-readable checklist run with checkboxes.
func printRunStatus(run *checklist.Run) {
	checkedCount := 0
	for _, item := range run.Items {
		if item.Checked {
			checkedCount++
		}
	}

	fmt.Printf("%s  [%d/%d]\n", run.TemplateName, checkedCount, len(run.Items))

	for _, item := range run.Items {
		mark := "[ ]"
		if item.Checked {
			mark = "[x]"
		}
		fmt.Printf("  %s %d. %s\n", mark, item.Position+1, item.Text)
	}

	if run.Status == checklist.RunStatusCompleted {
		fmt.Println("  ✓ Complete!")
	}
}
