package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/x/ansi"
)

func wrapHelpLines(text string, lineWidth int) string {
	if lineWidth < 24 {
		lineWidth = 24
	}
	var b strings.Builder
	for _, line := range strings.Split(text, "\n") {
		if strings.TrimSpace(line) == "" {
			b.WriteByte('\n')
			continue
		}
		b.WriteString(ansi.Wordwrap(line, lineWidth, " "))
		b.WriteByte('\n')
	}
	return b.String()
}

func (m *Model) layoutHelpViewport() {
	vw := max(40, m.width-6)
	vh := max(8, m.height-10)
	raw := strings.TrimPrefix(helpText, "\n")
	content := wrapHelpLines(raw, vw)
	if m.helpViewport.Width == 0 && m.helpViewport.Height == 0 {
		m.helpViewport = viewport.New(vw, vh)
	}
	m.helpViewport.Width = vw
	m.helpViewport.Height = vh
	m.helpViewport.SetContent(content)
}

func (m *Model) openHelp(returnTo viewMode) {
	m.helpReturnMode = returnTo
	m.layoutHelpViewport()
	m.mode = modeHelp
}
