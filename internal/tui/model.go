// Package tui implements the interactive session picker with Bubble Tea.
package tui

import (
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"

	"github.com/iainen/agent-session-switch/internal/favorites"
	"github.com/iainen/agent-session-switch/internal/session"
	"github.com/iainen/agent-session-switch/internal/trash"
)

// Options configures the picker.
type Options struct {
	// Home is the user's home directory; it is only used to abbreviate paths.
	Home string
	// Sessions and Trashed hold the sessions and the trashed sessions per agent.
	Sessions [session.NumAgents][]*session.Session
	Trashed  [session.NumAgents][]*session.Session
	// Trash and Favs persist deletions and favorites. Both are required.
	Trash *trash.Store
	Favs  *favorites.Store
	// Query pre-fills the filter box.
	Query string
}

// view selects what the list shows.
type view int

const (
	viewSessions view = iota
	viewFavorites
	viewTrash
	numViews
)

// bucket holds one agent's sessions and trashed sessions.
type bucket struct {
	sessions []*session.Session
	trashed  []*session.Session
}

// Model is the Bubble Tea model of the picker.
type Model struct {
	opts    Options
	buckets [session.NumAgents]bucket
	agent   session.Agent
	view    view

	filtered []*session.Session // the current agent and view, after applying the filter
	cursor   int
	offset   int

	input textinput.Model
	vp    viewport.Model

	cache    map[string]*session.Preview
	loading  map[string]bool
	seq      int    // identifies the latest scheduled preview load
	shownKey string // preview currently rendered into vp (key@width)

	w, h      int
	status    string
	filtering bool // typing goes to the filter box instead of vim navigation
	confirm   *session.Session
	purge     bool // confirm is a permanent delete rather than a move to the trash
	chosen    *session.Session
}

// New creates the picker model.
func New(opts Options) Model {
	ti := textinput.New()
	ti.Prompt = ""
	ti.Placeholder = "按 / 过滤（目录 / 标题 / id）"
	ti.SetVirtualCursor(true)
	ti.SetValue(opts.Query)

	// Start on the first agent that has sessions.
	agent := session.Claude
	for a := session.Agent(0); a < session.NumAgents; a++ {
		if len(opts.Sessions[a]) > 0 {
			agent = a
			break
		}
	}
	m := Model{
		opts:    opts,
		agent:   agent,
		input:   ti,
		vp:      viewport.New(),
		cache:   map[string]*session.Preview{},
		loading: map[string]bool{},
	}
	for a := range m.buckets {
		m.buckets[a] = bucket{sessions: opts.Sessions[a], trashed: opts.Trashed[a]}
	}
	m.refilter()
	return m
}

// Chosen returns the session the user picked to open, or nil if they quit.
func (m Model) Chosen() *session.Session { return m.chosen }

// ---------- list management ----------

func (m *Model) refilter() {
	terms := strings.Fields(strings.ToLower(m.input.Value()))
	m.filtered = m.filtered[:0]
	for _, s := range m.viewList() {
		if s.Match(terms) {
			m.filtered = append(m.filtered, s)
		}
	}
	m.cursor, m.offset, m.shownKey = 0, 0, ""
}

// listChanged refreshes the current list after its content changed and
// keeps the cursor near where it was.
func (m *Model) listChanged() {
	cur := m.cursor
	m.refilter()
	m.cursor = min(cur, max(len(m.filtered)-1, 0))
}

// viewList returns the list for the current agent and view. The favorites
// view is the subset of sessions that are starred.
func (m *Model) viewList() []*session.Session {
	b := &m.buckets[m.agent]
	switch m.view {
	case viewTrash:
		return b.trashed
	case viewFavorites:
		var out []*session.Session
		for _, s := range b.sessions {
			if m.isFav(s) {
				out = append(out, s)
			}
		}
		return out
	}
	return b.sessions
}

func (m *Model) current() *session.Session {
	if m.cursor >= 0 && m.cursor < len(m.filtered) {
		return m.filtered[m.cursor]
	}
	return nil
}

func (m *Model) move(delta int) tea.Cmd {
	if len(m.filtered) == 0 {
		return nil
	}
	m.cursor = min(max(m.cursor+delta, 0), len(m.filtered)-1)
	m.status = ""
	m.shownKey = ""
	m.syncViewport()
	return m.scheduleLoad()
}

func (m *Model) setView(v view) {
	m.view = v
	m.status = ""
	m.refilter()
}

// toggleView enters v, or returns to the session list if v is already shown.
func (m *Model) toggleView(v view) {
	if m.view == v {
		v = viewSessions
	}
	m.setView(v)
}

// switchAgent changes the agent tab. It does not wrap around, like h/l in vim.
func (m *Model) switchAgent(to session.Agent) {
	if to < 0 || to >= session.NumAgents || to == m.agent {
		return
	}
	m.agent = to
	m.status = ""
	m.refilter()
}

func without(list []*session.Session, target *session.Session) []*session.Session {
	kept := make([]*session.Session, 0, len(list))
	for _, s := range list {
		if s != target {
			kept = append(kept, s)
		}
	}
	return kept
}

// ---------- favorites ----------

func (m *Model) isFav(s *session.Session) bool { return m.opts.Favs.Has(s.Agent, s.ID) }

func (m *Model) favCount(a session.Agent) int {
	n := 0
	for _, s := range m.buckets[a].sessions {
		if m.isFav(s) {
			n++
		}
	}
	return n
}

func (m *Model) toggleFav(s *session.Session) (added bool, err error) {
	added, err = m.opts.Favs.Toggle(s.Agent, s.ID)
	if err == nil {
		m.listChanged()
	}
	return added, err
}

// ---------- trash ----------

func (m *Model) doTrash(s *session.Session) error {
	ts, err := m.opts.Trash.Put(s)
	if err != nil {
		return err
	}
	delete(m.cache, s.Key())
	b := &m.buckets[m.agent]
	b.sessions = without(b.sessions, s)
	b.trashed = append([]*session.Session{ts}, b.trashed...)
	m.listChanged()
	return nil
}

func (m *Model) doRestore(s *session.Session) error {
	rs, err := m.opts.Trash.Restore(s)
	if err != nil {
		return err
	}
	delete(m.cache, s.Key())
	b := &m.buckets[m.agent]
	b.trashed = without(b.trashed, s)
	b.sessions = append(b.sessions, rs)
	session.SortByRecent(b.sessions)
	m.listChanged()
	return nil
}

func (m *Model) doPurge(s *session.Session) error {
	if err := m.opts.Trash.Purge(s); err != nil {
		return err
	}
	delete(m.cache, s.Key())
	b := &m.buckets[m.agent]
	b.trashed = without(b.trashed, s)
	// A favorite that can never be restored is just clutter. Failing to
	// clean it up must not fail the purge that already happened.
	_ = m.opts.Favs.Remove(s.Agent, s.ID)
	m.listChanged()
	return nil
}

// ---------- preview loading ----------

type previewMsg struct {
	key string
	p   *session.Preview
}

type loadTickMsg struct {
	key string
	seq int
}

const previewDelay = 80 * time.Millisecond

// scheduleLoad loads the current session's preview once the cursor has rested
// for a moment, so fast scrolling does not read many large files.
func (m *Model) scheduleLoad() tea.Cmd {
	s := m.current()
	if s == nil || m.cache[s.Key()] != nil || m.loading[s.Key()] {
		return nil
	}
	m.seq++
	key, seq := s.Key(), m.seq
	return tea.Tick(previewDelay, func(time.Time) tea.Msg { return loadTickMsg{key, seq} })
}

func loadCmd(s *session.Session) tea.Cmd {
	return func() tea.Msg { return previewMsg{s.Key(), session.LoadPreview(s)} }
}

// ---------- small helpers used by the views ----------

// when describes the session's age; trashed sessions say when they were deleted.
func when(s *session.Session) string {
	if s.Trashed() {
		return "删除于" + ago(s.ModTime)
	}
	return ago(s.ModTime)
}

// shortDir abbreviates the home directory to "~". Only whole path segments
// match, so "/Users/foobar" is not treated as being inside "/Users/foo".
func (m *Model) shortDir(s *session.Session) string {
	home := strings.TrimRight(m.opts.Home, "/")
	if home == "" { // no home, or home is "/": nothing sensible to abbreviate
		return s.Cwd
	}
	if s.Cwd == home {
		return "~"
	}
	if rest, ok := strings.CutPrefix(s.Cwd, home+"/"); ok {
		return "~/" + rest
	}
	return s.Cwd
}
