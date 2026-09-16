// Package service implements the CRUD business operations on CopyNote
// entries. All mutations are validated, re-persisted to disk, and
// serialized via a mutex.
package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"copynote/internal/clipboard"
	"copynote/internal/model"
	"copynote/internal/storage"
	"copynote/internal/version"
)

// Errors returned from Service operations. These are surfaced to the
// JS side as rejected promises and can be matched via error message.
var (
	ErrEmptyLabel    = errors.New("label must not be empty")
	ErrNotFound      = errors.New("entry not found")
	ErrReorderLength = errors.New("reorder list size mismatch")
	ErrReorderDup    = errors.New("reorder list has duplicate id")
)

// Service holds the in-memory store and persists mutations to disk.
// Safe for concurrent use.
type Service struct {
	mu    sync.Mutex
	path  string
	store model.Store
	// now is overridable for deterministic tests.
	now func() time.Time
	// writeText is overridable for tests; defaults to clipboard.WriteText.
	writeText  func(string) error
	setAutorun func(bool) error
	saveStore  func(string, model.Store) error
}

// New loads the store from path and returns a ready-to-use Service.
// If the file does not exist, the service starts with an empty store.
func New(path string) (*Service, error) {
	s, err := storage.Load(path)
	if err != nil {
		return nil, fmt.Errorf("load store: %w", err)
	}
	return &Service{
		path:       path,
		store:      s,
		now:        func() time.Time { return time.Now().UTC() },
		writeText:  clipboard.WriteText,
		setAutorun: applyAutorun,
		saveStore:  storage.Save,
	}, nil
}

// List returns a snapshot of all entries sorted by order ascending.
func (s *Service) List() []model.Entry {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]model.Entry, len(s.store.Entries))
	copy(out, s.store.Entries)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Order < out[j].Order })
	return out
}

// Create inserts a new entry at the top of the list (order=0), shifting
// all existing entries down by one. Returns the newly created entry.
func (s *Service) Create(label, value string) (model.Entry, error) {
	label = strings.TrimSpace(label)
	if label == "" {
		return model.Entry{}, ErrEmptyLabel
	}
	if err := model.ValidateEntry(label, value); err != nil {
		return model.Entry{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	now := s.now()
	next := s.snapshotLocked()
	for i := range next.Entries {
		next.Entries[i].Order++
	}
	entry := model.Entry{
		ID:        model.NewUUID(),
		Label:     label,
		Value:     value,
		Order:     0,
		CreatedAt: now,
		UpdatedAt: now,
	}
	next.Entries = append(next.Entries, entry)
	if err := s.commitLocked(next); err != nil {
		return model.Entry{}, err
	}
	return entry, nil
}

// Update mutates label and value of an existing entry. Order and
// createdAt are preserved; updatedAt is refreshed.
func (s *Service) Update(id, label, value string) (model.Entry, error) {
	label = strings.TrimSpace(label)
	if label == "" {
		return model.Entry{}, ErrEmptyLabel
	}
	if err := model.ValidateEntry(label, value); err != nil {
		return model.Entry{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	idx := s.findLocked(id)
	if idx < 0 {
		return model.Entry{}, ErrNotFound
	}
	next := s.snapshotLocked()
	next.Entries[idx].Label = label
	next.Entries[idx].Value = value
	next.Entries[idx].UpdatedAt = s.now()
	if err := s.commitLocked(next); err != nil {
		return model.Entry{}, err
	}
	return s.store.Entries[idx], nil
}

// Copy writes the entry's value to the system clipboard. The store
// is not modified — copying is a read-only operation.
func (s *Service) Copy(id string) (model.Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	idx := s.findLocked(id)
	if idx < 0 {
		return model.Entry{}, ErrNotFound
	}
	entry := s.store.Entries[idx]
	text := entry.Value
	if text == "" {
		text = entry.Label
	}
	if err := s.writeText(text); err != nil {
		return model.Entry{}, fmt.Errorf("clipboard: %w", err)
	}
	return entry, nil
}

// Reorder applies a new ordering given as a slice of ids in the desired
// final order. The slice must contain every existing entry's id exactly
// once; partial reorderings are rejected to keep the operation atomic.
func (s *Service) Reorder(orderedIDs []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(orderedIDs) != len(s.store.Entries) {
		return ErrReorderLength
	}
	idx := make(map[string]int, len(orderedIDs))
	for i, id := range orderedIDs {
		if _, dup := idx[id]; dup {
			return ErrReorderDup
		}
		idx[id] = i
	}
	// Verify every existing entry appears in the new order.
	for i := range s.store.Entries {
		if _, ok := idx[s.store.Entries[i].ID]; !ok {
			return ErrNotFound
		}
	}
	next := s.snapshotLocked()
	for i := range next.Entries {
		next.Entries[i].Order = idx[next.Entries[i].ID]
	}
	return s.commitLocked(next)
}

// Delete removes an entry and re-packs the order values of the
// remaining ones so they stay contiguous from 0.
func (s *Service) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	idx := s.findLocked(id)
	if idx < 0 {
		return ErrNotFound
	}
	next := s.snapshotLocked()
	next.Entries = append(next.Entries[:idx], next.Entries[idx+1:]...)
	sort.SliceStable(next.Entries, func(i, j int) bool { return next.Entries[i].Order < next.Entries[j].Order })
	for i := range next.Entries {
		next.Entries[i].Order = i
	}
	return s.commitLocked(next)
}

// findLocked returns the index of an entry by id, or -1 if absent.
// Must be called with s.mu held.
func (s *Service) findLocked(id string) int {
	for i := range s.store.Entries {
		if s.store.Entries[i].ID == id {
			return i
		}
	}
	return -1
}

func (s *Service) snapshotLocked() model.Store {
	next := s.store
	next.Entries = append([]model.Entry{}, s.store.Entries...)
	return next
}

// Publish only after the complete snapshot has reached disk.
func (s *Service) commitLocked(next model.Store) error {
	if next.Settings == nil {
		settings, err := s.loadSettingsLocked()
		if err != nil {
			return err
		}
		next.Settings = &settings
	}
	next.Version = model.SchemaVersion
	if err := s.saveStore(s.path, next); err != nil {
		return err
	}
	s.store = next
	return nil
}

// ── Import / Export ─────────────────────────────────────────────

// backupFile is the combined structure written/read during export/import.
// AppVersion is sourced from the single-source-of-truth version package.
type backupFile struct {
	FormatVersion int            `json:"formatVersion,omitempty"`
	AppVersion    string         `json:"appVersion"`
	Entries       []model.Entry  `json:"entries"`
	Settings      model.Settings `json:"settings"`
}

// ExportData returns a JSON blob containing all entries and settings.
func (s *Service) ExportData() ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	settings, err := s.loadSettingsLocked()
	if err != nil {
		return nil, fmt.Errorf("load settings: %w", err)
	}

	bf := backupFile{
		FormatVersion: 1,
		AppVersion:    version.Version,
		Entries:       s.store.Entries,
		Settings:      settings,
	}
	data, err := json.MarshalIndent(bf, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}
	return data, nil
}

// ImportResult reports what ImportData did with a file's entries.
type ImportResult struct {
	Added   int `json:"added"`
	Skipped int `json:"skipped"`
}

// ImportData merges entries from a backup JSON blob and overwrites
// settings. Duplicate entries (same label + value, whether already in the
// list or earlier in the file) are skipped and counted, so the UI can say
// what the import actually did.
func (s *Service) ImportData(raw []byte) (ImportResult, error) {
	// Pointer fields distinguish missing/null values from a valid empty backup.
	var input struct {
		FormatVersion int             `json:"formatVersion"`
		AppVersion    string          `json:"appVersion"`
		Entries       *[]model.Entry  `json:"entries"`
		Settings      json.RawMessage `json:"settings"`
	}
	if err := json.Unmarshal(raw, &input); err != nil {
		return ImportResult{}, fmt.Errorf("invalid backup file: %w", err)
	}
	if input.AppVersion == "" || input.Entries == nil || len(input.Settings) == 0 || string(input.Settings) == "null" {
		return ImportResult{}, errors.New("invalid backup: appVersion, entries and settings are required")
	}
	if input.FormatVersion < 0 || input.FormatVersion > 1 {
		return ImportResult{}, errors.New("unsupported backup format version")
	}
	settings, err := model.DecodeSettings(input.Settings)
	if err != nil {
		return ImportResult{}, fmt.Errorf("invalid settings: %w", err)
	}
	entries := *input.Entries
	for i := range entries {
		entries[i].Label = strings.TrimSpace(entries[i].Label)
		if err := model.ValidateEntry(entries[i].Label, entries[i].Value); err != nil {
			return ImportResult{}, fmt.Errorf("entry %d: %w", i+1, err)
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	next := s.snapshotLocked()
	next.Settings = &settings

	// Build a set of existing label+value pairs for dedup.
	type key struct{ label, value string }
	existing := make(map[key]bool, len(s.store.Entries))
	for _, e := range s.store.Entries {
		existing[key{e.Label, e.Value}] = true
	}

	// Find max order in current entries.
	maxOrder := -1
	for _, e := range s.store.Entries {
		if e.Order > maxOrder {
			maxOrder = e.Order
		}
	}

	// Append non-duplicate entries with fresh IDs and sequential order.
	var result ImportResult
	now := s.now()
	for _, e := range entries {
		if existing[key{e.Label, e.Value}] {
			result.Skipped++
			continue
		}
		maxOrder++
		next.Entries = append(next.Entries, model.Entry{
			ID:        model.NewUUID(),
			Label:     e.Label,
			Value:     e.Value,
			Order:     maxOrder,
			CreatedAt: now,
			UpdatedAt: now,
		})
		existing[key{e.Label, e.Value}] = true
		result.Added++
	}

	if err := s.commitSettingsLocked(next); err != nil {
		return ImportResult{}, err
	}
	return result, nil
}
