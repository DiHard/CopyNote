package storage

import (
	"errors"
	"fmt"
	"io/fs"
	"os"

	"copynote/internal/model"
)

// LoadLegacySettings supports the one-way migration from settings.json.
func LoadLegacySettings(path string) (model.Settings, error) {
	raw, err := os.ReadFile(path)
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
