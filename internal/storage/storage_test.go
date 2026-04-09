package storage

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"copynote/internal/model"
)

func TestLoad_MissingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nope.json")

	s, err := Load(path)
	if err != nil {
		t.Fatalf("Load on missing file should succeed, got %v", err)
	}
	if s.Version != model.SchemaVersion {
		t.Errorf("want version %d, got %d", model.SchemaVersion, s.Version)
	}
	if len(s.Entries) != 0 {
		t.Errorf("want empty entries, got %d", len(s.Entries))
	}
}

func TestSaveLoad_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data.json")

	now := time.Date(2026, 4, 9, 10, 0, 0, 0, time.UTC)
	want := model.Store{
		Version: model.SchemaVersion,
		Entries: []model.Entry{
			{ID: "a", Label: "Email", Value: "me@x.com", Order: 0, CreatedAt: now, UpdatedAt: now},
			{ID: "b", Label: "Phone", Value: "+7 900", Order: 1, CreatedAt: now, UpdatedAt: now},
		},
	}
	if err := Save(path, want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !reflect.DeepEqual(want, got) {
		t.Errorf("round-trip mismatch\nwant=%#v\n got=%#v", want, got)
	}
}

func TestSave_Atomic_NoTempLeftover(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data.json")

	if err := Save(path, model.NewStore()); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Errorf("expected .tmp to be gone after Save, err=%v", err)
	}
}

func TestSave_CreatesMissingParentDir(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "nested", "data.json")

	if err := Save(path, model.NewStore()); err != nil {
		t.Fatalf("Save on missing parent: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("file not created: %v", err)
	}
}

func TestLoad_CorruptJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Errorf("expected error on corrupt JSON, got nil")
	}
}

func TestSave_OverwritesExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data.json")

	first := model.Store{Version: 1, Entries: []model.Entry{{ID: "1", Label: "A", Order: 0}}}
	if err := Save(path, first); err != nil {
		t.Fatal(err)
	}
	second := model.Store{Version: 1, Entries: []model.Entry{{ID: "2", Label: "B", Order: 0}}}
	if err := Save(path, second); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Entries) != 1 || got.Entries[0].ID != "2" {
		t.Errorf("expected second save to overwrite, got %#v", got.Entries)
	}
}
