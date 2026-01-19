package cli

import (
	"fmt"
	"strings"

	"github.com/MikeBiancalana/reckon/internal/config"
	"github.com/MikeBiancalana/reckon/internal/logger"
	"github.com/MikeBiancalana/reckon/internal/migrate"
	"github.com/MikeBiancalana/reckon/internal/storage"
	"github.com/spf13/cobra"
)

var migrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Data migration commands for ID system changes",
	Long: `Manage data migration from old ID-based format to new slug-based format.
	
This command helps migrate:
- Task files from {ID}.md to YYYY-MM-DD-slug.md format
- Log entries from database-only to individual markdown files

Backups are automatically created before any migration.`,
}

var migrateRunCmd = &cobra.Command{
	Use:   "run",
	Short: "Run the data migration",
	Long: `Runs the data migration to convert existing reckon data files
from opaque ID-based format to the new slug-based and date-prefixed format.

A backup is created automatically before any changes are made.
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		logger := logger.GetLogger()

		dbPath, err := config.DatabasePath()
		if err != nil {
			return fmt.Errorf("failed to get database path: %w", err)
		}

		db, err := storage.NewDatabase(dbPath)
		if err != nil {
			return fmt.Errorf("failed to open database: %w", err)
		}
		defer db.Close()

		migrator := migrate.NewMigrator(db, logger)

		fmt.Println("Starting migration...")
		fmt.Println("A backup will be created before any changes.")

		result, err := migrator.Run()
		if err != nil {
			fmt.Printf("\n❌ Migration failed: %v\n", err)
			fmt.Printf("\nTo rollback, run: rk migrate rollback %s\n", getBackupID(result.BackupPath))
			return err
		}

		fmt.Println("\n✅ Migration completed successfully!")
		fmt.Printf("  Task files migrated: %d\n", result.TaskFilesMigrated)
		fmt.Printf("  Task files skipped: %d\n", result.TaskFilesSkipped)
		fmt.Printf("  Log entries migrated: %d\n", result.LogEntriesMigrated)
		fmt.Printf("  Log entries skipped: %d\n", result.LogEntriesSkipped)
		fmt.Printf("\nBackup created at: %s\n", result.BackupPath)

		if len(result.Errors) > 0 {
			fmt.Printf("\n⚠️  Warnings during migration:\n")
			for _, e := range result.Errors {
				fmt.Printf("  - %s\n", e)
			}
		}

		return nil
	},
}

var migrateStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check migration status",
	Long: `Checks if migration is needed and shows a summary of what would be migrated.
	
This is a read-only operation that shows:
- Number of task files needing migration
- Number of log entries needing migration
- Any validation issues`,
	RunE: func(cmd *cobra.Command, args []string) error {
		logger := logger.GetLogger()

		dbPath, err := config.DatabasePath()
		if err != nil {
			return fmt.Errorf("failed to get database path: %w", err)
		}

		db, err := storage.NewDatabase(dbPath)
		if err != nil {
			return fmt.Errorf("failed to open database: %w", err)
		}
		defer db.Close()

		tasksDir, err := config.TasksDir()
		if err != nil {
			return fmt.Errorf("failed to get tasks directory: %w", err)
		}

		logDir, err := config.LogDir()
		if err != nil {
			return fmt.Errorf("failed to get log directory: %w", err)
		}

		preCheck, err := migrate.PreMigrationCheck(tasksDir, logDir, db, logger)
		if err != nil {
			return fmt.Errorf("failed to run pre-migration check: %w", err)
		}

		fmt.Println("Migration Status")
		fmt.Println(strings.Repeat("=", 50))
		fmt.Printf("Task files needing migration: %d\n", preCheck.TaskFilesNeedingMigration)
		fmt.Printf("Log entries needing migration: %d\n", preCheck.LogEntriesNeedingMigration)
		fmt.Printf("Orphaned task files: %d\n", len(preCheck.OrphanedTaskFiles))
		fmt.Printf("Orphaned log files: %d\n", len(preCheck.OrphanedLogFiles))

		totalNeedingMigration := preCheck.TaskFilesNeedingMigration + preCheck.LogEntriesNeedingMigration
		if totalNeedingMigration == 0 {
			fmt.Println("\n✅ No migration needed - system is up to date.")
		} else {
			fmt.Printf("\n⚠️  Migration needed - %d items need to be migrated.\n", totalNeedingMigration)
			fmt.Println("Run 'rk migrate run' to perform the migration.")
		}

		return nil
	},
}

var migrateRollbackCmd = &cobra.Command{
	Use:   "rollback <backup-id>",
	Short: "Rollback from a backup",
	Long: `Restores reckon data from a previous backup.

The backup-id is the name of the backup directory (e.g., migration-20260115-120000).
Use 'rk migrate status' to check if rollback is needed.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		backupID := args[0]
		logger := logger.GetLogger()

		fmt.Printf("Rolling back to backup: %s\n", backupID)
		fmt.Println("This will replace all current data with the backup.")

		if err := migrate.Rollback(backupID, logger); err != nil {
			return fmt.Errorf("rollback failed: %w", err)
		}

		fmt.Println("✅ Rollback completed successfully!")
		return nil
	},
}

func init() {
	migrateCmd.AddCommand(migrateRunCmd)
	migrateCmd.AddCommand(migrateStatusCmd)
	migrateCmd.AddCommand(migrateRollbackCmd)
	RootCmd.AddCommand(migrateCmd)
}

func getBackupID(backupPath string) string {
	parts := strings.Split(backupPath, "migration-")
	if len(parts) > 1 {
		return "migration-" + parts[1]
	}
	return ""
}
