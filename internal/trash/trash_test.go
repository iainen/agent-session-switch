package trash

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/iainen/agent-session-switch/internal/session"
)

// newSession creates a real Claude session file under home and loads it.
func newSession(t *testing.T, home, id string, mtime time.Time) *session.Session {
	t.Helper()
	cwd := t.TempDir()
	path := filepath.Join(home, ".claude", "projects", "-p", id+".jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	line := `{"type":"user","cwd":` + quote(cwd) + `,"message":{"content":"title ` + id + `"}}` + "\n"
	if err := os.WriteFile(path, []byte(line), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, mtime, mtime); err != nil {
		t.Fatal(err)
	}
	s := session.Rescan(home, session.Claude, path, mtime)
	if s == nil {
		t.Fatal("fixture session not readable")
	}
	return s
}

func quote(s string) string { b, _ := json.Marshal(s); return string(b) }

func TestPutListRestore(t *testing.T) {
	home := t.TempDir()
	st := New(home)
	s := newSession(t, home, "s1", time.Now().Add(-time.Hour))
	orig := s.Path

	ts, err := st.Put(s)
	if err != nil {
		t.Fatal(err)
	}
	if exists(orig) {
		t.Error("original file still exists after Put")
	}
	if !ts.Trashed() || ts.Trash.OrigPath != orig || ts.ID != "s1" || ts.Title != "title s1" {
		t.Errorf("trashed session = %+v (trash %+v)", ts, ts.Trash)
	}
	if !strings.HasPrefix(ts.Path, st.Dir()) {
		t.Errorf("trashed file %q is outside %q", ts.Path, st.Dir())
	}

	list := st.List()
	if len(list) != 1 || list[0].ID != "s1" || !list[0].Trashed() {
		t.Fatalf("List() = %v", list)
	}

	restored, err := st.Restore(list[0])
	if err != nil {
		t.Fatal(err)
	}
	if restored.Path != orig || restored.Trashed() || restored.Title != "title s1" {
		t.Errorf("restored = %+v", restored)
	}
	if !exists(orig) {
		t.Error("original file missing after Restore")
	}
	if left := st.List(); len(left) != 0 {
		t.Errorf("trash not empty after restore: %v", left)
	}
	if entries, _ := os.ReadDir(st.Dir()); len(entries) != 0 {
		t.Errorf("trash directory not clean: %v", entries)
	}
}

func TestListNewestDeletionFirst(t *testing.T) {
	home := t.TempDir()
	st := New(home)
	for _, id := range []string{"a", "b", "c"} {
		if _, err := st.Put(newSession(t, home, id, time.Now())); err != nil {
			t.Fatal(err)
		}
		time.Sleep(5 * time.Millisecond)
	}
	var ids []string
	for _, s := range st.List() {
		ids = append(ids, s.ID)
	}
	if strings.Join(ids, ",") != "c,b,a" {
		t.Errorf("order = %v, want c,b,a", ids)
	}
}

func TestPutSameIDTwiceKeepsBoth(t *testing.T) {
	home := t.TempDir()
	st := New(home)
	first, err := st.Put(newSession(t, home, "dup", time.Now()))
	if err != nil {
		t.Fatal(err)
	}
	second, err := st.Put(newSession(t, home, "dup", time.Now()))
	if err != nil {
		t.Fatal(err)
	}
	if first.Path == second.Path {
		t.Fatal("second Put overwrote the first")
	}
	if n := len(st.List()); n != 2 {
		t.Errorf("List() has %d entries, want 2", n)
	}
}

func TestRestoreRefusesToOverwrite(t *testing.T) {
	home := t.TempDir()
	st := New(home)
	s := newSession(t, home, "s", time.Now())
	orig := s.Path
	ts, err := st.Put(s)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(orig, []byte("someone else's data"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Restore(ts); err == nil {
		t.Fatal("Restore overwrote an existing file")
	}
	if b, _ := os.ReadFile(orig); string(b) != "someone else's data" {
		t.Errorf("existing file was modified: %q", b)
	}
	if !exists(ts.Path) {
		t.Error("trashed copy was lost")
	}
}

func TestRestoreRecreatesMissingDirectory(t *testing.T) {
	home := t.TempDir()
	st := New(home)
	s := newSession(t, home, "s", time.Now())
	dir := filepath.Dir(s.Path)
	ts, err := st.Put(s)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Restore(ts); err != nil {
		t.Fatalf("Restore: %v", err)
	}
}

func TestPurge(t *testing.T) {
	home := t.TempDir()
	st := New(home)
	ts, err := st.Put(newSession(t, home, "s", time.Now()))
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Purge(ts); err != nil {
		t.Fatal(err)
	}
	if entries, _ := os.ReadDir(st.Dir()); len(entries) != 0 {
		t.Errorf("files left after Purge: %v", entries)
	}
}

func TestPurgeAndRestoreRejectLiveSessions(t *testing.T) {
	home := t.TempDir()
	st := New(home)
	s := newSession(t, home, "live", time.Now())
	if err := st.Purge(s); err == nil {
		t.Error("Purge accepted a session that is not in the trash")
	}
	if _, err := st.Restore(s); err == nil {
		t.Error("Restore accepted a session that is not in the trash")
	}
	if _, err := st.Put(&session.Session{Path: s.Path, Trash: &session.TrashInfo{}}); err == nil {
		t.Error("Put accepted a session that is already trashed")
	}
	if !exists(s.Path) {
		t.Error("live session file was touched")
	}
}

func TestAgentSurvivesRoundTrip(t *testing.T) {
	home := t.TempDir()
	st := New(home)
	cwd := t.TempDir()
	path := filepath.Join(home, ".codex", "sessions", "2026", "09", "28", "rollout-x-01a00001-aaaa-7000-8000-000000000001.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	body := `{"type":"session_meta","payload":{"id":"01a00001-aaaa-7000-8000-000000000001","cwd":` + quote(cwd) + `}}` + "\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	s := session.Rescan(home, session.Codex, path, time.Now())
	if s == nil {
		t.Fatal("codex fixture unreadable")
	}
	if _, err := st.Put(s); err != nil {
		t.Fatal(err)
	}
	list := st.List()
	if len(list) != 1 || list[0].Agent != session.Codex {
		t.Fatalf("List() = %+v, want one Codex session", list)
	}
	restored, err := st.Restore(list[0])
	if err != nil || restored.Agent != session.Codex || restored.Path != path {
		t.Errorf("Restore() = %+v, %v", restored, err)
	}
}

// Trash written before the agent field existed has no "agent" key and must
// load as a Claude session.
func TestLegacyMetadataDefaultsToClaude(t *testing.T) {
	home := t.TempDir()
	st := New(home)
	if err := os.MkdirAll(st.Dir(), 0o700); err != nil {
		t.Fatal(err)
	}
	meta := `{"id":"old","base":"old","origPath":"/x/old.jsonl","cwd":"/x","title":"legacy","deletedAt":"2026-01-01T00:00:00Z"}`
	must(t, os.WriteFile(filepath.Join(st.Dir(), "old.json"), []byte(meta), 0o600))
	must(t, os.WriteFile(filepath.Join(st.Dir(), "old.jsonl"), []byte("{}\n"), 0o600))

	list := st.List()
	if len(list) != 1 || list[0].Agent != session.Claude || list[0].Title != "legacy" {
		t.Fatalf("List() = %+v", list)
	}
}

func TestListSkipsBrokenEntries(t *testing.T) {
	st := New(t.TempDir())
	must(t, os.MkdirAll(st.Dir(), 0o700))
	write := func(name, body string) { must(t, os.WriteFile(filepath.Join(st.Dir(), name), []byte(body), 0o600)) }
	write("nodata.json", `{"id":"nodata","origPath":"/x"}`) // no .jsonl next to it
	write("bad.json", `not json`)
	write("bad.jsonl", `{}`)
	write("noid.json", `{"origPath":"/x"}`)
	write("noid.jsonl", `{}`)
	if list := st.List(); len(list) != 0 {
		t.Errorf("List() = %v, want none", list)
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
