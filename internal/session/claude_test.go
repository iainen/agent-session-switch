package session

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadClaude(t *testing.T) {
	home, cwd := t.TempDir(), t.TempDir()

	writeJSONL(t, claudePath(home, "-p1", "old"), t0, claudeUser(cwd, "first question"))
	writeJSONL(t, claudePath(home, "-p1", "titled"), t0.Add(time.Hour),
		claudeUser(cwd, "ignored because a title exists"),
		m{"type": "custom-title", "customTitle": "My title"})
	writeJSONL(t, claudePath(home, "-p2", "injected"), t0.Add(2*time.Hour),
		claudeUser(cwd, "<system-reminder>noise</system-reminder>real prompt"))
	writeJSONL(t, claudePath(home, "-p2", "gone"), t0.Add(3*time.Hour),
		claudeUser(filepath.Join(cwd, "does-not-exist"), "dir was deleted"))
	writeJSONL(t, claudePath(home, "-p2", "nocwd"), t0.Add(4*time.Hour), m{"type": "mode"})
	if err := os.WriteFile(claudePath(home, "-p2", "empty"), nil, 0o644); err != nil {
		t.Fatal(err)
	}

	got := LoadClaude(home, 100)
	var ids []string
	for _, s := range got {
		ids = append(ids, s.ID)
	}
	want := []string{"gone", "injected", "titled", "old"} // newest first; nocwd and empty skipped
	if fmt.Sprint(ids) != fmt.Sprint(want) {
		t.Fatalf("ids = %v, want %v", ids, want)
	}

	byID := map[string]*Session{}
	for _, s := range got {
		byID[s.ID] = s
	}
	if s := byID["titled"]; s.Title != "My title" || s.Cwd != cwd || s.Agent != Claude || s.Missing {
		t.Errorf("titled = %+v", s)
	}
	if s := byID["injected"]; s.Title != "real prompt" {
		t.Errorf("injected block leaked into title: %q", s.Title)
	}
	if s := byID["old"]; s.Title != "first question" {
		t.Errorf("title = %q, want first user message", s.Title)
	}
	if !byID["gone"].Missing {
		t.Error("session whose directory is gone should be Missing")
	}

	if got := LoadClaude(home, 2); len(got) != 2 || got[0].ID != "gone" {
		t.Errorf("limit not applied: %d sessions", len(got))
	}
}

func TestLoadClaudeNoTitleFallback(t *testing.T) {
	home, cwd := t.TempDir(), t.TempDir()
	writeJSONL(t, claudePath(home, "-p", "s"), t0, m{"type": "system", "cwd": cwd})
	got := LoadClaude(home, 10)
	if len(got) != 1 || got[0].Title != noTitle {
		t.Fatalf("got %+v, want one session titled %q", got, noTitle)
	}
}

func TestLoadClaudePreview(t *testing.T) {
	home, cwd := t.TempDir(), t.TempDir()
	path := claudePath(home, "-p", "s")
	writeJSONL(t, path, t0,
		claudeUser(cwd, "<system-reminder>x</system-reminder>hello"),
		claudeAssistant(cwd, "hi there"),
		m{"type": "user", "cwd": cwd, "isSidechain": true, "message": m{"content": "subagent"}},
		m{"type": "user", "cwd": cwd, "isMeta": true, "message": m{"content": "meta"}},
		m{"type": "system", "cwd": cwd},
		claudeUser(cwd, "<system-reminder>only noise</system-reminder>"),
	)
	p := LoadPreview(&Session{Agent: Claude, Path: path})
	want := []Message{{User, "hello"}, {Assistant, "hi there"}}
	if fmt.Sprint(p.Messages) != fmt.Sprint(want) || p.Total != 2 {
		t.Errorf("preview = %+v (total %d), want %v", p.Messages, p.Total, want)
	}
}

func TestPreviewKeepsOnlyLastMessages(t *testing.T) {
	home, cwd := t.TempDir(), t.TempDir()
	path := claudePath(home, "-p", "s")
	var lines []any
	for i := 0; i < maxKeep*2+50; i++ {
		lines = append(lines, claudeUser(cwd, fmt.Sprintf("msg %d", i)))
	}
	writeJSONL(t, path, t0, lines...)

	p := LoadPreview(&Session{Agent: Claude, Path: path})
	if p.Total != maxKeep*2+50 {
		t.Errorf("Total = %d, want %d", p.Total, maxKeep*2+50)
	}
	if len(p.Messages) != maxKeep {
		t.Fatalf("kept %d messages, want %d", len(p.Messages), maxKeep)
	}
	if last := p.Messages[len(p.Messages)-1].Text; last != fmt.Sprintf("msg %d", maxKeep*2+49) {
		t.Errorf("last kept message = %q", last)
	}
}

func TestPreviewTruncatesLongMessages(t *testing.T) {
	home, cwd := t.TempDir(), t.TempDir()
	path := claudePath(home, "-p", "s")
	long := ""
	for i := 0; i < maxMessageRunes+100; i++ {
		long += "中"
	}
	writeJSONL(t, path, t0, claudeUser(cwd, long))
	p := LoadPreview(&Session{Agent: Claude, Path: path})
	if got := len([]rune(p.Messages[0].Text)); got != maxMessageRunes+2 { // +" …"
		t.Errorf("message has %d runes, want %d", got, maxMessageRunes+2)
	}
}

func TestLoadPreviewMissingFile(t *testing.T) {
	p := LoadPreview(&Session{Agent: Claude, Path: filepath.Join(t.TempDir(), "nope.jsonl")})
	if p == nil || len(p.Messages) != 0 || p.Total != 0 {
		t.Errorf("preview of missing file = %+v, want empty", p)
	}
}
