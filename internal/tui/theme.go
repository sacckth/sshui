package tui

import (
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/list"
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

// paneFill is the unified split-pane background (left, right, separator). Empty when NO_COLOR.
var paneFill lipgloss.TerminalColor

// paneFillActive is true when split panes use a solid ANSI background.
var paneFillActive bool

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
		fill := lipgloss.Color("236")
		setPaneStyles(noColor, fill, lipgloss.Color("244"))
		setGroupHeaderStyles(noColor, fill, lipgloss.Color("214"), lipgloss.Color("223"))
	case "muted":
		titleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("245"))
		statusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
		errStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("131"))
		helpStyle = lipgloss.NewStyle().Padding(1, 2).Foreground(lipgloss.Color("243"))
		readOnlyBannerStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("108"))
		writableWarnStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("185"))
		fill := lipgloss.Color("237")
		setPaneStyles(noColor, fill, lipgloss.Color("242"))
		setGroupHeaderStyles(noColor, fill, lipgloss.Color("247"), lipgloss.Color("252"))
	default:
		titleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
		statusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
		errStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
		helpStyle = lipgloss.NewStyle().Padding(1, 2)
		readOnlyBannerStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
		writableWarnStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("220"))
		fill := lipgloss.Color("236")
		setPaneStyles(noColor, fill, lipgloss.Color("245"))
		setGroupHeaderStyles(noColor, fill, lipgloss.Color("81"), lipgloss.Color("159"))
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
			Background(paneFill).
			Foreground(lipgloss.Color("255"))
		listSelectedDescStyle = lipgloss.NewStyle().
			Bold(true).
			Background(paneFill).
			Foreground(lipgloss.Color("252"))
	}
	if paneFillActive {
		colHeaderStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("245")).Background(paneFill)
	} else {
		colHeaderStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("245"))
	}
}

func setPaneStyles(noColor bool, fill, sepFG lipgloss.TerminalColor) {
	if noColor {
		paneFillActive = false
		paneFill = lipgloss.NoColor{}
		paneLeftStyle = lipgloss.NewStyle()
		paneRightStyle = lipgloss.NewStyle()
		paneSepStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
		footerRuleStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
		return
	}
	paneFillActive = true
	paneFill = fill
	paneLeftStyle = lipgloss.NewStyle().Background(fill)
	paneRightStyle = lipgloss.NewStyle().Background(fill)
	paneSepStyle = lipgloss.NewStyle().Foreground(sepFG).Background(fill)
	footerRuleStyle = lipgloss.NewStyle().Foreground(sepFG)
}

func setGroupHeaderStyles(noColor bool, paneBG, fg, fgSelected lipgloss.TerminalColor) {
	if noColor {
		groupHeaderNormalStyle = lipgloss.NewStyle().Bold(true)
		groupHeaderSelectedStyle = lipgloss.NewStyle().Bold(true).Reverse(true)
		groupHeaderDimStyle = lipgloss.NewStyle()
		return
	}
	groupHeaderNormalStyle = lipgloss.NewStyle().
		Bold(true).
		Italic(true).
		Foreground(fg).
		Background(paneBG)
	groupHeaderSelectedStyle = lipgloss.NewStyle().
		Bold(true).
		Italic(true).
		Foreground(fgSelected).
		Background(paneBG)
	groupHeaderDimStyle = lipgloss.NewStyle().
		Bold(true).
		Italic(true).
		Foreground(lipgloss.Color("243")).
		Background(paneBG)
}

// styleWithPaneBG returns a copy of s with the unified pane background (no-op if NO_COLOR).
func styleWithPaneBG(s lipgloss.Style) lipgloss.Style {
	if !paneFillActive {
		return s
	}
	return s.Copy().Background(paneFill)
}

// panePadStyle paints trailing spaces used to extend each row to the pane width (same fill as panes).
func panePadStyle() lipgloss.Style {
	if !paneFillActive {
		return lipgloss.NewStyle()
	}
	return lipgloss.NewStyle().Background(paneFill)
}

// applyPaneChromeToList tints list chrome so title bar, status, pagination, and filter line match the pane fill.
func applyPaneChromeToList(l *list.Model) {
	if !paneFillActive {
		return
	}
	bg := paneFill
	st := l.Styles
	st.TitleBar = st.TitleBar.Copy().Background(bg)
	st.Title = st.Title.Copy().Background(bg)
	st.Spinner = st.Spinner.Copy().Background(bg)
	st.FilterPrompt = st.FilterPrompt.Copy().Background(bg)
	st.FilterCursor = st.FilterCursor.Copy().Background(bg)
	st.StatusBar = st.StatusBar.Copy().Background(bg)
	st.StatusEmpty = st.StatusEmpty.Copy().Background(bg)
	st.StatusBarActiveFilter = st.StatusBarActiveFilter.Copy().Background(bg)
	st.StatusBarFilterCount = st.StatusBarFilterCount.Copy().Background(bg)
	st.NoItems = st.NoItems.Copy().Background(bg)
	st.PaginationStyle = st.PaginationStyle.Copy().Background(bg)
	st.HelpStyle = st.HelpStyle.Copy().Background(bg)
	st.ArabicPagination = st.ArabicPagination.Copy().Background(bg)
	st.ActivePaginationDot = st.ActivePaginationDot.Copy().Background(bg)
	st.InactivePaginationDot = st.InactivePaginationDot.Copy().Background(bg)
	st.DividerDot = st.DividerDot.Copy().Background(bg)
	l.Styles = st
	patchHelpStylesForPane(&l.Help, bg)
}

// patchHelpStylesForPane gives help-bubble key/desc/separator segments the pane background so
// short help (e.g. "↑/k up") is not drawn on the terminal default color inside a tinted list.
func patchHelpStylesForPane(h *help.Model, bg lipgloss.TerminalColor) {
	if h == nil || !paneFillActive {
		return
	}
	st := h.Styles
	st.ShortKey = st.ShortKey.Copy().Background(bg)
	st.ShortDesc = st.ShortDesc.Copy().Background(bg)
	st.ShortSeparator = st.ShortSeparator.Copy().Background(bg)
	st.Ellipsis = st.Ellipsis.Copy().Background(bg)
	st.FullKey = st.FullKey.Copy().Background(bg)
	st.FullDesc = st.FullDesc.Copy().Background(bg)
	st.FullSeparator = st.FullSeparator.Copy().Background(bg)
	h.Styles = st
}
