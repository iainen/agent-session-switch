// Package session discovers and reads the local session files written by
// coding agents (currently Claude Code and Codex).
//
// It only reads agent data; moving sessions to a trash and favoriting them
// live in the trash and favorites packages.
package session

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Agent identifies which tool produced a session.
type Agent int

// Supported agents. NumAgents is the number of supported agents and is
// useful as an array length.
const (
	Claude Agent = iota
	Codex
	NumAgents
)

// String returns the display name of the agent.
func (a Agent) String() string {
	switch a {
	case Codex:
		return "Codex"
	default:
		return "Claude"
	}
}

// Bin returns the executable name used to resume a session.
func (a Agent) Bin() string {
	switch a {
	case Codex:
		return "codex"
	default:
		return "claude"
	}
}

// ResumeArgs returns the command-line arguments (without the program name)
// that resume the session with the given id.
func (a Agent) ResumeArgs(id string) []string {
	if a == Codex {
		return []string{"resume", id}
	}
	return []string{"--resume", id}
}

// TrashInfo describes where a trashed session came from. When it was
// deleted is the session's ModTime.
type TrashInfo struct {
	// OrigPath is the path the session file had before it was trashed.
	OrigPath string
}

// Session is a single conversation stored on disk.
type Session struct {
	Agent Agent
	// ID is the identifier accepted by the agent's resume command.
	ID string
	// Path is the location of the session file.
	Path string
	// Cwd is the directory the session was started in.
	Cwd   string
	Title string
	// ModTime is the file's modification time. For a trashed session it is
	// the time the session was deleted.
	ModTime time.Time
	// Missing reports that Cwd no longer exists.
	Missing bool
	// Trash is non-nil when the session lives in the trash.
	Trash *TrashInfo

	haystack string
}

// Key returns a string that is unique per session file. It is used as a
// cache key because the trash may hold several copies of the same ID.
func (s *Session) Key() string { return s.Path }

// Trashed reports whether the session is in the trash.
func (s *Session) Trashed() bool { return s.Trash != nil }

// Match reports whether every term (already lower-cased) occurs in the
// session's directory, title or ID.
func (s *Session) Match(terms []string) bool {
	if s.haystack == "" {
		s.haystack = strings.ToLower(s.Cwd + " " + s.Title + " " + s.ID)
	}
	for _, t := range terms {
		if !strings.Contains(s.haystack, t) {
			return false
		}
	}
	return true
}

// SortByRecent sorts sessions with the most recently modified first.
func SortByRecent(list []*Session) {
	sort.SliceStable(list, func(i, j int) bool { return list[i].ModTime.After(list[j].ModTime) })
}

// Load returns the most recent sessions of the given agent found under home.
func Load(home string, a Agent, limit int) []*Session {
	if a == Codex {
		return LoadCodex(home, limit)
	}
	return LoadClaude(home, limit)
}

// Rescan reads a single session file again, for example after it was
// restored from the trash. It returns nil if the file is not a usable session.
func Rescan(home string, a Agent, path string, modTime time.Time) *Session {
	if a == Codex {
		return scanCodex(path, modTime, loadCodexTitles(home))
	}
	return scanClaude(path, modTime)
}

// ---------- helpers shared by the agent implementations ----------

// maxLineBytes is the largest JSONL line we are willing to parse; longer
// lines (for example embedded images) are skipped. It is a variable so tests
// can lower it.
var maxLineBytes = 128 << 20

const (
	// scanLookahead is how many lines are inspected to find a title.
	scanLookahead = 400
	// scanWorkers bounds concurrent file reads while loading.
	scanWorkers = 16
)

type candidate struct {
	path    string
	modTime time.Time
}

// recentFiles lists non-empty files matching pattern, newest first.
func recentFiles(pattern string) []candidate {
	paths, _ := filepath.Glob(pattern)
	var out []candidate
	for _, p := range paths {
		if st, err := os.Stat(p); err == nil && st.Size() > 0 {
			out = append(out, candidate{p, st.ModTime()})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].modTime.After(out[j].modTime) })
	return out
}

// scanAll scans files, newest first, and returns up to limit usable sessions.
// Files are scanned concurrently in batches of limit, so unusable files
// (no working directory, for example) do not eat into the limit.
func scanAll(files []candidate, limit int, scan func(candidate) *Session) []*Session {
	if limit < 1 {
		return nil
	}
	var out []*Session
	for start := 0; start < len(files) && len(out) < limit; start += limit {
		batch := files[start:min(start+limit, len(files))]
		res := make([]*Session, len(batch))
		var wg sync.WaitGroup
		sem := make(chan struct{}, scanWorkers)
		for i, f := range batch {
			wg.Add(1)
			sem <- struct{}{}
			go func() {
				defer wg.Done()
				defer func() { <-sem }()
				res[i] = scan(f)
			}()
		}
		wg.Wait()
		for _, s := range res {
			if s != nil && len(out) < limit {
				out = append(out, s)
			}
		}
	}
	return out
}

// newSession builds a Session from what a scanner found. The title falls back
// to the first user message and then to noTitle.
func newSession(agent Agent, id, path, cwd, title, first string, modTime time.Time) *Session {
	if title == "" {
		title = first
	}
	if title == "" {
		title = noTitle
	}
	return &Session{
		Agent:   agent,
		ID:      id,
		Path:    path,
		Cwd:     cwd,
		Title:   title,
		ModTime: modTime,
		Missing: dirMissing(cwd),
	}
}

func dirMissing(dir string) bool {
	st, err := os.Stat(dir)
	return err != nil || !st.IsDir()
}

const noTitle = "(无内容)"
