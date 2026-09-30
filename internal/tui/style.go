package tui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// The interface is monochrome. It never sets a color, so it uses whatever
// foreground and background the terminal has, and tells things apart with
// text attributes only: bold, faint (dimmed) and reverse video.
//
//	plain   normal text
//	dim     secondary information (faint)
//	strong  emphasis (bold)
//	warn    problems and destructive prompts (bold + underline)
//	badge   a label (reverse video)
var (
	plain  = lipgloss.NewStyle()
	dim    = lipgloss.NewStyle().Faint(true)
	strong = lipgloss.NewStyle().Bold(true)
	warn   = lipgloss.NewStyle().Bold(true).Underline(true)
)

// badge renders a label in reverse video, which works on any theme.
func badge(text string) string { return lipgloss.NewStyle().Reverse(true).Bold(true).Render(text) }

// selected styles the highlighted list row.
func selected() lipgloss.Style { return lipgloss.NewStyle().Reverse(true) }

// panel is a rounded box. Lipgloss renders the border with a foreground
// color only, so an unfocused (dimmed) border is drawn by the caller
// passing dimBorder; a focused one keeps the terminal's normal foreground.
func panel(focused bool) lipgloss.Style {
	st := lipgloss.NewStyle().Border(lipgloss.RoundedBorder())
	if !focused {
		st = st.BorderForeground(lipgloss.BrightBlack)
	}
	return st
}

// fitLine truncates or pads s to exactly w terminal columns (wide runes
// count as two), ending a truncated line with an ellipsis.
func fitLine(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if ansi.StringWidth(s) > w {
		s = ansi.Truncate(s, w, "…")
	}
	return padTo(s, w)
}

// fitPlain is like fitLine but truncates without an ellipsis. It is used for
// the text input, where an ellipsis would look like typed text.
func fitPlain(s string, w int) string {
	return padTo(ansi.Truncate(s, w, ""), w)
}

func padTo(s string, w int) string {
	if pad := w - ansi.StringWidth(s); pad > 0 {
		s += strings.Repeat(" ", pad)
	}
	return s
}

// ago formats how long ago t was, in Chinese.
func ago(t time.Time) string {
	d := time.Since(t)
	switch {
	case d >= 24*time.Hour:
		return fmt.Sprintf("%d天前", int(d/(24*time.Hour)))
	case d >= time.Hour:
		return fmt.Sprintf("%d小时前", int(d/time.Hour))
	case d >= time.Minute:
		return fmt.Sprintf("%d分钟前", int(d/time.Minute))
	}
	return "刚刚"
}
