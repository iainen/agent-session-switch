package session

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"time"
)

// codexRecord is the subset of a Codex rollout JSONL line that we read.
type codexRecord struct {
	Type    string `json:"type"`
	Payload struct {
		ID      string          `json:"id"`
		Cwd     string          `json:"cwd"`
		Type    string          `json:"type"`
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
	} `json:"payload"`
}

// codexInjectedPrefixes marks "user" messages that Codex inserts itself
// (instructions, environment context, ...) rather than the user typing them.
var codexInjectedPrefixes = []string{
	"# AGENTS.md", "<environment_context", "<user_instructions", "<INSTRUCTIONS",
	"<permissions", "<collaboration_mode", "<skills", "<turn_aborted",
}

func isCodexInjected(text string) bool {
	for _, p := range codexInjectedPrefixes {
		if strings.HasPrefix(text, p) {
			return true
		}
	}
	return false
}

// codexMessage returns the role and text of a genuine user or assistant
// message, or ok=false for every other kind of record.
func codexMessage(r *codexRecord) (role Role, text string, ok bool) {
	if r.Type != "response_item" || r.Payload.Type != "message" {
		return 0, "", false
	}
	switch r.Payload.Role {
	case "user":
		role = User
	case "assistant":
		role = Assistant
	default:
		return 0, "", false
	}
	text = strings.TrimSpace(contentText(r.Payload.Content, "\n"))
	if text == "" || (role == User && isCodexInjected(text)) {
		return 0, "", false
	}
	return role, text, true
}

// loadCodexTitles reads thread names from home/.codex/session_index.jsonl.
// Later entries override earlier ones.
func loadCodexTitles(home string) map[string]string {
	titles := map[string]string{}
	_ = readLines(filepath.Join(home, ".codex", "session_index.jsonl"), func(line []byte) bool {
		var e struct {
			ID   string `json:"id"`
			Name string `json:"thread_name"`
		}
		if json.Unmarshal(line, &e) == nil && e.ID != "" && e.Name != "" {
			titles[e.ID] = e.Name
		}
		return true
	})
	return titles
}

// LoadCodex returns the most recent Codex sessions stored under
// home/.codex/sessions, newest first.
func LoadCodex(home string, limit int) []*Session {
	files := recentFiles(filepath.Join(home, ".codex", "sessions", "*", "*", "*", "rollout-*.jsonl"))
	titles := loadCodexTitles(home)
	return scanAll(files, limit, func(c candidate) *Session { return scanCodex(c.path, c.modTime, titles) })
}

func scanCodex(path string, modTime time.Time, titles map[string]string) *Session {
	var id, cwd, first string
	i := 0
	if err := readLines(path, func(line []byte) bool {
		if i++; i > scanLookahead {
			return false
		}
		var r codexRecord
		if json.Unmarshal(line, &r) != nil {
			return true
		}
		if r.Type == "session_meta" {
			id, cwd = r.Payload.ID, r.Payload.Cwd
		}
		if first == "" {
			if role, text, ok := codexMessage(&r); ok && role == User {
				first = oneLine(text)
			}
		}
		return !(id != "" && cwd != "" && (titles[id] != "" || first != ""))
	}); err != nil {
		return nil
	}
	if id == "" { // fall back to the trailing UUID in the file name
		base := strings.TrimSuffix(filepath.Base(path), ".jsonl")
		if len(base) >= 36 {
			id = base[len(base)-36:]
		}
	}
	if id == "" || cwd == "" {
		return nil
	}
	return newSession(Codex, id, path, cwd, titles[id], first, modTime)
}

// codexPreviewMessage extracts a genuine user or assistant message from a line.
func codexPreviewMessage(line []byte) (Role, string, bool) {
	var r codexRecord
	if json.Unmarshal(line, &r) != nil {
		return 0, "", false
	}
	return codexMessage(&r)
}
