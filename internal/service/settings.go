package service

import (
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"time"

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

// relocateSnoozeFor is how long "remind me later" keeps the move banner
// away: long enough not to nag, short enough that a download folder does
// not quietly remain the install location for months.
const relocateSnoozeFor = 7 * 24 * time.Hour

// SnoozeRelocatePrompt hides the move banner until the snooze expires and
// returns the instant it stored, so the UI never has to duplicate the
// duration. The permanent "don't offer again" is a separate flag.
func (s *Service) SnoozeRelocatePrompt() (string, error) {
	until := s.now().Add(relocateSnoozeFor).UTC().Format(time.RFC3339)
	if err := s.UpdateSettings(func(set *model.Settings) { set.RelocateRemindAfter = until }); err != nil {
		return "", err
	}
	return until, nil
}

const autorunKeyPath = `Software\Microsoft\Windows\CurrentVersion\Run`
const autorunValueName = "CopyNote"

// AutostartFlag is appended to the autorun registry command line so the
// process can tell a Windows sign-in from a launch the user performed
// themselves — only the latter should surface the window. EnsureAutorunPath
// rewrites the value on every start, so installations made before this
// flag existed pick it up on their next run.
const AutostartFlag = "--autostart"

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
		return k.SetStringValue(autorunValueName, `"`+exe+`" `+AutostartFlag)
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
