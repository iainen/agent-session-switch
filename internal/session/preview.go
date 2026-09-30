package session

import "unicode/utf8"

// Role says who wrote a preview message.
type Role int

// Message roles.
const (
	User Role = iota
	Assistant
)

// Message is one user or assistant message of a conversation.
type Message struct {
	Role Role
	Text string
}

// Preview is the readable part of a conversation.
type Preview struct {
	// Messages holds the last messages of the conversation, oldest first.
	Messages []Message
	// Total is the number of messages in the whole conversation.
	Total int
}

const (
	// maxKeep is how many trailing messages a Preview retains.
	maxKeep = 300
	// maxMessageRunes truncates very long single messages.
	maxMessageRunes = 3000
)

// LoadPreview reads the conversation of s. Tool calls, reasoning and
// agent-injected instructions are left out. Read errors yield whatever was
// parsed so far, because a preview is best effort.
func LoadPreview(s *Session) *Preview {
	if s.Agent == Codex {
		return buildPreview(s.Path, codexPreviewMessage)
	}
	return buildPreview(s.Path, claudePreviewMessage)
}

// buildPreview reads the JSONL file at path and feeds every message that
// extract recognises into a Preview. A read error leaves a partial preview,
// which is the best we can show.
func buildPreview(path string, extract func(line []byte) (Role, string, bool)) *Preview {
	var b previewBuilder
	_ = readLines(path, func(line []byte) bool {
		if role, text, ok := extract(line); ok {
			b.add(role, text)
		}
		return true
	})
	return b.build()
}

type previewBuilder struct{ p Preview }

func (b *previewBuilder) add(role Role, text string) {
	// Converting to []rune allocates 4 bytes per character, and almost every
	// message is discarded later. Only messages longer than the limit in bytes
	// can be longer in runes, so check that first.
	if len(text) > maxMessageRunes && utf8.RuneCountInString(text) > maxMessageRunes {
		text = string([]rune(text)[:maxMessageRunes]) + " …"
	}
	b.p.Total++
	b.p.Messages = append(b.p.Messages, Message{role, text})
	// Compact occasionally so memory stays bounded on huge conversations.
	if len(b.p.Messages) > maxKeep*2 {
		b.p.Messages = append([]Message(nil), b.p.Messages[len(b.p.Messages)-maxKeep:]...)
	}
}

func (b *previewBuilder) build() *Preview {
	if len(b.p.Messages) > maxKeep {
		// Copy the tail so the result does not pin the larger backing array.
		b.p.Messages = append([]Message(nil), b.p.Messages[len(b.p.Messages)-maxKeep:]...)
	}
	return &b.p
}
