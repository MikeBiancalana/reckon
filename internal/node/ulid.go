package node

import "time"

// STUB — implementation lands in Phase 4.

// Mint returns a new ULID minted at the current time: 26-char Crockford base32,
// time-sortable, monotonic within a millisecond, decentralized-unique.
func Mint() string { return "" }

// MintAt returns a new ULID whose timestamp component is t. Same-instant mints
// are strictly increasing (monotonic entropy) and unique.
func MintAt(t time.Time) string { return "" }
