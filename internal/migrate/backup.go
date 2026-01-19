package migrate

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/MikeBiancalana/reckon/internal/config"
)

func CreateBackup(logger *slog.Logger) (string, error) {
	dataDir, err := config.DataDir()
	if err != nil {
		return "", fmt.Errorf("failed to get data directory: %w", err)
	}

	backupDir, err := getBackupDir()
	if err != nil {
		return "", fmt.Errorf("failed to create backup directory: %w", err)
	}

	logger.Info("Creating backup", "source", dataDir, "destination", backupDir)

	if err := copyDirectory(dataDir, backupDir, logger); err != nil {
		return "", fmt.Errorf("failed to copy data to backup: %w", err)
	}

	if err := verifyBackup(dataDir, backupDir, logger); err != nil {
		return "", fmt.Errorf("backup verification failed: %w", err)
	}

	logger.Info("Backup created and verified", "path", backupDir)
	return backupDir, nil
}

func getBackupDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	backupDir := filepath.Join(home, ".reckon", "backups", "migration-"+time.Now().Format("20060102-150405"))
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return "", err
	}

	return backupDir, nil
}

func copyDirectory(src, dst string, logger *slog.Logger) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if relPath == "." {
			return nil
		}

		dstPath := filepath.Join(dst, relPath)

		if info.IsDir() {
			return os.MkdirAll(dstPath, info.Mode())
		}

		if !info.Mode().IsRegular() {
			return nil
		}

		srcFile, err := os.Open(path)
		if err != nil {
			return err
		}
		defer srcFile.Close()

		dstFile, err := os.OpenFile(dstPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode())
		if err != nil {
			return err
		}
		defer dstFile.Close()

		logger.Debug("Copying file", "src", path, "dst", dstPath)

		buf := make([]byte, 32*1024)
		for {
			n, err := srcFile.Read(buf)
			if n > 0 {
				if _, writeErr := dstFile.Write(buf[:n]); writeErr != nil {
					return writeErr
				}
			}
			if err != nil {
				if err.Error() == "EOF" {
					break
				}
				return err
			}
		}

		return nil
	})
}

func verifyBackup(src, dst string, logger *slog.Logger) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if relPath == "." {
			return nil
		}

		dstPath := filepath.Join(dst, relPath)

		if info.IsDir() {
			if _, err := os.Stat(dstPath); os.IsNotExist(err) {
				return fmt.Errorf("directory missing in backup: %s", dstPath)
			}
			return nil
		}

		if !info.Mode().IsRegular() {
			return nil
		}

		dstInfo, err := os.Stat(dstPath)
		if err != nil {
			return fmt.Errorf("file missing in backup: %s", dstPath)
		}

		if dstInfo.Size() != info.Size() {
			return fmt.Errorf("file size mismatch: %s (src: %d, dst: %d)", dstPath, info.Size(), dstInfo.Size())
		}

		return nil
	})
}

func Rollback(backupID string, logger *slog.Logger) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	backupPath := filepath.Join(home, ".reckon", "backups", backupID)
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		return fmt.Errorf("backup not found: %s", backupID)
	}

	dataDir, err := config.DataDir()
	if err != nil {
		return fmt.Errorf("failed to get data directory: %w", err)
	}

	logger.Info("Performing rollback", "backup", backupPath, "destination", dataDir)

	if err := os.RemoveAll(dataDir); err != nil {
		return fmt.Errorf("failed to remove current data: %w", err)
	}

	if err := copyDirectory(backupPath, dataDir, logger); err != nil {
		return fmt.Errorf("failed to restore from backup: %w", err)
	}

	logger.Info("Rollback complete", "from", backupPath)
	return nil
}
