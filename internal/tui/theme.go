package tui

import (
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// applyTheme sets global lipgloss styles used by lists and chrome.
func applyTheme(name string) {
	if os.Getenv("NO_COLOR") != "" {
		lipgloss.SetColorProfile(termenv.Ascii)
	}
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "warm":
		titleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("208"))
		statusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
		errStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
		helpStyle = lipgloss.NewStyle().Padding(1, 2).Foreground(lipgloss.Color("223"))
	case "muted":
		titleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("245"))
		statusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
		errStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("131"))
		helpStyle = lipgloss.NewStyle().Padding(1, 2).Foreground(lipgloss.Color("243"))
	default:
		titleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
		statusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
		errStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
		helpStyle = lipgloss.NewStyle().Padding(1, 2)
	}
}
