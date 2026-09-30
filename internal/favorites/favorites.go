// Package favorites stores which sessions the user has starred.
//
// Favorites are kept in <home>/.claude/session-favorites.json, keyed by agent
// and session ID, so they survive a session being trashed and restored.
package favorites

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/iainen/agent-session-switch/internal/session"
)

type key struct {
	agent session.Agent
	id    string
}

// Store is a set of favorite sessions backed by a JSON file.
type Store struct {
	path  string
	items map[key]time.Time
	err   error
}

type entry struct {
	Agent   int       `json:"agent"`
	ID      string    `json:"id"`
	AddedAt time.Time `json:"addedAt"`
}

type file struct {
	Items []entry `json:"items"`
}

// ErrUnreadable is returned by mutating calls when the favorites file exists
// but could not be parsed. Writing would overwrite the user's data, so the
// store becomes read-only instead.
var ErrUnreadable = errors.New("收藏文件无法读取，为避免覆盖已禁用收藏")

// Open loads the favorites under home. A missing file is an empty store.
// Open never fails: if the file cannot be read the store is empty and
// read-only, and Err reports why.
func Open(home string) *Store {
	st := &Store{
		path:  filepath.Join(home, ".claude", "session-favorites.json"),
		items: map[key]time.Time{},
	}
	data, err := os.ReadFile(st.path)
	if errors.Is(err, os.ErrNotExist) {
		return st
	}
	if err != nil {
		st.err = err
		return st
	}
	var f file
	if err := json.Unmarshal(data, &f); err != nil {
		st.err = err
		return st
	}
	for _, e := range f.Items {
		st.items[key{session.Agent(e.Agent), e.ID}] = e.AddedAt
	}
	return st
}

// Err returns the error that made the store read-only, or nil.
func (st *Store) Err() error { return st.err }

// Has reports whether the session is a favorite.
func (st *Store) Has(a session.Agent, id string) bool {
	_, ok := st.items[key{a, id}]
	return ok
}

// Toggle adds the session to the favorites, or removes it if it is already
// there. It reports whether the session is now a favorite. On a write error
// the in-memory state is rolled back.
func (st *Store) Toggle(a session.Agent, id string) (added bool, err error) {
	had := st.Has(a, id)
	if err := st.set(key{a, id}, !had); err != nil {
		return had, err
	}
	return !had, nil
}

// Remove drops the session from the favorites if present.
func (st *Store) Remove(a session.Agent, id string) error {
	if st.err != nil {
		return ErrUnreadable
	}
	if !st.Has(a, id) {
		return nil
	}
	return st.set(key{a, id}, false)
}

// set adds or removes k and saves. If saving fails the change is undone, so
// memory never disagrees with the file.
func (st *Store) set(k key, present bool) error {
	if st.err != nil {
		return ErrUnreadable
	}
	old, had := st.items[k]
	if present {
		st.items[k] = time.Now()
	} else {
		delete(st.items, k)
	}
	if err := st.save(); err != nil {
		if had {
			st.items[k] = old
		} else {
			delete(st.items, k)
		}
		return err
	}
	return nil
}

// save writes the file atomically: a temporary file is renamed over it.
func (st *Store) save() error {
	f := file{Items: make([]entry, 0, len(st.items))}
	for k, t := range st.items {
		f.Items = append(f.Items, entry{Agent: int(k.agent), ID: k.id, AddedAt: t})
	}
	sort.Slice(f.Items, func(i, j int) bool {
		a, b := f.Items[i], f.Items[j]
		if !a.AddedAt.Equal(b.AddedAt) {
			return a.AddedAt.Before(b.AddedAt)
		}
		if a.Agent != b.Agent {
			return a.Agent < b.Agent
		}
		return a.ID < b.ID
	})
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(st.path), 0o755); err != nil {
		return err
	}
	tmp := st.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, st.path)
}
