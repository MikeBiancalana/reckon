package textmigrate

import "fmt"

// writeFileAtomic writes data to path via a temp file in the same directory
// followed by an atomic rename, so a half-written file can never corrupt a
// truth file at path. On any error the temp file is removed and path is
// left untouched.
func writeFileAtomic(path string, data []byte) error {
	return fmt.Errorf("textmigrate: writeFileAtomic not implemented")
}
