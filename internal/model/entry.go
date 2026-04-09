// Package model defines the core data types persisted to data.json
// and exchanged across the Go ↔ JS bridge. No I/O, no business logic.
package model

import (
	"crypto/rand"
	"fmt"
	"time"
)

// SchemaVersion is the current version of the on-disk JSON format.
// Bump this when introducing a breaking change to Store/Entry shape.
const SchemaVersion = 1

// Entry is a single CopyNote record.
//
// JSON field names are snake/camelCase per SPEC §7 (createdAt/updatedAt).
type Entry struct {
	ID        string    `json:"id"`
	Label     string    `json:"label"`
	Value     string    `json:"value"`
	Order     int       `json:"order"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// Store is the root object of data.json.
type Store struct {
	Version int     `json:"version"`
	Entries []Entry `json:"entries"`
}

// NewStore returns an empty store with the current schema version.
func NewStore() Store {
	return Store{Version: SchemaVersion, Entries: []Entry{}}
}

// NewUUID returns a random UUID v4 string (RFC 4122).
// Inline implementation to avoid pulling an external module.
func NewUUID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand.Read cannot fail on Windows; panic if the
		// platform invariant is violated.
		panic(fmt.Errorf("crypto/rand: %w", err))
	}
	// Set version (4) and variant (RFC 4122) bits.
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
