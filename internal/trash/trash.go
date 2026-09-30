// Package trash implements a recoverable trash for agent sessions.
//
// Trashed sessions are moved to <home>/.claude/session-trash/ next to a small
// JSON file that records where they came from, so they can be restored.
// Nothing here ever deletes a session that is not already in the trash,
// except Purge, which removes trashed sessions for good.
package trash

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/iainen/agent-session-switch/internal/session"
)

// Store is a trash directory.
type Store struct {
	home string
	dir  string
}

// New returns the trash store located under home.
func New(home string) *Store {
	return &Store{home: home, dir: filepath.Join(home, ".claude", "session-trash")}
}

// Dir returns the directory that holds trashed sessions.
func (st *Store) Dir() string { return st.dir }

// meta is stored next to each trashed session file. The field names are part
// of the on-disk format; keep them stable.
type meta struct {
	ID        string    `json:"id"`
	OrigPath  string    `json:"origPath"`
	Cwd       string    `json:"cwd"`
	Title     string    `json:"title"`
	DeletedAt time.Time `json:"deletedAt"`
	Agent     int       `json:"agent,omitempty"` // 0 (default, also for old data) = Claude
}

func metaPath(sessionPath string) string {
	return strings.TrimSuffix(sessionPath, ".jsonl") + ".json"
}

func exists(p string) bool { _, err := os.Lstat(p); return err == nil }

// Put moves s into the trash and returns the trashed entry. If the metadata
// cannot be written the move is undone, so a session is never left in the
// trash without the information needed to restore it.
func (st *Store) Put(s *session.Session) (*session.Session, error) {
	if s.Trashed() {
		return nil, errors.New("session is already in the trash")
	}
	if err := os.MkdirAll(st.dir, 0o700); err != nil {
		return nil, err
	}
	base := s.ID
	if exists(filepath.Join(st.dir, base+".jsonl")) || exists(filepath.Join(st.dir, base+".json")) {
		base += time.Now().Format("-20060102-150405")
	}
	dst := filepath.Join(st.dir, base+".jsonl")
	m := &meta{
		ID: s.ID, OrigPath: s.Path, Cwd: s.Cwd, Title: s.Title,
		DeletedAt: time.Now(), Agent: int(s.Agent),
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := moveFile(s.Path, dst); err != nil {
		return nil, err
	}
	if err := os.WriteFile(metaPath(dst), data, 0o600); err != nil {
		_ = moveFile(dst, s.Path)
		return nil, err
	}
	return m.session(dst), nil
}

func (m *meta) session(path string) *session.Session {
	return &session.Session{
		Agent:   session.Agent(m.Agent),
		ID:      m.ID,
		Path:    path,
		Cwd:     m.Cwd,
		Title:   m.Title,
		ModTime: m.DeletedAt,
		Trash:   &session.TrashInfo{OrigPath: m.OrigPath},
	}
}

// List returns every restorable session in the trash, most recently deleted
// first. Entries with missing or unreadable metadata are skipped.
func (st *Store) List() []*session.Session {
	metas, _ := filepath.Glob(filepath.Join(st.dir, "*.json"))
	var out []*session.Session
	for _, mp := range metas {
		data, err := os.ReadFile(mp)
		if err != nil {
			continue
		}
		var m meta
		if json.Unmarshal(data, &m) != nil || m.ID == "" {
			continue
		}
		jp := strings.TrimSuffix(mp, ".json") + ".jsonl"
		if !exists(jp) {
			continue
		}
		out = append(out, m.session(jp))
	}
	session.SortByRecent(out)
	return out
}

// Restore moves a trashed session back to where it came from and returns the
// restored session. It refuses to overwrite an existing file.
func (st *Store) Restore(s *session.Session) (*session.Session, error) {
	if !s.Trashed() {
		return nil, errors.New("session is not in the trash")
	}
	dest := s.Trash.OrigPath
	if exists(dest) {
		return nil, errors.New("原位置已存在同名会话，未还原")
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return nil, err
	}
	if err := moveFile(s.Path, dest); err != nil {
		return nil, err
	}
	_ = os.Remove(metaPath(s.Path))
	modTime := time.Now()
	if fi, err := os.Stat(dest); err == nil {
		modTime = fi.ModTime()
	}
	if r := session.Rescan(st.home, s.Agent, dest, modTime); r != nil {
		return r, nil
	}
	return nil, errors.New("已还原，但无法读取该会话")
}

// Purge permanently deletes a trashed session. It cannot be undone.
func (st *Store) Purge(s *session.Session) error {
	if !s.Trashed() {
		return errors.New("session is not in the trash")
	}
	if err := os.Remove(s.Path); err != nil {
		return err
	}
	_ = os.Remove(metaPath(s.Path))
	return nil
}

// moveFile renames src to dst, falling back to copy-and-delete across devices.
func moveFile(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(dst)
		return err
	}
	if err := out.Close(); err != nil {
		os.Remove(dst)
		return err
	}
	return os.Remove(src)
}
