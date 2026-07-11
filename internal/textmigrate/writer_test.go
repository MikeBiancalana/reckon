package textmigrate

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// Given a destination path with no existing file, writeFileAtomic creates
// it with exactly the given bytes.
func TestWriteFileAtomic_CreatesNewFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "new.md")

	err := writeFileAtomic(path, []byte("hello\n"))
	require.NoError(t, err)

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "hello\n", string(got))
}

// Given a destination path with an existing file, writeFileAtomic replaces
// its content wholesale rather than appending or partially overwriting.
func TestWriteFileAtomic_ReplacesExistingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "existing.md")
	require.NoError(t, os.WriteFile(path, []byte("old content\n"), 0o644))

	err := writeFileAtomic(path, []byte("new content\n"))
	require.NoError(t, err)

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "new content\n", string(got))
}

// writeFileAtomic must never leave a stray temp file behind in the target
// directory, on success or on failure.
func TestWriteFileAtomic_LeavesNoTempFileBehind(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "clean.md")

	require.NoError(t, writeFileAtomic(path, []byte("content\n")))

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, entries, 1, "exactly the target file, no leftover temp file")
	require.Equal(t, "clean.md", entries[0].Name())
}
