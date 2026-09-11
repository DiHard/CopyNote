// Package storage persists a complete data snapshot with a previous-version backup.
package storage

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"copynote/internal/model"
	"golang.org/x/sys/windows"
)

func decode(raw []byte) (model.Store, error) {
	var s model.Store
	if err := json.Unmarshal(raw, &s); err != nil {
		return model.Store{}, err
	}
	if err := model.ValidateStore(s); err != nil {
		return model.Store{}, err
	}
	if s.Entries == nil {
		s.Entries = []model.Entry{}
	}
	return s, nil
}

func Load(path string) (model.Store, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return model.NewStore(), nil
	}
	if err != nil {
		return model.Store{}, fmt.Errorf("read %s: %w", path, err)
	}
	s, err := decode(raw)
	if err != nil {
		return model.Store{}, fmt.Errorf("parse %s: %w", path, err)
	}
	return s, nil
}

// WriteAtomic flushes a temporary file before replacing the destination on Windows.
// Callers serialize writes to the same path.
func WriteAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("create temporary file: %w", err)
	}
	defer os.Remove(tmp)
	_, writeErr := f.Write(data)
	syncErr := f.Sync()
	closeErr := f.Close()
	if err := errors.Join(writeErr, syncErr, closeErr); err != nil {
		return err
	}
	from, err := windows.UTF16PtrFromString(tmp)
	if err != nil {
		return err
	}
	to, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	return windows.MoveFileEx(from, to, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH)
}

func Save(path string, store model.Store) error {
	if err := model.ValidateStore(store); err != nil {
		return err
	}
	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return err
	}
	previous, err := os.ReadFile(path)
	if err == nil {
		// Never replace an externally corrupted or newer-format file.
		if _, err := decode(previous); err != nil {
			return fmt.Errorf("existing data is invalid: %w", err)
		}
		if err := WriteAtomic(path+".bak", previous); err != nil {
			return fmt.Errorf("save backup: %w", err)
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return WriteAtomic(path, data)
}

// RestoreBackup preserves the unreadable file before restoring a validated backup.
// It is only called after the user chooses recovery in the startup dialog.
func RestoreBackup(path string) error {
	raw, err := os.ReadFile(path + ".bak")
	if err != nil {
		return err
	}
	if _, err := decode(raw); err != nil {
		return fmt.Errorf("invalid backup: %w", err)
	}
	previous, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	preserved := path + ".corrupt-" + time.Now().UTC().Format("20060102T150405.000000000")
	if err := WriteAtomic(preserved, previous); err != nil {
		return fmt.Errorf("preserve original: %w", err)
	}
	return WriteAtomic(path, raw)
}
