package tui

import (
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

func (m *Model) startNewOpenSSHGroup() tea.Cmd {
	m.returnAfterInput = modeTree
	m.mode = modeInputNewGroup
	m.valueInput.SetValue("")
	m.valueInput.Placeholder = "new group name (not (default))"
	m.valueInput.Focus()
	return textinput.Blink
}

func (m *Model) startNewPasswordGroup() tea.Cmd {
	m.returnAfterInput = modeTree
	m.mode = modeInputNewPasswordGroup
	m.valueInput.SetValue("")
	m.valueInput.Placeholder = "new password group name"
	m.valueInput.Focus()
	return textinput.Blink
}
