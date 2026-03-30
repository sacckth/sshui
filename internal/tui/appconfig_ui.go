package tui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/sacckth/sshui/internal/appcfg"
)

const appConfigTemplate = `# sshui application settings (not OpenSSH client config)
# ssh_config = "~/.ssh/config"
# editor = ""
# theme = "default"
# ssh_config_git_mirror = ""
`

// appConfigEditorFinishedMsg is sent after $EDITOR exits on sshui config.toml.
type appConfigEditorFinishedMsg struct {
	err error
}

// appConfigEditorErrMsg reports failures before the editor could start.
type appConfigEditorErrMsg struct {
	err error
}

func mirrorPathFromAppCfg(ac *appcfg.Config) (string, error) {
	if strings.TrimSpace(ac.SSHConfigGitMirror) == "" {
		return "", nil
	}
	return appcfg.ExpandPath(ac.SSHConfigGitMirror)
}

func (m *Model) openAppConfigEditor() tea.Cmd {
	if m.appConfigPath == "" {
		return func() tea.Msg {
			return appConfigEditorErrMsg{err: fmt.Errorf("app config path not set")}
		}
	}
	if err := os.MkdirAll(filepath.Dir(m.appConfigPath), 0o755); err != nil {
		return func() tea.Msg { return appConfigEditorErrMsg{err: err} }
	}
	if _, err := os.Stat(m.appConfigPath); os.IsNotExist(err) {
		if werr := os.WriteFile(m.appConfigPath, []byte(appConfigTemplate), 0o600); werr != nil {
			return func() tea.Msg { return appConfigEditorErrMsg{err: werr} }
		}
	} else if err != nil {
		return func() tea.Msg { return appConfigEditorErrMsg{err: err} }
	}

	ed := strings.TrimSpace(m.editor)
	if ed == "" {
		ed = os.Getenv("VISUAL")
	}
	if ed == "" {
		ed = os.Getenv("EDITOR")
	}
	if ed == "" {
		ed = "vi"
	}
	quoted := strconv.Quote(m.appConfigPath)
	c := exec.Command("sh", "-c", ed+" "+quoted)
	return tea.ExecProcess(c, func(runErr error) tea.Msg {
		return appConfigEditorFinishedMsg{err: runErr}
	})
}

func (m *Model) handleAppConfigEditorFinished(msg appConfigEditorFinishedMsg) (tea.Model, tea.Cmd) {
	ac, err := appcfg.Load()
	if err != nil {
		m.status = errStyle.Render("Reload app settings: " + err.Error())
		return m, nil
	}
	applyTheme(ac.Theme)
	m.themeName = ac.Theme
	m.editor = strings.TrimSpace(ac.Editor)
	mp, merr := mirrorPathFromAppCfg(&ac)
	if merr != nil {
		m.status = errStyle.Render("ssh_config_git_mirror: " + merr.Error())
		return m, nil
	}
	m.mirrorPath = mp
	m.status = "Reloaded sshui settings from " + m.appConfigPath
	if msg.err != nil {
		m.status += " " + statusStyle.Render("(editor: "+msg.err.Error()+")")
	}
	return m, nil
}

func (m *Model) openAppConfigView(returnTo viewMode) (tea.Model, tea.Cmd) {
	if m.appConfigPath == "" {
		m.status = errStyle.Render("App config path not set.")
		return m, nil
	}
	m.appCfgReturnMode = returnTo
	if err := os.MkdirAll(filepath.Dir(m.appConfigPath), 0o755); err != nil {
		m.status = errStyle.Render(err.Error())
		return m, nil
	}
	data, err := os.ReadFile(m.appConfigPath)
	if err != nil && os.IsNotExist(err) {
		if werr := os.WriteFile(m.appConfigPath, []byte(appConfigTemplate), 0o600); werr != nil {
			m.status = errStyle.Render(werr.Error())
			return m, nil
		}
		data = []byte(appConfigTemplate)
	} else if err != nil {
		m.status = errStyle.Render("Read app config: " + err.Error())
		return m, nil
	}
	w := m.width - 4
	h := m.height - 6
	if w < 24 {
		w = 24
	}
	if h < 8 {
		h = 8
	}
	vp := viewport.New(w, h)
	vp.SetContent(string(data))
	m.appCfgViewport = vp
	m.mode = modeAppCfgView
	return m, nil
}

func (m *Model) layoutAppCfgViewport() {
	w := m.width - 4
	h := m.height - 6
	if w < 24 {
		w = 24
	}
	if h < 8 {
		h = 8
	}
	m.appCfgViewport.Width = w
	m.appCfgViewport.Height = h
}

func (m *Model) viewAppConfigScreen() string {
	title := titleStyle.Render("sshui app config (read-only) — " + m.appConfigPath)
	hint := statusStyle.Render("esc or q close | arrows / PgUp PgDn scroll")
	body := m.appCfgViewport.View()
	box := lipgloss.JoinVertical(lipgloss.Left, title, "", body, "", hint)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center,
		lipgloss.NewStyle().Border(panelBorder).Padding(1, 2).Render(box))
}
