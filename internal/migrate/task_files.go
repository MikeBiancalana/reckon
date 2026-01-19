package migrate

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/MikeBiancalana/reckon/internal/storage"
	"gopkg.in/yaml.v3"
)

var taskSlugRegex = regexp.MustCompile(`[^a-z0-9]+`)

type TaskFrontmatter struct {
	ID      string   `yaml:"id"`
	Title   string   `yaml:"title"`
	Created string   `yaml:"created"`
	Status  string   `yaml:"status"`
	Tags    []string `yaml:"tags,omitempty"`
}

func GenerateSlug(text string) string {
	slug := strings.ToLower(text)
	slug = taskSlugRegex.ReplaceAllString(slug, "-")
	slug = strings.Trim(slug, "-")
	if len(slug) > 50 {
		slug = slug[:50]
		slug = strings.Trim(slug, "-")
	}
	if slug == "" {
		slug = "untitled"
	}
	return slug
}

func TaskFilename(task TaskMigrationInfo) string {
	datePrefix := task.CreatedAt.Format("2006-01-02")
	slug := GenerateSlug(task.Title)
	return fmt.Sprintf("%s-%s.md", datePrefix, slug)
}

type TaskMigrationInfo struct {
	ID        string
	Title     string
	CreatedAt time.Time
}

func MigrateTaskFiles(tasksDir string, db *storage.Database, logger *slog.Logger) (migrated, skipped int, err error) {
	files, err := os.ReadDir(tasksDir)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to read tasks directory: %w", err)
	}

	for _, file := range files {
		if !file.Type().IsRegular() || !strings.HasSuffix(file.Name(), ".md") {
			continue
		}

		if isNewTaskFormat(file.Name()) {
			skipped++
			continue
		}

		oldPath := filepath.Join(tasksDir, file.Name())
		content, err := os.ReadFile(oldPath)
		if err != nil {
			logger.Debug("Failed to read task file", "path", oldPath, "error", err)
			continue
		}

		frontmatter, err := parseTaskFrontmatter(string(content))
		if err != nil {
			logger.Debug("Failed to parse task frontmatter", "path", oldPath, "error", err)
			continue
		}

		createdAt := time.Now()
		if frontmatter.Created != "" {
			if parsed, err := time.Parse("2006-01-02", frontmatter.Created); err == nil {
				createdAt = parsed
			}
		}

		taskInfo := TaskMigrationInfo{
			ID:        frontmatter.ID,
			Title:     frontmatter.Title,
			CreatedAt: createdAt,
		}

		newFilename := TaskFilename(taskInfo)
		newPath := filepath.Join(tasksDir, newFilename)

		if oldPath != newPath {
			if err := os.Rename(oldPath, newPath); err != nil {
				logger.Error("Failed to rename task file", "oldPath", oldPath, "newPath", newPath, "error", err)
				continue
			}
			logger.Info("Renamed task file", "from", file.Name(), "to", newFilename)
		}

		migrated++
	}

	return migrated, skipped, nil
}

func parseTaskFrontmatter(content string) (*TaskFrontmatter, error) {
	parts := strings.Split(content, "---")
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid frontmatter format")
	}

	var fm TaskFrontmatter
	if err := yaml.Unmarshal([]byte(parts[1]), &fm); err != nil {
		return nil, err
	}

	return &fm, nil
}

func ValidateTaskFiles(tasksDir string, db *storage.Database, logger *slog.Logger) error {
	files, err := os.ReadDir(tasksDir)
	if err != nil {
		return fmt.Errorf("failed to read tasks directory: %w", err)
	}

	var invalidFiles []string
	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".md") {
			continue
		}

		if !isNewTaskFormat(file.Name()) {
			invalidFiles = append(invalidFiles, file.Name())
		}
	}

	if len(invalidFiles) > 0 {
		return fmt.Errorf("found %d task files not in new format: %v", len(invalidFiles), invalidFiles)
	}

	return nil
}

func CountOldTaskFiles(tasksDir string) (int, error) {
	files, err := os.ReadDir(tasksDir)
	if err != nil {
		return 0, err
	}

	count := 0
	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".md") {
			continue
		}
		if !isNewTaskFormat(file.Name()) {
			count++
		}
	}

	return count, nil
}
