package node

import (
	"crypto/rand"
	"sync"
	"time"

	"github.com/oklog/ulid/v2"
)

// A single monotonic entropy source guarded by a mutex: same-millisecond mints
// strictly increase (stable ordering, no collisions); a later millisecond
// reseeds. ULIDs are therefore time-sortable by their lexical string form.
var (
	mintMu      sync.Mutex
	mintEntropy = ulid.Monotonic(rand.Reader, 0)
)

// Mint returns a new ULID at the current time: 26-char Crockford base32,
// time-sortable, monotonic within a millisecond, decentralized-unique.
func Mint() string { return MintAt(time.Now()) }

// MintAt returns a new ULID whose timestamp component is t. Same-instant mints
// are strictly increasing (monotonic entropy) and unique.
func MintAt(t time.Time) string {
	mintMu.Lock()
	defer mintMu.Unlock()
	id, err := ulid.New(ulid.Timestamp(t), mintEntropy)
	if err != nil {
		// Monotonic overflow within a single millisecond (>2^80 mints) — not
		// reachable in practice; spill into the next millisecond rather than panic.
		id = ulid.MustNew(ulid.Timestamp(t.Add(time.Millisecond)), mintEntropy)
	}
	return id.String()
}
