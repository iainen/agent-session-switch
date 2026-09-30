package session

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"time"
)

// claudeRecord is the subset of a Claude Code JSONL line that we read.
type claudeRecord struct {
	Type        string `json:"type"`
	Cwd         string `json:"cwd"`
	IsSidechain bool   `json:"isSidechain"`
	IsMeta      bool   `json:"isMeta"`
	CustomTitle string `json:"customTitle"`
	AITitle     string `json:"aiTitle"`
	Title       string `json:"title"`
	Message     struct {
		Content json.RawMessage `json:"content"`
	} `json:"message"`
}

// LoadClaude returns the most recent Claude Code sessions stored under
// home/.claude/projects, newest first.
func LoadClaude(home string, limit int) []*Session {
	files := recentFiles(filepath.Join(home, ".claude", "projects", "*", "*.jsonl"))
	return scanAll(files, limit, func(c candidate) *Session { return scanClaude(c.path, c.modTime) })
}

func scanClaude(path string, modTime time.Time) *Session {
	var cwd, title, first string
	i := 0
	err := readLines(path, func(line []byte) bool {
		i++
		if i > scanLookahead && cwd != "" && (title != "" || first != "") {
			return false
		}
		var r claudeRecord
		if json.Unmarshal(line, &r) != nil {
			return true
		}
		if cwd == "" {
			cwd = r.Cwd
		}
		for _, t := range []string{r.CustomTitle, r.AITitle, r.Title} {
			if t != "" {
				title = t
			}
		}
		if first == "" && r.Type == "user" && !r.IsSidechain {
			first = oneLine(contentText(r.Message.Content, " "))
		}
		return true
	})
	if err != nil || cwd == "" {
		return nil
	}
	id := strings.TrimSuffix(filepath.Base(path), ".jsonl")
	return newSession(Claude, id, path, cwd, title, first, modTime)
}

// claudePreviewMessage extracts a user or assistant message from a line.
func claudePreviewMessage(line []byte) (Role, string, bool) {
	var r claudeRecord
	if json.Unmarshal(line, &r) != nil {
		return 0, "", false
	}
	if (r.Type != "user" && r.Type != "assistant") || r.IsSidechain || r.IsMeta {
		return 0, "", false
	}
	text := contentText(r.Message.Content, " ")
	role := Assistant
	if r.Type == "user" {
		role = User
		text = stripInjected(text)
	}
	text = strings.TrimSpace(text)
	return role, text, text != ""
}
