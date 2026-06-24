//go:build !unix

package index

// lock is a no-op on platforms without flock. WAL still serialises the underlying
// SQLite writes; cross-process reconcile coalescing is simply unavailable here.
func (ix *Index) lock() (func(), error) {
	return func() {}, nil
}
