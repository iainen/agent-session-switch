package session

import (
	"encoding/json"
	"regexp"
	"strings"
)

var (
	// injectedRe matches blocks that the agent inserts into user messages
	// (system reminders, pasted content, task notifications, ...).
	injectedRe = regexp.MustCompile(`(?s)<(pasted_content|system-reminder|task-notification|command-[a-z]+|local-command-[a-z]+)[^>]*>.*?</(pasted_content|system-reminder|task-notification|command-[a-z]+|local-command-[a-z]+)[^>]*>`)
	tagRe      = regexp.MustCompile(`<[^>]+>`)
	spaceRe    = regexp.MustCompile(`\s+`)
)

// stripInjected removes injected blocks but keeps the rest of the text as is.
func stripInjected(s string) string { return injectedRe.ReplaceAllString(s, " ") }

// oneLine turns arbitrary message text into a single-line title candidate.
func oneLine(s string) string {
	s = stripInjected(s)
	s = tagRe.ReplaceAllString(s, " ")
	return strings.TrimSpace(spaceRe.ReplaceAllString(s, " "))
}

// contentText extracts text from a message "content" value, which is either
// a JSON string or an array of {"text": ...} parts joined with sep.
func contentText(raw json.RawMessage, sep string) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var parts []struct {
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &parts) == nil {
		texts := make([]string, 0, len(parts))
		for _, p := range parts {
			texts = append(texts, p.Text)
		}
		return strings.Join(texts, sep)
	}
	return ""
}
