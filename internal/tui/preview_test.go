package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/iainen/agent-session-switch/internal/session"
)

func previewOf(msgs ...session.Message) *session.Preview {
	return &session.Preview{Messages: msgs, Total: len(msgs)}
}

func lines(s string) []string { return strings.Split(ansi.Strip(s), "\n") }

func TestBubblesAlignBySpeaker(t *testing.T) {
	const w = 60
	got := lines(renderPreview(previewOf(
		session.Message{Role: session.User, Text: "hi"},
		session.Message{Role: session.Assistant, Text: "hello"},
	), w, session.Claude))

	var userTop, aiTop string
	for _, l := range got {
		switch {
		case strings.Contains(l, "─ 你 "):
			userTop = l
		case strings.Contains(l, "─ Claude "):
			aiTop = l
		}
	}
	if userTop == "" || aiTop == "" {
		t.Fatalf("missing titled borders:\n%s", strings.Join(got, "\n"))
	}
	if !strings.HasPrefix(aiTop, "╭") {
		t.Errorf("assistant bubble should start at the left edge: %q", aiTop)
	}
	if !strings.HasPrefix(userTop, " ") || !strings.HasSuffix(userTop, "╮") {
		t.Errorf("user bubble should be flush right: %q", userTop)
	}
	if width := ansi.StringWidth(userTop); width != w {
		t.Errorf("user bubble ends at column %d, want %d", width, w)
	}
}

func TestBubbleUsesAgentName(t *testing.T) {
	out := ansi.Strip(renderPreview(previewOf(session.Message{Role: session.Assistant, Text: "x"}), 40, session.Codex))
	if !strings.Contains(out, "─ Codex ") {
		t.Errorf("title should name the agent:\n%s", out)
	}
}

func TestBubbleLinesHaveEqualWidth(t *testing.T) {
	long := strings.Repeat("很长的中文内容 mixed with English words ", 8)
	for _, w := range []int{14, 20, 33, 60, 100} {
		p := previewOf(
			session.Message{Role: session.Assistant, Text: long},
			session.Message{Role: session.User, Text: "短"},
			session.Message{Role: session.Assistant, Text: "a\n\nb\tc"},
		)
		var box []string
		for _, l := range lines(renderPreview(p, w, session.Claude)) {
			if w := ansi.StringWidth(l); w > 0 && strings.TrimSpace(l) != "" {
				box = append(box, l)
			}
			if ansi.StringWidth(l) > w {
				t.Errorf("width %d: a line is %d columns wide", w, ansi.StringWidth(l))
			}
		}
		if len(box) == 0 {
			t.Fatalf("width %d: nothing rendered", w)
		}
	}
}

func TestEachBubbleIsARectangle(t *testing.T) {
	long := strings.Repeat("中文 word ", 30)
	out := lines(bubble("你", long, 30))
	want := ansi.StringWidth(out[0])
	for i, l := range out {
		if got := ansi.StringWidth(l); got != want {
			t.Errorf("line %d is %d columns, want %d: %q", i, got, want, l)
		}
	}
	if !strings.HasPrefix(out[0], "╭─ 你 ") || !strings.HasSuffix(out[0], "╮") ||
		!strings.HasPrefix(out[len(out)-1], "╰") || !strings.HasSuffix(out[len(out)-1], "╯") {
		t.Errorf("border corners wrong:\n%s", strings.Join(out, "\n"))
	}
}

func TestBubbleTitleAlwaysFits(t *testing.T) {
	out := lines(bubble("Claude", "x", 30))
	if !strings.Contains(out[0], "─ Claude ") {
		t.Errorf("a one-character message must still show the full title: %q", out[0])
	}
}

func TestEmptyPreview(t *testing.T) {
	if got := ansi.Strip(renderPreview(previewOf(), 40, session.Claude)); !strings.Contains(got, "没有可显示的对话内容") {
		t.Errorf("empty preview text = %q", got)
	}
}

func TestNoBackgroundOrColorInPreview(t *testing.T) {
	out := renderPreview(previewOf(
		session.Message{Role: session.User, Text: "a"},
		session.Message{Role: session.Assistant, Text: "b"},
	), 40, session.Claude)
	for _, seq := range []string{"\x1b[7m", "\x1b[7;", ";7m", "\x1b[4"} {
		if strings.Contains(out, seq) {
			t.Errorf("preview uses reverse video / background attribute %q", seq)
		}
	}
	if strings.Contains(out, "48;") || strings.Contains(out, "38;2") || strings.Contains(out, "38;5") {
		t.Error("preview sets a color")
	}
}

// The list pads every line with spaces to the column width. If such a line
// carried an underline it would draw a rule across the padding.
func TestListLinesAreNeverUnderlined(t *testing.T) {
	f := newFixture(t)
	// Use a short directory so the marker is not cut off by the column width.
	f.model.filtered[1].Missing = true
	f.model.filtered[1].Cwd = "/gone"
	f.model.cursor = 0
	out := f.model.renderList(50, 20)
	if strings.Contains(out, "\x1b[4m") || strings.Contains(out, "\x1b[4;") || strings.Contains(out, ";4m") {
		t.Error("a list row is underlined")
	}
	if !strings.Contains(ansi.Strip(out), "[目录不存在]") {
		t.Error("a session whose directory is gone should still be marked")
	}
}

func TestShortDir(t *testing.T) {
	tests := []struct {
		name, home, cwd, want string
	}{
		{"inside home", "/Users/foo", "/Users/foo/code/app", "~/code/app"},
		{"home itself", "/Users/foo", "/Users/foo", "~"},
		{"trailing slash on home", "/Users/foo/", "/Users/foo/code", "~/code"},
		{"sibling sharing the prefix", "/Users/foo", "/Users/foobar/code", "/Users/foobar/code"},
		{"sibling, exact prefix", "/Users/foo", "/Users/foobar", "/Users/foobar"},
		{"outside home", "/Users/foo", "/opt/app", "/opt/app"},
		{"empty home", "", "/opt/app", "/opt/app"},
		{"root as home", "/", "/opt/app", "/opt/app"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &Model{opts: Options{Home: tt.home}}
			if got := m.shortDir(&session.Session{Cwd: tt.cwd}); got != tt.want {
				t.Errorf("shortDir(%q) with home %q = %q, want %q", tt.cwd, tt.home, got, tt.want)
			}
		})
	}
}
