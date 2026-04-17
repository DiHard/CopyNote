package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"copynote/internal/model"
	"golang.org/x/sys/windows/registry"
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
// If the autorun flag changed, the Windows Registry Run key is
// updated so the app starts (or stops starting) at user login.
func (s *Service) SaveSettings(settings model.Settings) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.saveSettingsLocked(settings); err != nil {
		return err
	}
	applyAutorun(settings.Autorun)
	return nil
}

const autorunKeyPath = `Software\Microsoft\Windows\CurrentVersion\Run`
const autorunValueName = "CopyNote"

// applyAutorun creates or removes the CopyNote entry under
// HKCU\Software\Microsoft\Windows\CurrentVersion\Run.
// The value points to the currently running exe so the correct
// binary is launched even if the user moves the file.
func applyAutorun(enabled bool) {
	k, err := registry.OpenKey(
		registry.CURRENT_USER,
		autorunKeyPath,
		registry.SET_VALUE|registry.QUERY_VALUE,
	)
	if err != nil {
		return // silently ignore — can't write to registry
	}
	defer k.Close()

	if enabled {
		exe, err := os.Executable()
		if err != nil {
			return
		}
		_ = k.SetStringValue(autorunValueName, exe)
	} else {
		_ = k.DeleteValue(autorunValueName)
	}
}

// EnsureAutorunPath checks whether autorun is enabled and, if so,
// verifies that the registry points to the current exe path. If the
// exe has been moved (path differs), the registry value is silently
// updated. Called once at startup — self-healing, ~1 ms, no UI.
func (s *Service) EnsureAutorunPath() {
	s.mu.Lock()
	settings, err := s.loadSettingsLocked()
	s.mu.Unlock()
	if err != nil || !settings.Autorun {
		return
	}

	exe, err := os.Executable()
	if err != nil {
		return
	}

	k, err := registry.OpenKey(
		registry.CURRENT_USER,
		autorunKeyPath,
		registry.SET_VALUE|registry.QUERY_VALUE,
	)
	if err != nil {
		return
	}
	defer k.Close()

	current, _, err := k.GetStringValue(autorunValueName)
	if err != nil || current != exe {
		_ = k.SetStringValue(autorunValueName, exe)
	}
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
