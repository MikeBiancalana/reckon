package cli

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

var scheduleCmd = &cobra.Command{
	Use:   "schedule",
	Short: "Manage daily schedule items",
	Long:  "Add, list, and delete schedule items for the current day.",
}

// scheduleAddCmd adds a new schedule item
var scheduleAddCmd = &cobra.Command{
	Use:   "add [time] <content>",
	Short: "Add a schedule item",
	Long:  `Add a schedule item for today. Time is optional (HH:MM format).`,
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if service == nil {
			return fmt.Errorf("journal service not initialized")
		}

		var timeStr, content string

		if len(args) == 1 {
			// No time specified, content only
			timeStr = ""
			content = args[0]
		} else {
			// First arg is time, rest is content
			timeStr = args[0]
			content = strings.Join(args[1:], " ")
		}

		// Get today's journal
		j, err := service.GetToday()
		if err != nil {
			return fmt.Errorf("failed to get today's journal: %w", err)
		}

		// Add schedule item
		if err := service.AddScheduleItem(j, timeStr, content); err != nil {
			return fmt.Errorf("failed to add schedule item: %w", err)
		}

		if timeStr != "" {
			fmt.Printf("✓ Scheduled: %s at %s\n", content, timeStr)
		} else {
			fmt.Printf("✓ Scheduled: %s\n", content)
		}

		return nil
	},
}

// scheduleListCmd lists schedule items
var scheduleListCmd = &cobra.Command{
	Use:   "list",
	Short: "List schedule items",
	RunE: func(cmd *cobra.Command, args []string) error {
		if service == nil {
			return fmt.Errorf("journal service not initialized")
		}

		// Get today's journal
		j, err := service.GetToday()
		if err != nil {
			return fmt.Errorf("failed to get today's journal: %w", err)
		}

		if len(j.ScheduleItems) == 0 {
			fmt.Println("No schedule items for today")
			return nil
		}

		fmt.Printf("Schedule for today:\n\n")
		for i, item := range j.ScheduleItems {
			if !item.Time.IsZero() {
				fmt.Printf("[%d] %s: %s\n", i+1, item.Time.Format("15:04"), item.Content)
			} else {
				fmt.Printf("[%d] %s\n", i+1, item.Content)
			}
		}

		return nil
	},
}

// scheduleDeleteCmd deletes a schedule item
var scheduleDeleteCmd = &cobra.Command{
	Use:   "delete <index>",
	Short: "Delete a schedule item",
	Long:  "Delete a schedule item by its index number (shown in list command).",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if service == nil {
			return fmt.Errorf("journal service not initialized")
		}

		// Parse index
		index, err := strconv.Atoi(args[0])
		if err != nil || index < 1 {
			return fmt.Errorf("invalid index: %s (must be a positive number)", args[0])
		}

		// Get today's journal
		j, err := service.GetToday()
		if err != nil {
			return fmt.Errorf("failed to get today's journal: %w", err)
		}

		if index > len(j.ScheduleItems) {
			return fmt.Errorf("schedule item %d not found (only %d items)", index, len(j.ScheduleItems))
		}

		// Get the item to delete
		item := j.ScheduleItems[index-1]

		// Delete by ID
		if err := service.DeleteScheduleItem(j, item.ID); err != nil {
			return fmt.Errorf("failed to delete schedule item: %w", err)
		}

		fmt.Printf("✓ Deleted schedule item: %s\n", item.Content)
		return nil
	},
}

func init() {
	// Add subcommands
	scheduleCmd.AddCommand(scheduleAddCmd)
	scheduleCmd.AddCommand(scheduleListCmd)
	scheduleCmd.AddCommand(scheduleDeleteCmd)
}

// GetScheduleCommand returns the schedule command
func GetScheduleCommand() *cobra.Command {
	return scheduleCmd
}
