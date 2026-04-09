// Package storage handles reading and writing data.json atomically.
// No business logic — just bytes in, bytes out.
package storage

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"copynote/internal/model"
)

// Load reads data.json from path. Returns an empty store with no error
// if the file does not exist — CopyNote starts with a clean slate on
// first launch (SPEC §5.6).
func Load(path string) (model.Store, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return model.NewStore(), nil
		}
		return model.Store{}, fmt.Errorf("read %s: %w", path, err)
	}
	var s model.Store
	if err := json.Unmarshal(raw, &s); err != nil {
		return model.Store{}, fmt.Errorf("parse %s: %w", path, err)
	}
	if s.Entries == nil {
		s.Entries = []model.Entry{}
	}
	return s, nil
}

// Save writes the store to path atomically: encode → write to "<path>.tmp"
// → rename to path. Rename is atomic on Windows within the same volume,
// so a crash between write and rename cannot corrupt the real file.
//
// The parent directory is created if missing.
func Save(path string, store model.Store) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(path), err)
	}
	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal store: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		// Leave the tmp around for post-mortem; caller decides what to do.
		return fmt.Errorf("rename %s → %s: %w", tmp, path, err)
	}
	return nil
}
