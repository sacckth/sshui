package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// padVisualLines extends every line to targetWidth terminal cells by appending spaces
// with the pane background. List and lipgloss output often ends ANSI runs with \x1b[0m or
// \x1b[49m, so without this only glyphs carry the gray background (“vitiligo”).
//
// lipgloss.JoinVertical pads shorter lines with plain ASCII spaces (no ANSI background);
// bubbles' list View joins title, status, and items that way. We strip trailing spaces first
// so those gaps are replaced with pane-colored fill.
func padVisualLines(block string, targetWidth int) string {
	if targetWidth < 1 || !paneFillActive {
		return block
	}
	padStyle := panePadStyle()
	lines := strings.Split(block, "\n")
	for i, line := range lines {
		line = strings.TrimRight(line, " ")
		lines[i] = padLineToVisualWidth(line, targetWidth, padStyle)
	}
	return strings.Join(lines, "\n")
}

// ensureMinVisualLines appends full-width filler rows so short content still fills the pane
// height with the same background (plain \n vertical padding from lipgloss can leave default BG).
func ensureMinVisualLines(block string, targetWidth, minLines int) string {
	if minLines < 1 || targetWidth < 1 || !paneFillActive {
		return block
	}
	padLine := panePadStyle().Render(strings.Repeat(" ", targetWidth))
	lines := strings.Split(block, "\n")
	for len(lines) < minLines {
		lines = append(lines, padLine)
	}
	return strings.Join(lines, "\n")
}

func padLineToVisualWidth(line string, targetWidth int, spaceStyle lipgloss.Style) string {
	w := ansi.StringWidth(line)
	if w >= targetWidth {
		return line
	}
	n := targetWidth - w
	if n <= 0 {
		return line
	}
	return line + spaceStyle.Render(strings.Repeat(" ", n))
}
