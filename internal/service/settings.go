package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"copynote/internal/model"
)

// GetSettings returns the current user settings. If the settings
// file does not exist yet, returns DefaultSettings (no error).
func (s *Service) GetSettings() (model.Settings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadSettingsLocked()
}

// SaveSettings persists the given settings to settings.json next to
// the entries data file. The write is atomic (tmp + rename).
func (s *Service) SaveSettings(settings model.Settings) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveSettingsLocked(settings)
}

// settingsPath derives the settings file path from the entries file.
func (s *Service) settingsPath() string {
	return filepath.Join(filepath.Dir(s.path), "settings.json")
}

func (s *Service) loadSettingsLocked() (model.Settings, error) {
	p := s.settingsPath()
	raw, err := os.ReadFile(p)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return model.DefaultSettings(), nil
		}
		return model.Settings{}, fmt.Errorf("read %s: %w", p, err)
	}
	var settings model.Settings
	if err := json.Unmarshal(raw, &settings); err != nil {
		return model.Settings{}, fmt.Errorf("parse %s: %w", p, err)
	}
	return settings, nil
}

func (s *Service) saveSettingsLocked(settings model.Settings) error {
	p := s.settingsPath()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal settings: %w", err)
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, p); err != nil {
		return fmt.Errorf("rename %s → %s: %w", tmp, p, err)
	}
	return nil
}
