package tui

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	scfg "github.com/sacckth/sshui/internal/config"
)

// rawEditorFinishedMsg is sent after $EDITOR (or configured editor) exits.
type rawEditorFinishedMsg struct {
	path string
	err  error
}

// rawEditorErrMsg reports failures before the editor could start.
type rawEditorErrMsg struct {
	err error
}

func (m *Model) rawEditorCmd() tea.Cmd {
	s, err := scfg.String(m.cfg)
	if err != nil {
		return func() tea.Msg {
			return rawEditorErrMsg{err: fmt.Errorf("serialize config: %w", err)}
		}
	}
	f, err := os.CreateTemp("", "sshui-raw-*.conf")
	if err != nil {
		return func() tea.Msg {
			return rawEditorErrMsg{err: fmt.Errorf("temp file: %w", err)}
		}
	}
	path := f.Name()
	if _, werr := f.WriteString(s); werr != nil {
		_ = f.Close()
		_ = os.Remove(path)
		return func() tea.Msg {
			return rawEditorErrMsg{err: werr}
		}
	}
	if cerr := f.Close(); cerr != nil {
		_ = os.Remove(path)
		return func() tea.Msg {
			return rawEditorErrMsg{err: cerr}
		}
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

	// shell -c 'editor /path' so values like `code --wait` work
	quoted := strconv.Quote(path)
	c := exec.Command("sh", "-c", ed+" "+quoted)
	return tea.ExecProcess(c, func(runErr error) tea.Msg {
		return rawEditorFinishedMsg{path: path, err: runErr}
	})
}

func (m *Model) handleRawEditorFinished(msg rawEditorFinishedMsg) (tea.Model, tea.Cmd) {
	defer func() { _ = os.Remove(msg.path) }()

	data, rerr := os.ReadFile(msg.path)
	if rerr != nil {
		m.status = errStyle.Render(fmt.Sprintf("read temp buffer: %v", rerr))
		if msg.err != nil {
			m.status += "\n" + errStyle.Render(msg.err.Error())
		}
		return m, nil
	}

	cfg, perr := scfg.Parse(strings.NewReader(string(data)))
	if perr != nil {
		m.status = errStyle.Render("Config invalid after editor: " + perr.Error())
		return m, nil
	}

	if cfg.HasInclude {
		m.cfg = scfg.MergeIncludes(m.path, cfg)
		m.readOnly = true
	} else {
		m.cfg = cfg
		m.readOnly = false
	}
	m.dirty = true
	rb := m.rebuildHostList()
	if m.mode == modeDetail {
		if m.cfg.ValidateRef(m.selRef) != nil {
			m.mode = modeTree
			m.layoutDetailPanes()
		} else {
			m.layoutDetailPanes()
			m.refreshDetailList()
		}
	} else if m.mode == modeTree {
		m.layoutDetailPanes()
		m.refreshDetailList()
	}
	m.status = "Replaced in-memory config from editor buffer (save with s)."
	if msg.err != nil {
		m.status += " " + statusStyle.Render("(editor process: "+msg.err.Error()+")")
	}
	return m, rb
}
