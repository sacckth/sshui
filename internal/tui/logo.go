package tui

import "github.com/charmbracelet/lipgloss"

// titleBarMinWidthForLogo is the minimum terminal width before we draw the braille logo
// beside the title (avoids wrapping on narrow terminals).
const titleBarMinWidthForLogo = 72

// titleBarLogoBlock returns a small 3×2-cell braille motif (styled like the title).
func titleBarLogoBlock() string {
	lines := []string{
		"⢎⣱",
		"⢸⣿",
		"⠈⠹",
	}
	raw := lipgloss.JoinVertical(lipgloss.Left, lines...)
	return titleStyle.Copy().Render(raw)
}

// titleBarLine renders the top title row: optional braille + "sshui — " + path (path unstyled).
func (m *Model) titleBarLine() string {
	rest := titleStyle.Render("sshui — ") + m.path
	if m.width < titleBarMinWidthForLogo {
		return rest
	}
	logo := titleBarLogoBlock()
	pad := "  "
	joined := lipgloss.JoinHorizontal(lipgloss.Top, logo, pad, rest)
	if lipgloss.Width(joined) > m.width {
		return rest
	}
	return joined
}
