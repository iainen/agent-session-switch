package tui

import (
	"errors"
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/iainen/agent-session-switch/internal/session"
)

// Run shows the picker and returns the session the user chose to open. It
// returns nil, nil when the user quits without choosing.
func Run(opts Options) (*session.Session, error) {
	if opts.Trash == nil || opts.Favs == nil {
		return nil, errors.New("tui: Options.Trash and Options.Favs are required")
	}
	final, err := tea.NewProgram(New(opts)).Run()
	if err != nil {
		return nil, fmt.Errorf("界面启动失败（需要在交互终端中运行）: %w", err)
	}
	return final.(Model).Chosen(), nil
}
