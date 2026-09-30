package session

import (
	"reflect"
	"strings"
	"testing"
)

func TestAgent(t *testing.T) {
	tests := []struct {
		agent  Agent
		name   string
		bin    string
		resume []string
	}{
		{Claude, "Claude", "claude", []string{"--resume", "abc"}},
		{Codex, "Codex", "codex", []string{"resume", "abc"}},
	}
	for _, tt := range tests {
		if got := tt.agent.String(); got != tt.name {
			t.Errorf("%d String() = %q, want %q", tt.agent, got, tt.name)
		}
		if got := tt.agent.Bin(); got != tt.bin {
			t.Errorf("%s Bin() = %q, want %q", tt.name, got, tt.bin)
		}
		if got := tt.agent.ResumeArgs("abc"); !reflect.DeepEqual(got, tt.resume) {
			t.Errorf("%s ResumeArgs() = %v, want %v", tt.name, got, tt.resume)
		}
	}
	if NumAgents != 2 {
		t.Errorf("NumAgents = %d, want 2", NumAgents)
	}
}

func TestMatch(t *testing.T) {
	s := &Session{ID: "AbC-123", Cwd: "/Work/Proj", Title: "Fix 登录 bug"}
	tests := []struct {
		terms []string
		want  bool
	}{
		{nil, true},
		{[]string{"proj"}, true},
		{[]string{"登录", "fix"}, true}, // terms are ANDed
		{[]string{"abc-123"}, true},
		{[]string{"fix", "missing"}, false},
	}
	for _, tt := range tests {
		if got := s.Match(tt.terms); got != tt.want {
			t.Errorf("Match(%v) = %v, want %v", tt.terms, got, tt.want)
		}
	}
}

func TestStripInjectedAndOneLine(t *testing.T) {
	in := "<system-reminder>hidden\nstuff</system-reminder>  hello\n\n<b>world</b>"
	if got := stripInjected(in); got == in || strings.Contains(got, "hidden") {
		t.Errorf("stripInjected kept injected block: %q", got)
	}
	if got, want := oneLine(in), "hello world"; got != want {
		t.Errorf("oneLine() = %q, want %q", got, want)
	}
}

func TestReadLinesSkipsOversizedLines(t *testing.T) {
	old := maxLineBytes
	maxLineBytes = 64
	t.Cleanup(func() { maxLineBytes = old })

	path := t.TempDir() + "/f.jsonl"
	long := make([]byte, 500)
	for i := range long {
		long[i] = 'x'
	}
	writeJSONL(t, path, t0, m{"n": 1}, m{"big": string(long)}, m{"n": 3})

	var got int
	if err := readLines(path, func([]byte) bool { got++; return true }); err != nil {
		t.Fatal(err)
	}
	if got != 2 {
		t.Errorf("read %d lines, want 2 (oversized line skipped)", got)
	}
}
