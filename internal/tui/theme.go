package tui

import (
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// filterMatchStyle highlights fuzzy-filter matches in list delegates (not underline).
var filterMatchStyle lipgloss.Style

// readOnlyBannerStyle is used for merged Include read-only mode messaging (green).
var readOnlyBannerStyle lipgloss.Style

// writableWarnStyle is used for writable main-file-only Include messaging (yellow warning).
var writableWarnStyle lipgloss.Style

// applyTheme sets global lipgloss styles used by lists and chrome.
func applyTheme(name string) {
	noColor := os.Getenv("NO_COLOR") != ""
	if noColor {
		lipgloss.SetColorProfile(termenv.Ascii)
	}
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "warm":
		titleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("208"))
		statusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
		errStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
		helpStyle = lipgloss.NewStyle().Padding(1, 2).Foreground(lipgloss.Color("223"))
		readOnlyBannerStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("150"))
		writableWarnStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("222"))
	case "muted":
		titleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("245"))
		statusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
		errStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("131"))
		helpStyle = lipgloss.NewStyle().Padding(1, 2).Foreground(lipgloss.Color("243"))
		readOnlyBannerStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("108"))
		writableWarnStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("185"))
	default:
		titleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
		statusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
		errStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
		helpStyle = lipgloss.NewStyle().Padding(1, 2)
		readOnlyBannerStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
		writableWarnStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("220"))
	}
	if noColor {
		filterMatchStyle = lipgloss.NewStyle().Reverse(true)
		readOnlyBannerStyle = lipgloss.NewStyle().Bold(true)
		writableWarnStyle = lipgloss.NewStyle().Bold(true).Underline(true)
	} else {
		filterMatchStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("99")).
			Foreground(lipgloss.Color("230"))
	}
}
