package favorites

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/iainen/agent-session-switch/internal/session"
)

func TestToggleAndPersist(t *testing.T) {
	home := t.TempDir()
	st := Open(home)
	if st.Has(session.Claude, "a") {
		t.Fatal("new store should be empty")
	}

	added, err := st.Toggle(session.Claude, "a")
	if err != nil || !added {
		t.Fatalf("Toggle = %v, %v; want added", added, err)
	}
	if !st.Has(session.Claude, "a") {
		t.Error("Has = false after adding")
	}

	if reopened := Open(home); !reopened.Has(session.Claude, "a") {
		t.Error("favorite was not persisted")
	}

	added, err = st.Toggle(session.Claude, "a")
	if err != nil || added {
		t.Fatalf("second Toggle = %v, %v; want removed", added, err)
	}
	if Open(home).Has(session.Claude, "a") {
		t.Error("removal was not persisted")
	}
}

func TestAgentsAreSeparate(t *testing.T) {
	st := Open(t.TempDir())
	if _, err := st.Toggle(session.Codex, "same-id"); err != nil {
		t.Fatal(err)
	}
	if st.Has(session.Claude, "same-id") {
		t.Error("a Codex favorite leaked into Claude")
	}
	if !st.Has(session.Codex, "same-id") {
		t.Error("Codex favorite missing")
	}
}

func TestRemove(t *testing.T) {
	home := t.TempDir()
	st := Open(home)
	if err := st.Remove(session.Claude, "never-added"); err != nil {
		t.Errorf("removing a non-favorite: %v", err)
	}
	if _, err := st.Toggle(session.Claude, "a"); err != nil {
		t.Fatal(err)
	}
	if err := st.Remove(session.Claude, "a"); err != nil {
		t.Fatal(err)
	}
	if Open(home).Has(session.Claude, "a") {
		t.Error("Remove was not persisted")
	}
}

func TestCorruptFileIsNeverOverwritten(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, ".claude", "session-favorites.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	const junk = "{ this is not json"
	if err := os.WriteFile(path, []byte(junk), 0o644); err != nil {
		t.Fatal(err)
	}

	st := Open(home)
	if st.Err() == nil {
		t.Fatal("Err() = nil for a corrupt file")
	}
	if _, err := st.Toggle(session.Claude, "a"); err != ErrUnreadable {
		t.Errorf("Toggle error = %v, want ErrUnreadable", err)
	}
	if err := st.Remove(session.Claude, "a"); err != ErrUnreadable {
		t.Errorf("Remove error = %v, want ErrUnreadable", err)
	}
	if b, _ := os.ReadFile(path); string(b) != junk {
		t.Errorf("corrupt file was overwritten: %q", b)
	}
}

func TestFailedWriteRollsBack(t *testing.T) {
	// A regular file where the ".claude" directory should be makes every write fail.
	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, ".claude"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	st := Open(home)
	if _, err := st.Toggle(session.Claude, "a"); err == nil {
		t.Fatal("expected a write error")
	}
	if st.Has(session.Claude, "a") {
		t.Error("in-memory state was not rolled back after a failed write")
	}
}

func TestSaveLeavesNoTempFile(t *testing.T) {
	home := t.TempDir()
	st := Open(home)
	if _, err := st.Toggle(session.Claude, "a"); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(filepath.Join(home, ".claude"))
	if len(entries) != 1 || entries[0].Name() != "session-favorites.json" {
		t.Errorf("unexpected files: %v", entries)
	}
}
