package tui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/iainen/agent-session-switch/internal/session"
)

// Init implements tea.Model.
func (m Model) Init() tea.Cmd { return nil }

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		m.input.SetWidth(max(m.w-6, 10))
		m.shownKey = ""
		m.syncViewport()
		cmd := m.scheduleLoad()
		return m, cmd

	case loadTickMsg:
		s := m.current()
		if s == nil || s.Key() != msg.key || msg.seq != m.seq || m.cache[msg.key] != nil || m.loading[msg.key] {
			return m, nil
		}
		m.loading[msg.key] = true
		return m, loadCmd(s)

	case previewMsg:
		m.cache[msg.key] = msg.p
		delete(m.loading, msg.key)
		m.syncViewport()
		return m, nil

	case tea.MouseWheelMsg:
		switch msg.Button {
		case tea.MouseWheelUp:
			m.vp.ScrollUp(3)
		case tea.MouseWheelDown:
			m.vp.ScrollDown(3)
		}
		return m, nil

	case tea.KeyPressMsg:
		if cmd, handled := m.handleKey(msg.String()); handled {
			return m, cmd
		}
	}

	// Everything else (typed characters, cursor blink, ...) goes to the filter box.
	before := m.input.Value()
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	if m.input.Value() != before {
		m.refilter()
		m.status = ""
		m.syncViewport()
		return m, tea.Batch(cmd, m.scheduleLoad())
	}
	return m, cmd
}

// handleKey processes a key press. handled is false only when the key
// should be forwarded to the filter box.
func (m *Model) handleKey(key string) (cmd tea.Cmd, handled bool) {
	if key == "ctrl+c" {
		return tea.Quit, true
	}
	if m.confirm != nil {
		return m.handleConfirm(key), true
	}
	if cmd, ok := m.handleCommonKey(key); ok {
		return cmd, true
	}
	if m.filtering {
		if key == "esc" { // back to navigation, keeping the typed filter
			m.filtering = false
			m.input.Blur()
			return nil, true
		}
		return nil, false
	}
	return m.handleNavKey(key), true // other keys are ignored while navigating
}

// handleConfirm answers a pending delete confirmation. Only "y" confirms;
// any other key cancels.
func (m *Model) handleConfirm(key string) tea.Cmd {
	target, purge := m.confirm, m.purge
	m.confirm, m.purge = nil, false
	if key != "y" && key != "Y" {
		m.status = "已取消"
		return nil
	}
	done := "已移到回收站: "
	var err error
	if purge {
		done, err = "已永久删除: ", m.doPurge(target)
	} else {
		err = m.doTrash(target)
	}
	if err != nil {
		m.status = "失败: " + err.Error()
		return nil
	}
	m.status = done + target.Title
	return m.scheduleLoad()
}

// handleCommonKey handles keys that work both while navigating and filtering.
func (m *Model) handleCommonKey(key string) (tea.Cmd, bool) {
	switch key {
	case "enter":
		return m.openCurrent(), true
	case "up", "ctrl+p":
		return m.move(-1), true
	case "down", "ctrl+n":
		return m.move(1), true
	case "pgup":
		return m.move(-10), true
	case "pgdown":
		return m.move(10), true
	case "ctrl+u":
		m.vp.HalfPageUp()
		return nil, true
	case "ctrl+d":
		m.vp.HalfPageDown()
		return nil, true
	}
	return nil, false
}

// handleNavKey handles the vim-style navigation keys.
func (m *Model) handleNavKey(key string) tea.Cmd {
	switch key {
	case "j":
		return m.move(1)
	case "k":
		return m.move(-1)
	case "g", "home":
		return m.move(-len(m.filtered))
	case "G", "end":
		return m.move(len(m.filtered))
	case "h", "left":
		return m.changeAgent(m.agent - 1)
	case "l", "right":
		return m.changeAgent(m.agent + 1)
	case "d":
		if s := m.current(); s != nil && m.view != viewTrash {
			m.confirm, m.purge, m.status = s, false, ""
		}
	case "x":
		if s := m.current(); s != nil && m.view == viewTrash {
			m.confirm, m.purge, m.status = s, true, ""
		}
	case "r":
		if m.view == viewTrash {
			return m.restoreCurrent()
		}
	case "f":
		return m.favoriteCurrent()
	case "F":
		m.toggleView(viewFavorites)
		return m.viewChanged()
	case "t":
		m.toggleView(viewTrash)
		return m.viewChanged()
	case "tab": // sessions -> favorites -> trash -> sessions
		m.setView((m.view + 1) % numViews)
		return m.viewChanged()
	case "/":
		m.filtering = true
		return m.input.Focus()
	case "q", "esc":
		if m.view != viewSessions { // in favorites / trash these mean "go back"
			m.setView(viewSessions)
			return m.viewChanged()
		}
		return tea.Quit
	}
	return nil
}

func (m *Model) viewChanged() tea.Cmd {
	m.syncViewport()
	return m.scheduleLoad()
}

func (m *Model) changeAgent(a session.Agent) tea.Cmd {
	m.switchAgent(a)
	return m.viewChanged()
}

// openCurrent opens the selected session, or restores it in the trash view.
func (m *Model) openCurrent() tea.Cmd {
	s := m.current()
	if s == nil {
		return nil
	}
	if m.view == viewTrash {
		return m.restoreCurrent()
	}
	if s.Missing {
		m.status = "原目录已不存在: " + s.Cwd
		return nil
	}
	m.chosen = s
	return tea.Quit
}

func (m *Model) restoreCurrent() tea.Cmd {
	s := m.current()
	if s == nil {
		return nil
	}
	if err := m.doRestore(s); err != nil {
		m.status = "还原失败: " + err.Error()
		return nil
	}
	m.status = "已还原: " + s.Title
	return m.viewChanged()
}

func (m *Model) favoriteCurrent() tea.Cmd {
	s := m.current()
	if s == nil || m.view == viewTrash {
		return nil
	}
	added, err := m.toggleFav(s)
	switch {
	case err != nil:
		m.status = "收藏失败: " + err.Error()
	case added:
		m.status = "已收藏: " + s.Title
	default:
		m.status = "已取消收藏: " + s.Title
	}
	return m.viewChanged()
}
