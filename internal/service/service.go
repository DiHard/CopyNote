// Package service implements the CRUD business operations on CopyNote
// entries. All mutations are validated, re-persisted to disk, and
// serialized via a mutex.
package service

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"copynote/internal/model"
	"copynote/internal/storage"
)

// Errors returned from Service operations. These are surfaced to the
// JS side as rejected promises and can be matched via error message.
var (
	ErrEmptyLabel = errors.New("label must not be empty")
	ErrNotFound   = errors.New("entry not found")
)

// Service holds the in-memory store and persists mutations to disk.
// Safe for concurrent use.
type Service struct {
	mu    sync.Mutex
	path  string
	store model.Store
	// now is overridable for deterministic tests.
	now func() time.Time
}

// New loads the store from path and returns a ready-to-use Service.
// If the file does not exist, the service starts with an empty store.
func New(path string) (*Service, error) {
	s, err := storage.Load(path)
	if err != nil {
		return nil, fmt.Errorf("load store: %w", err)
	}
	return &Service{
		path:  path,
		store: s,
		now:   func() time.Time { return time.Now().UTC() },
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
	s.mu.Lock()
	defer s.mu.Unlock()

	now := s.now()
	for i := range s.store.Entries {
		s.store.Entries[i].Order++
	}
	entry := model.Entry{
		ID:        model.NewUUID(),
		Label:     label,
		Value:     value,
		Order:     0,
		CreatedAt: now,
		UpdatedAt: now,
	}
	s.store.Entries = append(s.store.Entries, entry)
	if err := s.persistLocked(); err != nil {
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
	s.mu.Lock()
	defer s.mu.Unlock()

	idx := s.findLocked(id)
	if idx < 0 {
		return model.Entry{}, ErrNotFound
	}
	s.store.Entries[idx].Label = label
	s.store.Entries[idx].Value = value
	s.store.Entries[idx].UpdatedAt = s.now()
	if err := s.persistLocked(); err != nil {
		return model.Entry{}, err
	}
	return s.store.Entries[idx], nil
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
	s.store.Entries = append(s.store.Entries[:idx], s.store.Entries[idx+1:]...)
	s.repackLocked()
	return s.persistLocked()
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

// repackLocked renumbers entries so that order is 0, 1, 2… with no
// gaps, based on the current order values. Must be called with s.mu held.
func (s *Service) repackLocked() {
	sort.SliceStable(s.store.Entries, func(i, j int) bool {
		return s.store.Entries[i].Order < s.store.Entries[j].Order
	})
	for i := range s.store.Entries {
		s.store.Entries[i].Order = i
	}
}

// persistLocked flushes the in-memory store to disk. Must be called
// with s.mu held.
func (s *Service) persistLocked() error {
	return storage.Save(s.path, s.store)
}
