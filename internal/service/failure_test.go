package service

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"copynote/internal/model"
	"copynote/internal/storage"
	"copynote/internal/testutil"
)

func TestMutationsLeaveMemoryAndDiskUnchangedOnSaveFailure(t *testing.T) {
	for _, operation := range []string{"create", "update", "delete", "reorder"} {
		t.Run(operation, func(t *testing.T) {
			s, path, _ := newTestService(t)
			a, err := s.Create("A", "a")
			if err != nil {
				t.Fatal(err)
			}
			b, err := s.Create("B", "b")
			if err != nil {
				t.Fatal(err)
			}
			before := s.List()
			disk, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			// Exercise a real failure opening the target's temporary file.
			if err := os.Mkdir(path+".tmp", 0o700); err != nil {
				t.Fatal(err)
			}
			switch operation {
			case "create":
				_, err = s.Create("ghost", "")
			case "update":
				_, err = s.Update(a.ID, "ghost", "")
			case "delete":
				err = s.Delete(a.ID)
			case "reorder":
				err = s.Reorder([]string{a.ID, b.ID})
			}
			if err == nil {
				t.Fatal("expected save failure")
			}
			if !reflect.DeepEqual(before, s.List()) {
				t.Fatal("memory changed after failed save")
			}
			after, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if string(after) != string(disk) {
				t.Fatal("disk changed after failed save")
			}
			if err := os.Remove(path + ".tmp"); err != nil {
				t.Fatal(err)
			}
			if _, err := s.Create("success", ""); err != nil {
				t.Fatal(err)
			}
			reloaded, err := New(path)
			if err != nil {
				t.Fatal(err)
			}
			if got := reloaded.List(); len(got) != 3 || !reflect.DeepEqual(got[1].Label, before[0].Label) || got[2].Label != before[1].Label {
				t.Fatalf("a later save persisted a failed operation: %#v", got)
			}
		})
	}
}

func backup(t *testing.T, entries []model.Entry, settings model.Settings) []byte {
	t.Helper()
	raw, err := json.Marshal(backupFile{AppVersion: "1.2.0", Entries: entries, Settings: settings})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestImportValidatesBeforeSideEffects(t *testing.T) {
	for _, raw := range []string{
		`{}`, `null`, `[]`, `{"entries":[]}`, `{"appVersion":"1","entries":null,"settings":{}}`,
		`{"appVersion":"1","entries":[],"settings":null}`,
		`{"appVersion":"1","entries":[{"label":"  "}],"settings":{"theme":"system","locale":"en"}}`,
		`{"appVersion":"1","entries":[{"label":"A","value":"\u0000"}],"settings":{"theme":"system","locale":"en"}}`,
		`{"appVersion":"1","entries":[],"settings":{"theme":"unknown","locale":"en"}}`,
		`{"formatVersion":2,"appVersion":"1","entries":[],"settings":{"theme":"system","locale":"en"}}`,
	} {
		t.Run(raw, func(t *testing.T) {
			s, path, _ := newTestService(t)
			called := false
			s.setAutorun = func(bool) error { called = true; return nil }
			if err := s.ImportData([]byte(raw)); err == nil {
				t.Fatal("accepted invalid backup")
			}
			if called || len(s.List()) != 0 {
				t.Fatal("invalid import had side effects")
			}
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Fatal("invalid import wrote data")
			}
		})
	}
}

func TestImportFailureIsAtomicAndRestoresAutorun(t *testing.T) {
	s, path, _ := newTestService(t)
	if _, err := s.Create("existing", "value"); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	settings := model.DefaultSettings()
	settings.Autorun = true
	settings.Theme = "dark"
	var calls []bool
	s.setAutorun = func(v bool) error { calls = append(calls, v); return nil }
	s.saveStore = func(string, model.Store) error { return errors.New("disk full") }
	if err := s.ImportData(backup(t, []model.Entry{{Label: "new"}}, settings)); err == nil {
		t.Fatal("expected failure")
	}
	if !reflect.DeepEqual(calls, []bool{true, false}) {
		t.Fatalf("autorun changes: %v", calls)
	}
	got, err := s.GetSettings()
	if err != nil || got != model.DefaultSettings() {
		t.Fatalf("settings changed: %#v, %v", got, err)
	}
	if len(s.List()) != 1 {
		t.Fatal("entries changed")
	}
	after, err := os.ReadFile(path)
	if err != nil || string(before) != string(after) {
		t.Fatal("disk changed")
	}
}

func TestImportDeduplicatesAndPersistsSettingsTogether(t *testing.T) {
	s, path, _ := newTestService(t)
	if _, err := s.Create("A", "a"); err != nil {
		t.Fatal(err)
	}
	settings := model.DefaultSettings()
	settings.Theme = "dark"
	raw := backup(t, []model.Entry{{Label: " A ", Value: "a"}, {ID: "old", Label: "B", Value: "b"}, {Label: "B", Value: "b"}}, settings)
	if err := s.ImportData(raw); err != nil {
		t.Fatal(err)
	}
	if err := s.ImportData(raw); err != nil {
		t.Fatal(err)
	}
	reloaded, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	entries := reloaded.List()
	if len(entries) != 2 || entries[1].ID == "old" || entries[1].Order != 1 {
		t.Fatalf("bad merge: %#v", entries)
	}
	got, err := reloaded.GetSettings()
	if err != nil || got != settings {
		t.Fatalf("settings: %#v, %v", got, err)
	}
	exported, err := reloaded.ExportData()
	if err != nil {
		t.Fatal(err)
	}
	other, _, _ := newTestService(t)
	if err := other.ImportData(exported); err != nil || len(other.List()) != 2 {
		t.Fatalf("round trip: %v", err)
	}
}

func TestLegacySettingsMigrateWithDefaults(t *testing.T) {
	dir := testutil.TempDir(t)
	path := filepath.Join(dir, "data.json")
	if err := os.WriteFile(path, []byte(`{"version":1,"entries":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	legacy := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(legacy, []byte(`{"theme":"dark","locale":"ru"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Create("new", "value"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacy, []byte(`corrupt legacy file`), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err = New(path)
	if err != nil {
		t.Fatal(err)
	}
	settings, err := s.GetSettings()
	if err != nil || settings.Theme != "dark" || settings.Locale != "ru" || !settings.Topmost {
		t.Fatalf("migration: %#v, %v", settings, err)
	}
	stored, err := storage.Load(path)
	if err != nil || stored.Version != model.SchemaVersion || stored.Settings == nil {
		t.Fatalf("stored snapshot: %#v, %v", stored, err)
	}
}

func TestSettingsRegistryFailureDoesNotPersistPreference(t *testing.T) {
	s, path, _ := newTestService(t)
	s.setAutorun = func(bool) error { return errors.New("access denied") }
	settings := model.DefaultSettings()
	settings.Autorun = true
	if err := s.SaveSettings(settings); err == nil {
		t.Fatal("expected registry error")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("preference saved despite registry failure")
	}
}
