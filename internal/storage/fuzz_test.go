package storage

import (
	"bytes"
	"encoding/json"
	"testing"
)

func FuzzDecode(f *testing.F) {
	for _, seed := range []string{
		`{"version":2,"entries":[]}`,
		`{"version":1,"entries":null}`,
		`{"version":999,"entries":[]}`,
		// Settings need their two core fields to get past the decoder at all.
		`{"version":2,"entries":[],"settings":{"theme":"dark","locale":"ru","hotkey":"Ctrl+Alt+N"}}`,
		`{"version":2,"entries":[],"settings":{"hotkey":"Ctrl+Alt+N"}}`,
		// An offset written as +00:00 and one that is not UTC: the same
		// instants come back from a round trip spelled differently.
		`{"version":2,"entries":[{"id":"a","label":"mail","value":"me@example.com","order":0,` +
			`"createdAt":"2026-01-01T00:00:00+00:00","updatedAt":"2026-01-02T03:04:05+03:00"}]}`,
		`{"version":2,"entries":[{"id":"a","label":" ","value":""}]}`,
		`null`, `[]`, `{`, "\x00",
	} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, raw []byte) {
		store, err := decode(raw)
		if err != nil {
			return
		}
		if store.Entries == nil {
			t.Fatal("successful decode must normalize missing entries")
		}
		// What decode accepts, it must accept again once written back, and
		// write back unchanged. Compared as JSON rather than as structs: a
		// time.Time remembers how its zone was spelled, so the same instant
		// read from "+00:00" and from "Z" is not DeepEqual.
		encoded, err := json.Marshal(store)
		if err != nil {
			t.Fatal(err)
		}
		restored, err := decode(encoded)
		if err != nil {
			t.Fatalf("a snapshot decode accepted is rejected after a round trip: %v\n%s", err, encoded)
		}
		again, err := json.Marshal(restored)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(encoded, again) {
			t.Fatalf("snapshot changed after a JSON round trip:\n first %s\nsecond %s", encoded, again)
		}
	})
}
