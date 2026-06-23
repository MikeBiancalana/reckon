package node

import (
	"strings"
	"sync"
	"testing"
	"time"
)

const crockford = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

func isCrockford(s string) bool {
	if len(s) != 26 {
		return false
	}
	for _, r := range s {
		if !strings.ContainsRune(crockford, r) {
			return false
		}
	}
	return true
}

// AC5 — stable format: 26-char Crockford base32, unique.
func TestULIDFormatStable(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 1000; i++ {
		u := Mint()
		if !isCrockford(u) {
			t.Fatalf("ULID %q not 26-char Crockford base32", u)
		}
		if seen[u] {
			t.Fatalf("duplicate ULID minted: %q", u)
		}
		seen[u] = true
	}
}

// AC5 — time-sortable: earlier timestamp => lexically smaller ULID.
func TestULIDTimeSortable(t *testing.T) {
	base := time.Date(2026, 6, 19, 9, 0, 0, 0, time.UTC)
	var prev string
	for i := 0; i < 50; i++ {
		u := MintAt(base.Add(time.Duration(i) * time.Millisecond))
		if prev != "" && u <= prev {
			t.Fatalf("ULID not time-sortable at step %d: %q <= %q", i, u, prev)
		}
		prev = u
	}
}

// AC5 — concurrency-safe: minting from many goroutines yields no collisions and
// no data race (run with -race). The shared monotonic entropy is mutex-guarded.
func TestULIDConcurrentMintUnique(t *testing.T) {
	const goroutines, per = 16, 500
	var wg sync.WaitGroup
	var mu sync.Mutex
	seen := map[string]bool{}
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			local := make([]string, 0, per)
			for i := 0; i < per; i++ {
				local = append(local, Mint())
			}
			mu.Lock()
			defer mu.Unlock()
			for _, u := range local {
				if seen[u] {
					t.Errorf("concurrent duplicate ULID: %q", u)
				}
				seen[u] = true
			}
		}()
	}
	wg.Wait()
	if len(seen) != goroutines*per {
		t.Errorf("want %d unique ULIDs, got %d", goroutines*per, len(seen))
	}
}

// AC5 — monotonic within a single instant: same-ms mints strictly increasing
// and unique (no collisions, stable ordering).
func TestULIDMonotonicSameInstant(t *testing.T) {
	at := time.Date(2026, 6, 19, 9, 0, 0, 0, time.UTC)
	var prev string
	seen := map[string]bool{}
	for i := 0; i < 1000; i++ {
		u := MintAt(at)
		if prev != "" && u <= prev {
			t.Fatalf("same-instant ULID not monotonic at %d: %q <= %q", i, u, prev)
		}
		if seen[u] {
			t.Fatalf("same-instant duplicate: %q", u)
		}
		seen[u] = true
		prev = u
	}
}
