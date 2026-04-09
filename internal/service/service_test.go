package service

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"copynote/internal/storage"
)

// newTestService returns a service backed by a temp file with a
// deterministic clock and a no-op clipboard. Tests that need to assert
// on clipboard interaction replace s.writeText with a closure of their own.
func newTestService(t *testing.T) (*Service, string, *time.Time) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "data.json")
	s, err := New(path)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	clock := time.Date(2026, 4, 9, 10, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return clock }
	s.writeText = func(string) error { return nil }
	return s, path, &clock
}

func TestCreate_RejectsEmptyLabel(t *testing.T) {
	s, _, _ := newTestService(t)

	cases := []string{"", "   ", "\t\n"}
	for _, in := range cases {
		if _, err := s.Create(in, "value"); !errors.Is(err, ErrEmptyLabel) {
			t.Errorf("Create(%q): want ErrEmptyLabel, got %v", in, err)
		}
	}
}

func TestCreate_TrimsLabel(t *testing.T) {
	s, _, _ := newTestService(t)

	e, err := s.Create("  hello  ", "v")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if e.Label != "hello" {
		t.Errorf("want label 'hello', got %q", e.Label)
	}
}

func TestCreate_InsertsAtTop_ShiftsOthers(t *testing.T) {
	s, _, _ := newTestService(t)

	b, _ := s.Create("B", "")
	a, _ := s.Create("A", "")

	list := s.List()
	if len(list) != 2 {
		t.Fatalf("want 2 entries, got %d", len(list))
	}
	if list[0].ID != a.ID || list[0].Order != 0 {
		t.Errorf("first entry should be A with order 0, got %#v", list[0])
	}
	if list[1].ID != b.ID || list[1].Order != 1 {
		t.Errorf("second entry should be B with order 1, got %#v", list[1])
	}
}

func TestCreate_SetsTimestampsAndID(t *testing.T) {
	s, _, clock := newTestService(t)
	e, err := s.Create("x", "y")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if e.ID == "" || len(e.ID) != 36 {
		t.Errorf("want UUID id of len 36, got %q", e.ID)
	}
	if !e.CreatedAt.Equal(*clock) || !e.UpdatedAt.Equal(*clock) {
		t.Errorf("timestamps mismatch: created=%v updated=%v clock=%v", e.CreatedAt, e.UpdatedAt, *clock)
	}
}

func TestUpdate_KeepsOrderAndCreatedAt_UpdatesTimestamp(t *testing.T) {
	s, _, clock := newTestService(t)

	orig, _ := s.Create("A", "a")
	*clock = clock.Add(1 * time.Hour)

	upd, err := s.Update(orig.ID, "A2", "a2")
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if upd.Order != orig.Order {
		t.Errorf("order changed: %d → %d", orig.Order, upd.Order)
	}
	if !upd.CreatedAt.Equal(orig.CreatedAt) {
		t.Errorf("createdAt changed: %v → %v", orig.CreatedAt, upd.CreatedAt)
	}
	if !upd.UpdatedAt.After(orig.UpdatedAt) {
		t.Errorf("updatedAt not advanced: %v → %v", orig.UpdatedAt, upd.UpdatedAt)
	}
	if upd.Label != "A2" || upd.Value != "a2" {
		t.Errorf("label/value not updated: %#v", upd)
	}
}

func TestUpdate_RejectsEmptyLabel(t *testing.T) {
	s, _, _ := newTestService(t)
	e, _ := s.Create("A", "")
	if _, err := s.Update(e.ID, "   ", "v"); !errors.Is(err, ErrEmptyLabel) {
		t.Errorf("want ErrEmptyLabel, got %v", err)
	}
}

func TestUpdate_NotFound(t *testing.T) {
	s, _, _ := newTestService(t)
	if _, err := s.Update("nope", "x", "y"); !errors.Is(err, ErrNotFound) {
		t.Errorf("want ErrNotFound, got %v", err)
	}
}

func TestDelete_RepacksOrder(t *testing.T) {
	s, _, _ := newTestService(t)

	c, _ := s.Create("C", "")
	b, _ := s.Create("B", "")
	a, _ := s.Create("A", "")

	// Initial order: A=0, B=1, C=2
	if err := s.Delete(b.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	list := s.List()
	if len(list) != 2 {
		t.Fatalf("want 2 remaining, got %d", len(list))
	}
	if list[0].ID != a.ID || list[0].Order != 0 {
		t.Errorf("first should be A/0, got %#v", list[0])
	}
	if list[1].ID != c.ID || list[1].Order != 1 {
		t.Errorf("second should be C/1, got %#v", list[1])
	}
}

func TestDelete_NotFound(t *testing.T) {
	s, _, _ := newTestService(t)
	if err := s.Delete("nope"); !errors.Is(err, ErrNotFound) {
		t.Errorf("want ErrNotFound, got %v", err)
	}
}

func TestList_SortedByOrder(t *testing.T) {
	s, _, _ := newTestService(t)

	// Create in reverse alphabetical order: each new entry bumps
	// existing ones down, so final order should be A, B, C.
	_, _ = s.Create("C", "")
	_, _ = s.Create("B", "")
	_, _ = s.Create("A", "")

	list := s.List()
	wantLabels := []string{"A", "B", "C"}
	for i, w := range wantLabels {
		if list[i].Label != w {
			t.Errorf("pos %d: want %s, got %s", i, w, list[i].Label)
		}
		if list[i].Order != i {
			t.Errorf("pos %d: want order %d, got %d", i, i, list[i].Order)
		}
	}
}

func TestService_PersistsEachMutation(t *testing.T) {
	s, path, _ := newTestService(t)

	_, _ = s.Create("A", "av")

	// Re-load from disk through a fresh store.
	reload, err := storage.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(reload.Entries) != 1 || reload.Entries[0].Label != "A" {
		t.Errorf("after Create, disk state wrong: %#v", reload.Entries)
	}

	// Update and re-check.
	_, _ = s.Update(reload.Entries[0].ID, "A2", "av2")
	reload, _ = storage.Load(path)
	if reload.Entries[0].Label != "A2" || reload.Entries[0].Value != "av2" {
		t.Errorf("after Update, disk state wrong: %#v", reload.Entries)
	}

	// Delete and re-check.
	_ = s.Delete(reload.Entries[0].ID)
	reload, _ = storage.Load(path)
	if len(reload.Entries) != 0 {
		t.Errorf("after Delete, disk state wrong: %#v", reload.Entries)
	}
}

func TestCopy_FetchesValueAndCallsWriteText(t *testing.T) {
	s, _, _ := newTestService(t)

	created, err := s.Create("Email", "me@example.com")
	if err != nil {
		t.Fatal(err)
	}
	var captured string
	calls := 0
	s.writeText = func(v string) error {
		calls++
		captured = v
		return nil
	}

	got, err := s.Copy(created.ID)
	if err != nil {
		t.Fatalf("Copy: %v", err)
	}
	if calls != 1 {
		t.Errorf("want 1 writeText call, got %d", calls)
	}
	if captured != "me@example.com" {
		t.Errorf("want value 'me@example.com', got %q", captured)
	}
	if got.ID != created.ID {
		t.Errorf("want returned entry id %q, got %q", created.ID, got.ID)
	}
}

func TestCopy_NotFound(t *testing.T) {
	s, _, _ := newTestService(t)
	if _, err := s.Copy("nope"); !errors.Is(err, ErrNotFound) {
		t.Errorf("want ErrNotFound, got %v", err)
	}
}

func TestCopy_PassesUpClipboardError(t *testing.T) {
	s, _, _ := newTestService(t)
	created, _ := s.Create("X", "v")
	boom := errors.New("clipboard busy")
	s.writeText = func(string) error { return boom }

	_, err := s.Copy(created.ID)
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if !errors.Is(err, boom) {
		t.Errorf("want wrapped boom, got %v", err)
	}
}

func TestCopy_DoesNotMutateStore(t *testing.T) {
	s, path, _ := newTestService(t)
	created, _ := s.Create("X", "v")

	statBefore, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	updatedBefore := s.store.Entries[0].UpdatedAt
	// Sleep just enough that any persist would change ModTime detectably.
	time.Sleep(20 * time.Millisecond)

	if _, err := s.Copy(created.ID); err != nil {
		t.Fatal(err)
	}
	statAfter, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !statAfter.ModTime().Equal(statBefore.ModTime()) {
		t.Errorf("Copy unexpectedly persisted to disk: before=%v after=%v", statBefore.ModTime(), statAfter.ModTime())
	}
	if !s.store.Entries[0].UpdatedAt.Equal(updatedBefore) {
		t.Errorf("Copy unexpectedly mutated UpdatedAt")
	}
}

func TestCopy_EmptyValueIsAllowed(t *testing.T) {
	s, _, _ := newTestService(t)
	created, _ := s.Create("Empty", "")
	got := ""
	called := false
	s.writeText = func(v string) error { called = true; got = v; return nil }

	if _, err := s.Copy(created.ID); err != nil {
		t.Fatalf("Copy: %v", err)
	}
	if !called || got != "" {
		t.Errorf("want writeText called with empty string, got called=%v val=%q", called, got)
	}
}

func TestService_ReloadsExistingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data.json")

	// Create a service, add an entry, throw it away.
	first, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := first.Create("Persisted", "value"); err != nil {
		t.Fatal(err)
	}

	// A fresh service should see the existing entry on load.
	second, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	list := second.List()
	if len(list) != 1 || list[0].Label != "Persisted" {
		t.Errorf("reload failed: %#v", list)
	}
}
