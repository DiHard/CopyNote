package storage

import (
	"os"
	"path/filepath"
	"testing"

	"copynote/internal/model"
	"copynote/internal/testutil"
)

func TestBackupAndRecoveryPreserveOriginal(t *testing.T) {
	path := filepath.Join(testutil.TempDir(t), "data.json")
	first := model.NewStore()
	first.Entries = []model.Entry{{ID: "a", Label: "A"}}
	if err := Save(path, first); err != nil {
		t.Fatal(err)
	}
	second := model.NewStore()
	second.Entries = []model.Entry{{ID: "b", Label: "B"}}
	if err := Save(path, second); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Save(path, second); err == nil {
		t.Fatal("overwrote unreadable data")
	}
	if err := RestoreBackup(path); err != nil {
		t.Fatal(err)
	}
	restored, err := Load(path)
	if err != nil || len(restored.Entries) != 1 || restored.Entries[0].ID != "a" {
		t.Fatalf("restore: %#v, %v", restored, err)
	}
	files, err := filepath.Glob(path + ".corrupt-*")
	if err != nil || len(files) != 1 {
		t.Fatalf("preserved files: %v, %v", files, err)
	}
	raw, err := os.ReadFile(files[0])
	if err != nil || string(raw) != "broken" {
		t.Fatal("original not preserved")
	}
}

func TestRejectUnsupportedSchemaAndDuplicateIDs(t *testing.T) {
	for _, raw := range []string{`{}`, `null`, `{"version":99,"entries":[]}`, `{"version":2,"entries":[{"id":"x","label":"A"},{"id":"x","label":"B"}]}`} {
		path := filepath.Join(testutil.TempDir(t), "data.json")
		if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(path); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
}

func TestInvalidBackupCannotReplaceOriginal(t *testing.T) {
	path := filepath.Join(testutil.TempDir(t), "data.json")
	for _, file := range []string{path, path + ".bak"} {
		if err := os.WriteFile(file, []byte("broken"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := RestoreBackup(path); err == nil {
		t.Fatal("accepted invalid backup")
	}
	raw, err := os.ReadFile(path)
	if err != nil || string(raw) != "broken" {
		t.Fatal("original changed")
	}
}
