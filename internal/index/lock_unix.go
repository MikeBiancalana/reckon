//go:build unix

package index

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

// lock acquires the single reconcile-writer lock for this index via an advisory
// flock on index.lock, serialising Rebuild/Reconcile across processes (WAL
// already serialises the underlying SQLite writes). The returned func releases it.
func (ix *Index) lock() (func(), error) {
	f, err := os.OpenFile(ix.lockPath, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, fmt.Errorf("index: open lock file: %w", err)
	}
	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX); err != nil {
		f.Close()
		return nil, fmt.Errorf("index: acquire reconcile lock: %w", err)
	}
	return func() {
		// Best-effort release; closing the fd drops the flock regardless.
		_ = unix.Flock(int(f.Fd()), unix.LOCK_UN)
		_ = f.Close()
	}, nil
}
