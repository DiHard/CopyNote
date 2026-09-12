package service

import (
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"

	"copynote/internal/model"
	"golang.org/x/sys/windows/registry"
)

func (s *Service) GetSettings() (model.Settings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadSettingsLocked()
}

// Settings and entries share one snapshot, including during import.
func (s *Service) SaveSettings(settings model.Settings) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := model.ValidateSettings(settings); err != nil {
		return err
	}
	next := s.snapshotLocked()
	next.Settings = &settings
	return s.commitSettingsLocked(next)
}

// UpdateSettings applies mutate to the stored settings under the service
// lock. Preferences changed by Go rather than by the settings form go
// through here so a concurrent save cannot clobber them.
func (s *Service) UpdateSettings(mutate func(*model.Settings)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	settings, err := s.loadSettingsLocked()
	if err != nil {
		return err
	}
	mutate(&settings)
	if err := model.ValidateSettings(settings); err != nil {
		return err
	}
	next := s.snapshotLocked()
	next.Settings = &settings
	return s.commitSettingsLocked(next)
}

func (s *Service) commitSettingsLocked(next model.Store) error {
	previous, err := s.loadSettingsLocked()
	if err != nil {
		return err
	}
	changed := previous.Autorun != next.Settings.Autorun
	if changed {
		if err := s.setAutorun(next.Settings.Autorun); err != nil {
			return fmt.Errorf("update autorun: %w", err)
		}
	}
	if err := s.commitLocked(next); err != nil {
		if changed {
			if rollbackErr := s.setAutorun(previous.Autorun); rollbackErr != nil {
				return errors.Join(err, fmt.Errorf("restore autorun: %w", rollbackErr))
			}
		}
		return err
	}
	return nil
}

const autorunKeyPath = `Software\Microsoft\Windows\CurrentVersion\Run`
const autorunValueName = "CopyNote"

func applyAutorun(enabled bool) error {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, autorunKeyPath, registry.SET_VALUE|registry.QUERY_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	if enabled {
		exe, err := os.Executable()
		if err != nil {
			return err
		}
		return k.SetStringValue(autorunValueName, `"`+exe+`"`)
	}
	err = k.DeleteValue(autorunValueName)
	if errors.Is(err, registry.ErrNotExist) {
		return nil
	}
	return err
}

// Reconcile the external registry state after a move or interrupted settings save.
func (s *Service) EnsureAutorunPath() {
	settings, err := s.GetSettings()
	if err != nil {
		log.Printf("load autorun preference: %v", err)
		return
	}
	if err := s.setAutorun(settings.Autorun); err != nil {
		log.Printf("apply autorun: %v", err)
	}
}

func (s *Service) settingsPath() string {
	return filepath.Join(filepath.Dir(s.path), "settings.json")
}

func (s *Service) loadSettingsLocked() (model.Settings, error) {
	if s.store.Settings != nil {
		return *s.store.Settings, nil
	}
	// One-way migration: after the next successful commit data.json is authoritative.
	raw, err := os.ReadFile(s.settingsPath())
	if errors.Is(err, fs.ErrNotExist) {
		return model.DefaultSettings(), nil
	}
	if err != nil {
		return model.Settings{}, fmt.Errorf("read settings: %w", err)
	}
	settings, err := model.DecodeSettings(raw)
	if err != nil {
		return model.Settings{}, fmt.Errorf("parse settings: %w", err)
	}
	return settings, nil
}
