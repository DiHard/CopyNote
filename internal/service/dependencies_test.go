package service

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"copynote/internal/model"
	"copynote/internal/version"
)

func TestInjectedDependenciesAndSettingsNotifications(t *testing.T) {
	const path = "unused/data.json"
	now := time.Date(2026, 10, 3, 8, 0, 0, 0, time.UTC)
	var copied string
	var autorun []bool
	var notifications int
	failSave := false
	s, err := NewWithDependencies(path, Dependencies{
		LoadStore: func(got string) (model.Store, error) {
			if got != path {
				t.Fatalf("load path: %s", got)
			}
			return model.NewStore(), nil
		},
		LoadLegacySettings: func(string) (model.Settings, error) { return model.DefaultSettings(), nil },
		SaveStore: func(got string, _ model.Store) error {
			if got != path {
				t.Fatalf("save path: %s", got)
			}
			if failSave {
				return errors.New("disk full")
			}
			return nil
		},
		Now:           func() time.Time { return now },
		WriteText:     func(text string) error { copied = text; return nil },
		SetAutorun:    func(enabled bool) error { autorun = append(autorun, enabled); return nil },
		SettingsSaved: func() { notifications++ },
	})
	if err != nil {
		t.Fatal(err)
	}
	entry, err := s.Create("label", "value")
	if err != nil {
		t.Fatal(err)
	}
	if !entry.CreatedAt.Equal(now) {
		t.Fatalf("createdAt: %v", entry.CreatedAt)
	}
	if _, err := s.Copy(entry.ID); err != nil {
		t.Fatal(err)
	}
	if copied != "value" || notifications != 0 {
		t.Fatalf("copy=%q notifications=%d", copied, notifications)
	}
	settings := model.DefaultSettings()
	settings.Autorun = true
	if err := s.SaveSettings(settings); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateSettings(func(set *model.Settings) { set.Locale = "ru" }); err != nil {
		t.Fatal(err)
	}
	importedSettings := settings
	importedSettings.Theme = "light"
	importedSettings.Locale = "en"
	backup, err := json.Marshal(backupFile{AppVersion: version.Version, Entries: []model.Entry{}, Settings: importedSettings})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ImportData(backup); err != nil {
		t.Fatal(err)
	}
	if notifications != 3 {
		t.Fatalf("successful save, update and import notifications: %d", notifications)
	}
	failSave = true
	settings.Autorun = false
	if err := s.SaveSettings(settings); err == nil {
		t.Fatal("expected save failure")
	}
	if notifications != 3 {
		t.Fatal("failed save sent a settings notification")
	}
	if !reflect.DeepEqual(autorun, []bool{true, false, true}) {
		t.Fatalf("autorun changes and rollback: %v", autorun)
	}
	got, err := s.GetSettings()
	if err != nil || got.Locale != "en" || !got.Autorun {
		t.Fatalf("last successful snapshot: %+v, %v", got, err)
	}
}

func TestInjectedLoadFailureIsReturned(t *testing.T) {
	want := errors.New("cannot load")
	_, err := NewWithDependencies("unused", Dependencies{
		LoadStore: func(string) (model.Store, error) { return model.Store{}, want },
	})
	if !errors.Is(err, want) {
		t.Fatalf("load failure: %v", err)
	}
}
