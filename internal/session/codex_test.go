package session

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"
)

const (
	idA = "01a00001-aaaa-7000-8000-000000000001"
	idB = "01a00002-bbbb-7000-8000-000000000002"
)

func TestLoadCodex(t *testing.T) {
	home, cwd := t.TempDir(), t.TempDir()

	writeJSONL(t, codexPath(home, idA), t0,
		codexMeta(idA, cwd),
		codexMsg("developer", "system prompt"),
		codexMsg("user", "# AGENTS.md instructions\n<INSTRUCTIONS>x</INSTRUCTIONS>"),
		codexMsg("user", "<environment_context>cwd</environment_context>"),
		codexMsg("user", "real question"))
	writeJSONL(t, codexPath(home, idB), t0.Add(time.Hour),
		codexMeta(idB, cwd),
		codexMsg("user", "question that is not used as the title"))
	writeJSONL(t, filepath.Join(home, ".codex", "session_index.jsonl"), t0,
		m{"id": idB, "thread_name": "old name"},
		m{"id": idB, "thread_name": "index title"}) // the later entry wins

	got := LoadCodex(home, 10)
	if len(got) != 2 || got[0].ID != idB || got[1].ID != idA {
		t.Fatalf("sessions = %v", got)
	}
	if got[0].Title != "index title" {
		t.Errorf("title = %q, want the session index name", got[0].Title)
	}
	if got[1].Title != "real question" {
		t.Errorf("title = %q, want the first real user message (injected ones skipped)", got[1].Title)
	}
	if got[1].Agent != Codex || got[1].Cwd != cwd {
		t.Errorf("session = %+v", got[1])
	}
}

func TestLoadCodexIDFromFileName(t *testing.T) {
	home, cwd := t.TempDir(), t.TempDir()
	writeJSONL(t, codexPath(home, idA), t0, m{"type": "session_meta", "payload": m{"cwd": cwd}})
	got := LoadCodex(home, 10)
	if len(got) != 1 || got[0].ID != idA {
		t.Fatalf("got %v, want one session with ID %s taken from the file name", got, idA)
	}
}

func TestLoadCodexSkipsSessionsWithoutCwd(t *testing.T) {
	home := t.TempDir()
	writeJSONL(t, codexPath(home, idA), t0, m{"type": "event_msg"})
	if got := LoadCodex(home, 10); len(got) != 0 {
		t.Errorf("got %d sessions, want 0", len(got))
	}
}

func TestLoadCodexPreview(t *testing.T) {
	home, cwd := t.TempDir(), t.TempDir()
	path := codexPath(home, idA)
	writeJSONL(t, path, t0,
		codexMeta(idA, cwd),
		codexMsg("developer", "hidden"),
		codexMsg("user", "<permissions>hidden</permissions>"),
		codexMsg("user", "question"),
		m{"type": "response_item", "payload": m{"type": "reasoning", "summary": []m{}}},
		m{"type": "response_item", "payload": m{"type": "custom_tool_call", "name": "exec"}},
		codexMsg("assistant", "answer"))
	p := LoadPreview(&Session{Agent: Codex, Path: path})
	want := []Message{{User, "question"}, {Assistant, "answer"}}
	if fmt.Sprint(p.Messages) != fmt.Sprint(want) || p.Total != 2 {
		t.Errorf("preview = %+v, want %v", p.Messages, want)
	}
}

func TestRescan(t *testing.T) {
	home, cwd := t.TempDir(), t.TempDir()
	cpath := claudePath(home, "-p", "cid")
	writeJSONL(t, cpath, t0, claudeUser(cwd, "claude one"))
	if s := Rescan(home, Claude, cpath, t0); s == nil || s.Title != "claude one" || s.Agent != Claude {
		t.Errorf("Rescan(Claude) = %+v", s)
	}
	xpath := codexPath(home, idA)
	writeJSONL(t, xpath, t0, codexMeta(idA, cwd), codexMsg("user", "codex one"))
	if s := Rescan(home, Codex, xpath, t0); s == nil || s.Title != "codex one" || s.Agent != Codex {
		t.Errorf("Rescan(Codex) = %+v", s)
	}
	if s := Rescan(home, Claude, filepath.Join(home, "missing.jsonl"), t0); s != nil {
		t.Errorf("Rescan of a missing file = %+v, want nil", s)
	}
}

func TestLoadDispatch(t *testing.T) {
	home, cwd := t.TempDir(), t.TempDir()
	writeJSONL(t, claudePath(home, "-p", "c"), t0, claudeUser(cwd, "c"))
	writeJSONL(t, codexPath(home, idA), t0, codexMeta(idA, cwd), codexMsg("user", "x"))
	if got := Load(home, Claude, 10); len(got) != 1 || got[0].Agent != Claude {
		t.Errorf("Load(Claude) = %v", got)
	}
	if got := Load(home, Codex, 10); len(got) != 1 || got[0].Agent != Codex {
		t.Errorf("Load(Codex) = %v", got)
	}
}
