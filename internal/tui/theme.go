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

// listSelectedTitleStyle / listSelectedDescStyle replace bubbles' bordered selected row.
var listSelectedTitleStyle lipgloss.Style
var listSelectedDescStyle lipgloss.Style

// groupHeaderNormalStyle / groupHeaderSelectedStyle / groupHeaderDimStyle — tree group rows
// (terminals cannot change font size; bold+italic approximate emphasis).
var groupHeaderNormalStyle lipgloss.Style
var groupHeaderSelectedStyle lipgloss.Style
var groupHeaderDimStyle lipgloss.Style

// Pane chrome (disabled when NO_COLOR).
var paneLeftStyle lipgloss.Style
var paneRightStyle lipgloss.Style
var paneSepStyle lipgloss.Style
var footerRuleStyle lipgloss.Style

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
		setPaneStyles(noColor, lipgloss.Color("236"), lipgloss.Color("233"), lipgloss.Color("244"))
		setGroupHeaderStyles(noColor, lipgloss.Color("214"), lipgloss.Color("223"))
	case "muted":
		titleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("245"))
		statusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
		errStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("131"))
		helpStyle = lipgloss.NewStyle().Padding(1, 2).Foreground(lipgloss.Color("243"))
		readOnlyBannerStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("108"))
		writableWarnStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("185"))
		setPaneStyles(noColor, lipgloss.Color("237"), lipgloss.Color("234"), lipgloss.Color("242"))
		setGroupHeaderStyles(noColor, lipgloss.Color("247"), lipgloss.Color("252"))
	default:
		titleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
		statusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
		errStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
		helpStyle = lipgloss.NewStyle().Padding(1, 2)
		readOnlyBannerStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
		writableWarnStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("220"))
		setPaneStyles(noColor, lipgloss.Color("236"), lipgloss.Color("234"), lipgloss.Color("245"))
		setGroupHeaderStyles(noColor, lipgloss.Color("81"), lipgloss.Color("159"))
	}
	if noColor {
		filterMatchStyle = lipgloss.NewStyle().Reverse(true)
		readOnlyBannerStyle = lipgloss.NewStyle().Bold(true)
		writableWarnStyle = lipgloss.NewStyle().Bold(true).Underline(true)
		listSelectedTitleStyle = lipgloss.NewStyle().Bold(true).Reverse(true)
		listSelectedDescStyle = lipgloss.NewStyle().Bold(true).Reverse(true)
	} else {
		filterMatchStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("99")).
			Foreground(lipgloss.Color("230"))
		listSelectedTitleStyle = lipgloss.NewStyle().
			Bold(true).
			Background(lipgloss.Color("238")).
			Foreground(lipgloss.Color("252"))
		listSelectedDescStyle = lipgloss.NewStyle().
			Bold(true).
			Background(lipgloss.Color("238")).
			Foreground(lipgloss.Color("246"))
	}
}

func setPaneStyles(noColor bool, leftBG, rightBG, sepFG lipgloss.TerminalColor) {
	if noColor {
		paneLeftStyle = lipgloss.NewStyle()
		paneRightStyle = lipgloss.NewStyle()
		paneSepStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
		footerRuleStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
		return
	}
	paneLeftStyle = lipgloss.NewStyle().Background(leftBG)
	paneRightStyle = lipgloss.NewStyle().Background(rightBG)
	// Separator column uses left pane background so the bar reads as a divide, not a third shade.
	paneSepStyle = lipgloss.NewStyle().Foreground(sepFG).Background(leftBG)
	footerRuleStyle = lipgloss.NewStyle().Foreground(sepFG)
}

func setGroupHeaderStyles(noColor bool, fg, fgSelected lipgloss.TerminalColor) {
	if noColor {
		groupHeaderNormalStyle = lipgloss.NewStyle().Bold(true)
		groupHeaderSelectedStyle = lipgloss.NewStyle().Bold(true).Reverse(true)
		groupHeaderDimStyle = lipgloss.NewStyle()
		return
	}
	groupHeaderNormalStyle = lipgloss.NewStyle().
		Bold(true).
		Italic(true).
		Foreground(fg)
	groupHeaderSelectedStyle = lipgloss.NewStyle().
		Bold(true).
		Italic(true).
		Foreground(fgSelected).
		Background(lipgloss.Color("237"))
	groupHeaderDimStyle = lipgloss.NewStyle().
		Bold(true).
		Italic(true).
		Foreground(lipgloss.Color("243"))
}
