package tui

import (
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

// patchFilterAcceptKeys stops bubbles from treating ↑/↓ (and ctrl+j) as "apply filter".
// We handle list cursor movement in tryFilterListArrowNav while the filter prompt is open.
func patchFilterAcceptKeys(l *list.Model) {
	if l == nil {
		return
	}
	km := l.KeyMap
	km.AcceptWhileFiltering = key.NewBinding(
		key.WithKeys("enter", "tab", "shift+tab", "ctrl+k"),
		key.WithHelp("enter", "apply filter"),
	)
	l.KeyMap = km
}

// tryFilterListArrowNav moves the list cursor while the filter prompt is open.
func tryFilterListArrowNav(l *list.Model, msg tea.Msg) bool {
	if l == nil || !l.SettingFilter() {
		return false
	}
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		return false
	}
	switch km.Type {
	case tea.KeyUp:
		l.CursorUp()
		return true
	case tea.KeyDown:
		l.CursorDown()
		return true
	case tea.KeyRunes:
		if len(km.Runes) == 1 {
			switch km.Runes[0] {
			case 'k', 'K':
				l.CursorUp()
				return true
			case 'j', 'J':
				l.CursorDown()
				return true
			}
		}
	}
	return false
}

// isEnterKey is true for the main Enter/Return key (not Ctrl+Enter, etc.).
func isEnterKey(msg tea.Msg) bool {
	km, ok := msg.(tea.KeyMsg)
	return ok && km.Type == tea.KeyEnter && !km.Alt
}
