package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/iainen/agent-session-switch/internal/session"
)

const (
	headerH = 1
	inputH  = 3
	footerH = 1

	// minSplitWidth is the terminal width below which the preview is hidden.
	minSplitWidth = 80
)

// layout returns the widths of the list and preview panels and the height
// available inside them.
func (m *Model) layout() (leftW, rightW, innerH int) {
	innerH = max(m.h-headerH-inputH-footerH-2, 2)
	if m.w < minSplitWidth {
		return m.w, 0, innerH
	}
	leftW = min(max(m.w*38/100, 34), 56)
	return leftW, m.w - leftW, innerH
}

// syncViewport loads the current preview into the viewport, once per
// session and width.
func (m *Model) syncViewport() {
	_, rightW, innerH := m.layout()
	if rightW == 0 {
		return
	}
	iw := rightW - 2 - 2 // border + horizontal padding
	m.vp.SetWidth(iw)
	m.vp.SetHeight(max(innerH-3, 1))
	s := m.current()
	if s == nil {
		m.vp.SetContent("")
		return
	}
	p := m.cache[s.Key()]
	key := fmt.Sprintf("%s@%d", s.Key(), iw)
	if p == nil || key == m.shownKey {
		return
	}
	m.shownKey = key
	m.vp.SetContent(renderPreview(p, iw, s.Agent))
	m.vp.GotoBottom()
}

// bubbleFrac is the widest a message bubble may be, as a share of the
// preview width. The remainder keeps the two speakers visibly apart.
const bubbleFrac = 80

// minBubbleInner is the narrowest usable text area inside a bubble.
const minBubbleInner = 8

// renderPreview draws the conversation as chat bubbles: the user's messages
// are boxed and aligned to the right, the agent's to the left. The speaker's
// name sits in the top border, so no background color is needed.
func renderPreview(p *session.Preview, w int, agent session.Agent) string {
	if len(p.Messages) == 0 {
		return dim.Render("(没有可显示的对话内容)")
	}
	maxOuter := max(w*bubbleFrac/100, minBubbleInner+4)
	maxOuter = min(maxOuter, w)
	maxInner := max(maxOuter-4, 1) // border + one space of padding on each side

	var out []string
	for i, msg := range p.Messages {
		if i > 0 {
			out = append(out, "")
		}
		name, right := agent.String(), false
		if msg.Role == session.User {
			name, right = "你", true
		}
		box := bubble(name, msg.Text, maxInner)
		for _, line := range strings.Split(box, "\n") {
			if right {
				line = strings.Repeat(" ", max(w-ansi.StringWidth(line), 0)) + line
			}
			out = append(out, line)
		}
	}
	return strings.Join(out, "\n")
}

// bubble draws text inside a rounded box titled with name. The box is only as
// wide as its content, up to maxInner columns of text.
func bubble(name, text string, maxInner int) string {
	wrapped := ansi.Wrap(strings.ReplaceAll(text, "\t", "    "), maxInner, "")
	lines := strings.Split(wrapped, "\n")
	for i := range lines {
		lines[i] = strings.TrimRight(lines[i], " \r")
	}
	inner := ansi.StringWidth(name) + 2 // the title must fit on the top border
	for _, l := range lines {
		inner = max(inner, ansi.StringWidth(l))
	}
	inner = min(max(inner, minBubbleInner), maxInner)

	b := lipgloss.RoundedBorder()
	title := strong.Render(" " + name + " ")
	// inner+2 columns between the corners: one padding space on each side.
	top := b.TopLeft + b.Top + title + strings.Repeat(b.Top, max(inner+2-1-ansi.StringWidth(title), 0)) + b.TopRight

	var sb strings.Builder
	sb.WriteString(dim.Render(top))
	for _, l := range lines {
		sb.WriteString("\n")
		sb.WriteString(dim.Render(b.Left) + " " + padTo(l, inner) + " " + dim.Render(b.Right))
	}
	sb.WriteString("\n")
	sb.WriteString(dim.Render(b.BottomLeft + strings.Repeat(b.Bottom, inner+2) + b.BottomRight))
	return sb.String()
}

// View implements tea.Model.
func (m Model) View() tea.View {
	var v tea.View
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	if m.w == 0 {
		return v
	}
	m.syncViewport()
	leftW, rightW, innerH := m.layout()

	input := panel(m.filtering).
		Render(strong.Render("❯ ") + fitPlain(m.input.View(), max(m.w-4, 10)))

	body := panel(!m.filtering).Render(m.renderList(leftW-2, innerH))
	if rightW > 0 {
		right := panel(false).Padding(0, 1).Render(m.renderRight(rightW-4, innerH))
		body = lipgloss.JoinHorizontal(lipgloss.Top, body, right)
	}

	v.SetContent(strings.Join([]string{m.renderHeader(), input, body, m.renderFooter()}, "\n"))
	return v
}

func (m *Model) renderHeader() string {
	var b strings.Builder
	for a := session.Agent(0); a < session.NumAgents; a++ {
		label := fmt.Sprintf(" %s %d ", a, len(m.buckets[a].sessions))
		if a == m.agent {
			b.WriteString(badge(label))
		} else {
			b.WriteString(dim.Render(label))
		}
	}
	switch m.view {
	case viewTrash:
		b.WriteString(" " + badge(" ♻ 回收站 "))
	case viewFavorites:
		b.WriteString(" " + badge(" ★ 收藏 "))
	}
	b.WriteString(dim.Render(fmt.Sprintf("  %d / %d", min(m.cursor+1, len(m.filtered)), len(m.filtered))))
	return b.String()
}

func (m *Model) renderFooter() string {
	if m.confirm != nil {
		question, note := "删除会话「%s」？ y 确认 / 其他键取消 ", " 会移到回收站，可还原"
		if m.purge {
			question, note = "永久删除「%s」？ y 确认 / 其他键取消 ", " 无法恢复！"
		}
		title := ansi.Truncate(m.confirm.Title, max(m.w-50, 10), "…")
		return warn.Render(fmt.Sprintf(question, title)) + dim.Render(note)
	}
	if m.status != "" {
		return strong.Render(ansi.Truncate(m.status, m.w, "…"))
	}
	return dim.Render(ansi.Truncate(m.help(), m.w, "…"))
}

func (m *Model) help() string {
	switch {
	case m.filtering:
		return "输入过滤 · ↑↓ 选择 · Enter 确认 · Esc 返回导航"
	case m.view == viewTrash:
		return "h/l 切换 · j/k 选择 · r/Enter 还原 · x 永久删除 · / 过滤 · Tab 切视图 · t/q 返回"
	case m.view == viewFavorites:
		return "h/l 切换 · j/k 选择 · f 取消收藏 · Enter 打开 · d 删除 · / 过滤 · Tab 切视图 · F/q 返回"
	}
	return fmt.Sprintf("h/l 切换 · j/k 选择 · f 收藏 · F 收藏夹(%d) · t 回收站(%d) · d 删除 · Enter 打开 · q 退出",
		m.favCount(m.agent), len(m.buckets[m.agent].trashed))
}

func (m *Model) renderList(w, h int) string {
	perPage := max(h/2, 1)
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+perPage {
		m.offset = m.cursor - perPage + 1
	}
	lines := make([]string, 0, h)
	if len(m.filtered) == 0 {
		lines = append(lines, dim.Render(fitLine(m.emptyMessage(), w)))
	}
	bar := strong.Render("▌")
	for i := m.offset; i < len(m.filtered) && len(lines) < h-1; i++ {
		s := m.filtered[i]
		title := s.Title
		if m.view != viewTrash && m.isFav(s) {
			title = "★ " + title
		}
		meta := when(s) + " · " + m.shortDir(s)
		if s.Missing {
			meta += " [目录不存在]"
		}
		if i == m.cursor {
			sel := selected()
			lines = append(lines,
				bar+sel.Bold(true).Render(fitLine(" "+title, w-1)),
				bar+sel.Faint(true).Render(fitLine(" "+meta, w-1)))
			continue
		}
		// A directory that is gone is already marked by "[目录不存在]" in meta.
		// Do not underline it: the line is padded with spaces to the column
		// width, and the underline would run across the padding.
		lines = append(lines,
			plain.Render(fitLine("  "+title, w)),
			dim.Render(fitLine("  "+meta, w)))
	}
	for len(lines) < h {
		lines = append(lines, strings.Repeat(" ", w))
	}
	return strings.Join(lines[:h], "\n")
}

func (m *Model) emptyMessage() string {
	if len(m.viewList()) > 0 {
		return "  没有匹配的会话"
	}
	switch m.view {
	case viewTrash:
		return "  回收站是空的"
	case viewFavorites:
		return "  还没有收藏，按 f 收藏"
	}
	return "  还没有 " + m.agent.String() + " 会话"
}

func (m *Model) renderRight(w, h int) string {
	s := m.current()
	blank := strings.Repeat(" ", w)
	if s == nil {
		return strings.TrimRight(strings.Repeat(blank+"\n", h), "\n")
	}
	head := strong.Render(fitLine(m.shortDir(s), w))
	var meta, body string
	if p := m.cache[s.Key()]; p == nil {
		meta = fitLine("加载中…", w)
		body = strings.Repeat("\n", max(h-4, 0))
	} else {
		info := fmt.Sprintf("%s · %d 条消息 · %s", s.ID[:min(8, len(s.ID))], p.Total, when(s))
		if m.vp.TotalLineCount() > m.vp.Height() {
			info += fmt.Sprintf(" · %d%%", int(m.vp.ScrollPercent()*100))
		}
		meta = fitLine(info, w)
		lines := strings.Split(m.vp.View(), "\n")
		for i := range lines {
			lines[i] = fitLine(lines[i], w)
		}
		body = strings.Join(lines, "\n")
	}
	rule := dim.Render(strings.Repeat("─", w))
	out := strings.Split(strings.Join([]string{head, dim.Render(meta), rule, body}, "\n"), "\n")
	for len(out) < h {
		out = append(out, blank)
	}
	return strings.Join(out[:h], "\n")
}
