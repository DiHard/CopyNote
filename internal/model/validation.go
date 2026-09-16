package model

import (
	"fmt"
	"strings"
	"time"

	"copynote/internal/hotkey"
)

func ValidateEntry(label, value string) error {
	if strings.TrimSpace(label) == "" {
		return fmt.Errorf("label must not be empty")
	}
	if strings.ContainsRune(label, 0) || strings.ContainsRune(value, 0) {
		return fmt.Errorf("text must not contain NUL characters")
	}
	return nil
}

func ValidateSettings(s Settings) error {
	switch s.Theme {
	case "system", "light", "dark":
	default:
		return fmt.Errorf("invalid theme %q", s.Theme)
	}
	switch s.Locale {
	case "system", "en", "ru":
	default:
		return fmt.Errorf("invalid locale %q", s.Locale)
	}
	if err := hotkey.Valid(s.Hotkey); err != nil {
		return err
	}
	if s.RelocateRemindAfter != "" {
		if _, err := time.Parse(time.RFC3339, s.RelocateRemindAfter); err != nil {
			return fmt.Errorf("invalid relocateRemindAfter %q: %w", s.RelocateRemindAfter, err)
		}
	}
	return nil
}

func ValidateStore(s Store) error {
	if s.Version != 1 && s.Version != SchemaVersion {
		return fmt.Errorf("unsupported data schema version %d", s.Version)
	}
	ids := make(map[string]bool, len(s.Entries))
	for _, e := range s.Entries {
		if e.ID == "" || ids[e.ID] {
			return fmt.Errorf("missing or duplicate entry id %q", e.ID)
		}
		ids[e.ID] = true
		if err := ValidateEntry(e.Label, e.Value); err != nil {
			return fmt.Errorf("entry %s: %w", e.ID, err)
		}
	}
	if s.Settings != nil {
		return ValidateSettings(*s.Settings)
	}
	return nil
}
