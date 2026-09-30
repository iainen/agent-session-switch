package tui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/iainen/agent-session-switch/internal/favorites"
	"github.com/iainen/agent-session-switch/internal/session"
	"github.com/iainen/agent-session-switch/internal/trash"
)

// fixture is a picker over a throwaway home directory with three Claude
// sessions (gamma is the newest) and two Codex sessions.
type fixture struct {
	t     *testing.T
	home  string
	cwd   string
	model Model
}

func writeJSON(t *testing.T, path string, mtime time.Time, lines ...any) {
	t.Helper()
	must(t, os.MkdirAll(filepath.Dir(path), 0o755))
	var sb strings.Builder
	for _, l := range lines {
		b, err := json.Marshal(l)
		must(t, err)
		sb.Write(b)
		sb.WriteByte('\n')
	}
	must(t, os.WriteFile(path, []byte(sb.String()), 0o644))
	must(t, os.Chtimes(path, mtime, mtime))
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	home, cwd := t.TempDir(), t.TempDir()
	base := time.Now().Add(-48 * time.Hour)
	for i, name := range []string{"alpha", "beta", "gamma"} {
		writeJSON(t, claudeFile(home, name), base.Add(time.Duration(i)*time.Hour),
			map[string]any{"type": "user", "cwd": cwd, "customTitle": name,
				"message": map[string]any{"content": "question " + name}},
			map[string]any{"type": "assistant", "cwd": cwd,
				"message": map[string]any{"content": []map[string]any{{"text": "answer " + name}}}})
	}
	for i, id := range []string{"01a00001-aaaa-7000-8000-000000000001", "01a00002-bbbb-7000-8000-000000000002"} {
		writeJSON(t, codexFile(home, id), base.Add(time.Duration(i)*time.Hour),
			map[string]any{"type": "session_meta", "payload": map[string]any{"id": id, "cwd": cwd}},
			map[string]any{"type": "response_item", "payload": map[string]any{
				"type": "message", "role": "user",
				"content": []map[string]any{{"text": "codex question " + string(rune('A'+i))}}}})
	}

	f := &fixture{t: t, home: home, cwd: cwd}
	opts := Options{Home: home, Trash: trash.New(home), Favs: favorites.Open(home)}
	for a := session.Agent(0); a < session.NumAgents; a++ {
		opts.Sessions[a] = session.Load(home, a, 100)
	}
	f.model = New(opts)
	f.send(tea.WindowSizeMsg{Width: 120, Height: 30})
	return f
}

func claudeFile(home, id string) string {
	return filepath.Join(home, ".claude", "projects", "-p", id+".jsonl")
}

func codexFile(home, id string) string {
	return filepath.Join(home, ".codex", "sessions", "2026", "09", "28", "rollout-2026-09-28T10-00-00-"+id+".jsonl")
}

func (f *fixture) send(msg tea.Msg) tea.Cmd {
	f.t.Helper()
	next, cmd := f.model.Update(msg)
	f.model = next.(Model)
	return cmd
}

// press sends key presses and returns the command of the last one.
func (f *fixture) press(keys ...string) tea.Cmd {
	f.t.Helper()
	var cmd tea.Cmd
	for _, k := range keys {
		cmd = f.send(keyPress(k))
	}
	return cmd
}

func keyPress(s string) tea.KeyPressMsg {
	switch s {
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "tab":
		return tea.KeyPressMsg{Code: tea.KeyTab}
	case "up":
		return tea.KeyPressMsg{Code: tea.KeyUp}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	case "left":
		return tea.KeyPressMsg{Code: tea.KeyLeft}
	case "right":
		return tea.KeyPressMsg{Code: tea.KeyRight}
	case "ctrl+c":
		return tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}
	}
	r := []rune(s)
	return tea.KeyPressMsg{Code: r[0], Text: s}
}

func (f *fixture) titles() []string {
	var out []string
	for _, s := range f.model.filtered {
		out = append(out, s.Title)
	}
	return out
}

func (f *fixture) current() string {
	if s := f.model.current(); s != nil {
		return s.Title
	}
	return ""
}

func isQuit(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.QuitMsg)
	return ok
}

func exists(p string) bool { _, err := os.Stat(p); return err == nil }

func TestKeyPressStrings(t *testing.T) {
	// The tests below rely on these synthetic key presses matching what a
	// real terminal produces, as far as String() is concerned.
	for _, s := range []string{"j", "k", "G", "g", "/", "f", "F", "d", "y", "enter", "esc", "tab", "ctrl+c", "left", "right"} {
		if got := keyPress(s).String(); got != s {
			t.Errorf("keyPress(%q).String() = %q", s, got)
		}
	}
}

func TestInitialState(t *testing.T) {
	f := newFixture(t)
	if got := strings.Join(f.titles(), ","); got != "gamma,beta,alpha" {
		t.Errorf("titles = %s, want newest first", got)
	}
	if f.model.agent != session.Claude || f.model.view != viewSessions {
		t.Errorf("agent=%v view=%v", f.model.agent, f.model.view)
	}
}

func TestStartsOnCodexWhenClaudeIsEmpty(t *testing.T) {
	m := New(Options{
		Trash: trash.New(t.TempDir()), Favs: favorites.Open(t.TempDir()),
		Sessions: [session.NumAgents][]*session.Session{session.Codex: {{Agent: session.Codex, ID: "x", Title: "only codex"}}},
	})
	if m.agent != session.Codex {
		t.Errorf("agent = %v, want Codex", m.agent)
	}
}

func TestNavigation(t *testing.T) {
	f := newFixture(t)
	tests := []struct {
		keys []string
		want string
	}{
		{[]string{"j"}, "beta"},
		{[]string{"j", "j"}, "alpha"},
		{[]string{"j", "j", "j", "j"}, "alpha"}, // stops at the end
		{[]string{"j", "k"}, "gamma"},
		{[]string{"k"}, "gamma"}, // stops at the start
		{[]string{"G"}, "alpha"},
		{[]string{"G", "g"}, "gamma"},
		{[]string{"down", "down", "up"}, "beta"},
	}
	for _, tt := range tests {
		f.press("g")
		f.press(tt.keys...)
		if got := f.current(); got != tt.want {
			t.Errorf("after %v: at %q, want %q", tt.keys, got, tt.want)
		}
	}
}

func TestSwitchAgentDoesNotWrap(t *testing.T) {
	f := newFixture(t)
	f.press("h")
	if f.model.agent != session.Claude {
		t.Error("h at the first tab should stay put")
	}
	f.press("l")
	if f.model.agent != session.Codex || len(f.model.filtered) != 2 {
		t.Fatalf("agent=%v, %d sessions", f.model.agent, len(f.model.filtered))
	}
	f.press("l", "right")
	if f.model.agent != session.Codex {
		t.Error("l at the last tab should stay put")
	}
	f.press("left")
	if f.model.agent != session.Claude {
		t.Error("left arrow should go back to Claude")
	}
}

func TestTypingInFilterModeIsNotNavigation(t *testing.T) {
	f := newFixture(t)
	f.press("/")
	if !f.model.filtering {
		t.Fatal("/ should start filtering")
	}
	f.press("j", "k")
	if f.model.input.Value() != "jk" {
		t.Errorf("filter = %q, want the typed jk", f.model.input.Value())
	}
	if f.model.cursor != 0 {
		t.Errorf("cursor moved to %d while typing", f.model.cursor)
	}
}

func TestFilterNarrowsList(t *testing.T) {
	f := newFixture(t)
	f.press("/", "b", "e", "t")
	if got := strings.Join(f.titles(), ","); got != "beta" {
		t.Errorf("titles = %q, want beta", got)
	}
	f.press("esc")
	if f.model.filtering {
		t.Error("esc should leave filter mode")
	}
	if got := strings.Join(f.titles(), ","); got != "beta" {
		t.Errorf("esc dropped the filter: %q", got)
	}
	f.press("x", "z") // ignored while navigating
	if f.model.input.Value() != "bet" {
		t.Errorf("navigation keys leaked into the filter: %q", f.model.input.Value())
	}
}

func TestOpen(t *testing.T) {
	f := newFixture(t)
	f.press("j")
	cmd := f.press("enter")
	if !isQuit(cmd) {
		t.Fatal("enter should quit")
	}
	if got := f.model.Chosen(); got == nil || got.Title != "beta" {
		t.Errorf("chosen = %+v, want beta", got)
	}
}

func TestOpenRefusesMissingDirectory(t *testing.T) {
	f := newFixture(t)
	f.model.current().Missing = true
	if cmd := f.press("enter"); isQuit(cmd) || f.model.Chosen() != nil {
		t.Error("a session whose directory is gone must not be opened")
	}
	if !strings.Contains(f.model.status, "原目录已不存在") {
		t.Errorf("status = %q", f.model.status)
	}
}

func TestQuitKeys(t *testing.T) {
	for _, k := range []string{"q", "esc", "ctrl+c"} {
		f := newFixture(t)
		if !isQuit(f.press(k)) {
			t.Errorf("%s should quit", k)
		}
		if f.model.Chosen() != nil {
			t.Errorf("%s must not choose a session", k)
		}
	}
	f := newFixture(t)
	f.press("/")
	if !isQuit(f.press("ctrl+c")) {
		t.Error("ctrl+c should quit while filtering too")
	}
}

func TestDeleteCancelAndConfirm(t *testing.T) {
	f := newFixture(t)
	path := claudeFile(f.home, "gamma")

	f.press("d")
	if f.model.confirm == nil {
		t.Fatal("d should ask for confirmation")
	}
	f.press("n")
	if f.model.confirm != nil || !exists(path) {
		t.Fatal("any key other than y must cancel and keep the file")
	}

	f.press("d", "esc")
	if !exists(path) {
		t.Fatal("esc must cancel the deletion")
	}

	f.press("d", "y")
	if exists(path) {
		t.Error("session file still in place after confirming")
	}
	if got := strings.Join(f.titles(), ","); got != "beta,alpha" {
		t.Errorf("titles = %s", got)
	}
	if n := len(f.model.opts.Trash.List()); n != 1 {
		t.Errorf("trash has %d entries, want 1", n)
	}
}

func TestTrashRestoreAndPurge(t *testing.T) {
	f := newFixture(t)
	gamma := claudeFile(f.home, "gamma")
	f.press("d", "y", "t")
	if f.model.view != viewTrash || f.current() != "gamma" {
		t.Fatalf("view=%v current=%q", f.model.view, f.current())
	}

	f.press("d") // not available in the trash
	if f.model.confirm != nil {
		t.Error("d must do nothing in the trash view")
	}

	f.press("r")
	if !exists(gamma) {
		t.Fatal("r should restore the session")
	}
	if len(f.model.buckets[session.Claude].trashed) != 0 || len(f.model.buckets[session.Claude].sessions) != 3 {
		t.Errorf("buckets not updated: %+v", f.model.buckets[session.Claude])
	}

	f.press("q") // back to sessions
	if f.model.view != viewSessions || f.titles()[0] != "gamma" {
		t.Errorf("view=%v titles=%v", f.model.view, f.titles())
	}

	f.press("d", "y", "t", "x")
	if f.model.confirm == nil || !f.model.purge {
		t.Fatal("x should ask before deleting for good")
	}
	f.press("n")
	if len(f.model.opts.Trash.List()) != 1 {
		t.Fatal("cancelled purge removed the session")
	}
	f.press("x", "y")
	if len(f.model.opts.Trash.List()) != 0 {
		t.Error("purge did not delete the session")
	}
}

func TestEnterInTrashRestores(t *testing.T) {
	f := newFixture(t)
	f.press("d", "y", "t")
	if cmd := f.press("enter"); isQuit(cmd) {
		t.Error("enter in the trash should restore, not open")
	}
	if !exists(claudeFile(f.home, "gamma")) {
		t.Error("session was not restored")
	}
}

func TestTrashIsPerAgent(t *testing.T) {
	f := newFixture(t)
	f.press("l", "d", "y")
	f.press("h", "t")
	if len(f.model.filtered) != 0 {
		t.Errorf("Claude trash shows %d entries after deleting a Codex session", len(f.model.filtered))
	}
	f.press("l")
	if len(f.model.filtered) != 1 {
		t.Errorf("Codex trash shows %d entries, want 1", len(f.model.filtered))
	}
}

func TestFavorites(t *testing.T) {
	f := newFixture(t)
	f.press("f") // gamma
	if !f.model.isFav(f.model.current()) {
		t.Fatal("f should star the current session")
	}
	f.press("j", "j", "f") // alpha

	f.press("F")
	if f.model.view != viewFavorites {
		t.Fatalf("view = %v", f.model.view)
	}
	if got := strings.Join(f.titles(), ","); got != "gamma,alpha" {
		t.Errorf("favorites = %s", got)
	}

	f.press("f") // unstar gamma while in the favorites view
	if got := strings.Join(f.titles(), ","); got != "alpha" {
		t.Errorf("after unstarring: %s", got)
	}

	f.press("l")
	if len(f.model.filtered) != 0 {
		t.Error("Claude favorites leaked into the Codex tab")
	}
	f.press("F") // toggles back to the session list
	if f.model.view != viewSessions {
		t.Errorf("F in favorites should return to sessions, view = %v", f.model.view)
	}
}

func TestFavoritesPersistAndSurviveTrash(t *testing.T) {
	f := newFixture(t)
	f.press("f", "d", "y")
	if !f.model.opts.Favs.Has(session.Claude, "gamma") {
		t.Error("trashing must keep the favorite")
	}
	f.press("t", "f")
	if !f.model.opts.Favs.Has(session.Claude, "gamma") {
		t.Error("f must do nothing in the trash view")
	}
	f.press("r", "F")
	if got := strings.Join(f.titles(), ","); got != "gamma" {
		t.Errorf("restored favorite missing from favorites: %q", got)
	}

	f.press("q", "d", "y", "t", "x", "y")
	if f.model.opts.Favs.Has(session.Claude, "gamma") {
		t.Error("purging a session should drop its favorite")
	}
}

func TestViewCycleWithTab(t *testing.T) {
	f := newFixture(t)
	want := []view{viewFavorites, viewTrash, viewSessions}
	for i, w := range want {
		f.press("tab")
		if f.model.view != w {
			t.Errorf("after %d tabs: view = %v, want %v", i+1, f.model.view, w)
		}
	}
	f.press("t", "t")
	if f.model.view != viewSessions {
		t.Error("t twice should return to sessions")
	}
}

func TestBackKeysInSubViews(t *testing.T) {
	for _, enter := range []string{"F", "t"} {
		f := newFixture(t)
		f.press(enter)
		if isQuit(f.press("esc")) {
			t.Errorf("esc in view entered with %s must go back, not quit", enter)
		}
		if f.model.view != viewSessions {
			t.Errorf("view = %v after esc", f.model.view)
		}
	}
}

func TestPreviewLoadsAfterCursorRests(t *testing.T) {
	f := newFixture(t)
	key := f.model.current().Key()
	if f.model.cache[key] != nil {
		t.Fatal("preview should not be loaded before the cursor rests")
	}

	stale := f.send(tea.WindowSizeMsg{Width: 120, Height: 30}) // schedules a load
	fresh := f.send(tea.WindowSizeMsg{Width: 120, Height: 30}) // supersedes it
	if stale == nil || fresh == nil {
		t.Fatal("expected a delayed load to be scheduled")
	}
	if next := f.send(stale()); next != nil {
		t.Error("a superseded tick must not start a load")
	}
	load := f.send(fresh())
	if load == nil {
		t.Fatal("the latest tick should start a load")
	}
	f.send(load())

	p := f.model.cache[key]
	if p == nil || p.Total != 2 {
		t.Fatalf("preview = %+v, want 2 messages", p)
	}
	if out := ansi.Strip(f.model.View().Content); !strings.Contains(out, "answer gamma") {
		t.Error("loaded preview is not rendered")
	}
}

func TestLayout(t *testing.T) {
	f := newFixture(t)
	if _, right, _ := f.model.layout(); right == 0 {
		t.Error("a 120-column terminal should show the preview")
	}
	f.send(tea.WindowSizeMsg{Width: minSplitWidth - 1, Height: 30})
	if _, right, _ := f.model.layout(); right != 0 {
		t.Error("a narrow terminal should hide the preview")
	}
}

func TestViewRenders(t *testing.T) {
	f := newFixture(t)
	out := ansi.Strip(f.model.View().Content)
	for _, want := range []string{"Claude 3", "Codex 2", "gamma", "beta", "alpha", "1 / 3", "q 退出"} {
		if !strings.Contains(out, want) {
			t.Errorf("view is missing %q", want)
		}
	}
	f.press("d")
	if out := ansi.Strip(f.model.View().Content); !strings.Contains(out, "删除会话「gamma」") {
		t.Errorf("confirmation prompt missing:\n%s", out)
	}
}

func TestFitLine(t *testing.T) {
	for _, tt := range []struct {
		in string
		w  int
	}{{"abc", 10}, {"这是一个很长的中文标题", 9}, {"mixed 中文 text", 8}, {"x", 1}} {
		if got := ansi.StringWidth(fitLine(tt.in, tt.w)); got != tt.w {
			t.Errorf("fitLine(%q, %d) has width %d", tt.in, tt.w, got)
		}
	}
	if fitLine("x", 0) != "" {
		t.Error("width 0 should give an empty string")
	}
}

func TestAgo(t *testing.T) {
	now := time.Now()
	for _, tt := range []struct {
		d    time.Duration
		want string
	}{
		{10 * time.Second, "刚刚"},
		{5 * time.Minute, "5分钟前"},
		{3*time.Hour + time.Minute, "3小时前"},
		{50 * time.Hour, "2天前"},
	} {
		if got := ago(now.Add(-tt.d)); got != tt.want {
			t.Errorf("ago(-%v) = %q, want %q", tt.d, got, tt.want)
		}
	}
}
