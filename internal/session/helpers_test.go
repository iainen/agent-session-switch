package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// writeJSONL writes each value as one JSON line and sets the file's mtime.
func writeJSONL(t *testing.T, path string, mtime time.Time, lines ...any) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	var sb strings.Builder
	for _, l := range lines {
		b, err := json.Marshal(l)
		if err != nil {
			t.Fatal(err)
		}
		sb.Write(b)
		sb.WriteByte('\n')
	}
	if err := os.WriteFile(path, []byte(sb.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, mtime, mtime); err != nil {
		t.Fatal(err)
	}
}

type m = map[string]any

func claudeUser(cwd, text string) m {
	return m{"type": "user", "cwd": cwd, "message": m{"role": "user", "content": text}}
}

func claudeAssistant(cwd, text string) m {
	return m{"type": "assistant", "cwd": cwd, "message": m{"role": "assistant", "content": []m{{"type": "text", "text": text}}}}
}

func claudePath(home, project, id string) string {
	return filepath.Join(home, ".claude", "projects", project, id+".jsonl")
}

func codexPath(home, id string) string {
	return filepath.Join(home, ".codex", "sessions", "2026", "09", "28", "rollout-2026-09-28T10-00-00-"+id+".jsonl")
}

func codexMeta(id, cwd string) m {
	return m{"type": "session_meta", "payload": m{"id": id, "cwd": cwd}}
}

func codexMsg(role, text string) m {
	kind := "input_text"
	if role == "assistant" {
		kind = "output_text"
	}
	return m{"type": "response_item", "payload": m{"type": "message", "role": role, "content": []m{{"type": kind, "text": text}}}}
}

var t0 = time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
