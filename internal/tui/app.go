package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	scfg "github.com/sacckth/sshui/internal/config"
	"github.com/sacckth/sshui/internal/sshkeywords"
)

// Options carries theme and editor preferences (from ~/.config/sshui/config.toml).
type Options struct {
	Theme  string
	Editor string
}

type viewMode int

const (
	modeTree viewMode = iota
	modeDetail
	modePicker
	modeInputDirectiveValue
	modeInputCustomKey
	modeInputNewHost
	modeInputDuplicateHost
	modeHelp
	modeConfirmDeleteHost
)

// Model is the root Bubble Tea model for sshui.
type Model struct {
	cfg    *scfg.Config
	path   string
	dirty  bool
	width  int
	height int
	mode   viewMode

	hostList   list.Model
	detailList list.Model
	pickerList list.Model

	valueInput textinput.Model
	keyInput   textinput.Model

	selRef               scfg.HostRef
	pendingDirectiveKey  string
	editDirectiveIndex   int // >=0 when editing value; -1 when adding
	status               string
	editor               string // from app config; VISUAL/EDITOR used when empty
}

var (
	titleStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
	statusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	errStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	helpStyle   = lipgloss.NewStyle().Padding(1, 2)
)

// New builds a TUI model for the given config path and parsed config.
func New(cfg *scfg.Config, path string, opts Options) *Model {
	applyTheme(opts.Theme)
	w, h := 80, 24
	hostItems := buildHostItems(cfg)
	delegate := list.NewDefaultDelegate()
	delegate.ShowDescription = true
	l := list.New(hostItems, delegate, w, h-3)
	l.Title = "SSH hosts"
	l.Styles.Title = titleStyle
	l.SetShowStatusBar(true)
	l.SetFilteringEnabled(true)
	l.DisableQuitKeybindings()

	ti := textinput.New()
	ti.CharLimit = 2048
	ti.Width = 60

	ki := textinput.New()
	ki.CharLimit = 128
	ki.Width = 40
	ki.Placeholder = "DirectiveKey (custom / future keywords)"

	return &Model{
		cfg:                cfg,
		path:               path,
		width:              w,
		height:             h,
		hostList:           l,
		valueInput:         ti,
		keyInput:           ki,
		editDirectiveIndex: -1,
		editor:             opts.Editor,
	}
}

func (m *Model) Init() tea.Cmd {
	return nil
}

// InitProgram returns a prepared tea.Program.
func InitProgram(cfg *scfg.Config, path string, opts Options) *tea.Program {
	return tea.NewProgram(New(cfg, path, opts), tea.WithAltScreen())
}

type hostEntry struct {
	title string
	desc  string
	ref   scfg.HostRef
}

func (e hostEntry) Title() string       { return e.title }
func (e hostEntry) Description() string { return e.desc }
func (e hostEntry) FilterValue() string { return e.title + " " + e.desc }

type dirEntry struct {
	idx int
	d   scfg.Directive
}

func (e dirEntry) Title() string       { return e.d.Key }
func (e dirEntry) Description() string { return e.d.Value }
func (e dirEntry) FilterValue() string { return e.d.Key + " " + e.d.Value }

type kwEntry sshkeywords.Entry

func (e kwEntry) Title() string       { return e.Name }
func (e kwEntry) Description() string { return e.Hint }
func (e kwEntry) FilterValue() string { return e.Name + " " + e.Hint }

func buildHostItems(cfg *scfg.Config) []list.Item {
	var items []list.Item
	for i := range cfg.DefaultHosts {
		h := &cfg.DefaultHosts[i]
		items = append(items, hostEntry{
			title: hostTitle(h),
			desc:  "group: (default)",
			ref:   scfg.HostRef{InDefault: true, HostIdx: i},
		})
	}
	for gi := range cfg.Groups {
		g := &cfg.Groups[gi]
		for hi := range g.Hosts {
			h := &g.Hosts[hi]
			items = append(items, hostEntry{
				title: hostTitle(h),
				desc:  "group: " + g.Name,
				ref:   scfg.HostRef{InDefault: false, GroupIdx: gi, HostIdx: hi},
			})
		}
	}
	return items
}

func hostTitle(h *scfg.HostBlock) string {
	if len(h.Patterns) == 0 {
		return "(empty Host)"
	}
	return strings.Join(h.Patterns, " ")
}

func (m *Model) rebuildHostList() {
	items := buildHostItems(m.cfg)
	m.hostList.SetItems(items)
}

func (m *Model) openDetail(ref scfg.HostRef) {
	m.selRef = ref
	m.detailList = m.newDetailList()
	m.mode = modeDetail
}

func (m *Model) newDetailList() list.Model {
	h := m.cfg.HostAt(m.selRef)
	items := make([]list.Item, len(h.Directives))
	for i := range h.Directives {
		items[i] = dirEntry{idx: i, d: h.Directives[i]}
	}
	delegate := list.NewDefaultDelegate()
	delegate.ShowDescription = true
	l := list.New(items, delegate, m.width, m.height-3)
	l.Title = "Directives — " + hostTitle(h)
	l.Styles.Title = titleStyle
	l.SetShowStatusBar(true)
	l.SetFilteringEnabled(true)
	l.DisableQuitKeybindings()
	return l
}

func (m *Model) refreshDetailList() {
	m.detailList = m.newDetailList()
}

func (m *Model) openPicker() {
	m.editDirectiveIndex = -1
	entries := sshkeywords.Catalog
	items := make([]list.Item, len(entries))
	for i := range entries {
		items[i] = kwEntry(entries[i])
	}
	delegate := list.NewDefaultDelegate()
	delegate.ShowDescription = true
	l := list.New(items, delegate, m.width, m.height-3)
	l.Title = "Add directive (type to filter)"
	l.Styles.Title = titleStyle
	l.SetShowStatusBar(true)
	l.SetFilteringEnabled(true)
	l.DisableQuitKeybindings()
	m.pickerList = l
	m.mode = modePicker
}

// hiddenBackupPath returns a dot-prefixed backup path beside the config file
// (e.g. ~/.ssh/config → ~/.ssh/.config.bkp).
func hiddenBackupPath(configPath string) string {
	dir := filepath.Dir(configPath)
	base := filepath.Base(configPath)
	return filepath.Join(dir, "."+base+".bkp")
}

func (m *Model) save() error {
	prev, err := os.ReadFile(m.path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read existing file for backup: %w", err)
	}
	if err == nil {
		bkp := hiddenBackupPath(m.path)
		if werr := os.WriteFile(bkp, prev, 0o600); werr != nil {
			return fmt.Errorf("write backup %s: %w", bkp, werr)
		}
	}

	f, err := os.OpenFile(m.path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := scfg.Write(f, m.cfg); err != nil {
		return err
	}
	m.dirty = false
	return nil
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.hostList.SetWidth(msg.Width)
		m.hostList.SetHeight(max(6, msg.Height-3))
		if m.mode == modeDetail {
			m.detailList.SetWidth(msg.Width)
			m.detailList.SetHeight(max(6, msg.Height-3))
		}
		if m.mode == modePicker {
			m.pickerList.SetWidth(msg.Width)
			m.pickerList.SetHeight(max(6, msg.Height-3))
		}
		return m, nil

	case rawEditorFinishedMsg:
		return m.handleRawEditorFinished(msg)

	case rawEditorErrMsg:
		m.status = errStyle.Render(msg.err.Error())
		return m, nil

	case tea.KeyMsg:
		switch m.mode {
		case modeHelp:
			m.mode = modeTree
			return m, nil

		case modeConfirmDeleteHost:
			switch msg.String() {
			case "y", "Y":
				m.cfg.DeleteHost(m.selRef)
				m.dirty = true
				m.rebuildHostList()
				m.mode = modeTree
				m.status = "Host deleted."
			case "n", "N", "esc":
				m.mode = modeDetail
				m.refreshDetailList()
			}
			return m, nil

		case modeInputDirectiveValue, modeInputCustomKey, modeInputNewHost, modeInputDuplicateHost:
			switch msg.String() {
			case "esc":
				m.editDirectiveIndex = -1
				m.pendingDirectiveKey = ""
				m.mode = modeDetail
				m.refreshDetailList()
				return m, nil
			case "enter":
				return m.submitInput()
			}
			var cmd tea.Cmd
			if m.mode == modeInputCustomKey {
				m.keyInput, cmd = m.keyInput.Update(msg)
			} else {
				m.valueInput, cmd = m.valueInput.Update(msg)
			}
			return m, cmd
		}

		if m.mode == modeTree {
			return m.updateTree(msg)
		}
		if m.mode == modeDetail {
			return m.updateDetail(msg)
		}
		if m.mode == modePicker {
			return m.updatePicker(msg)
		}
	}

	var cmd tea.Cmd
	m.hostList, cmd = m.hostList.Update(msg)
	return m, cmd
}

func (m *Model) submitInput() (tea.Model, tea.Cmd) {
	switch m.mode {
	case modeInputDirectiveValue:
		val := strings.TrimSpace(m.valueInput.Value())
		h := m.cfg.HostAt(m.selRef)
		if m.editDirectiveIndex >= 0 && m.editDirectiveIndex < len(h.Directives) {
			h.Directives[m.editDirectiveIndex].Value = val
			m.status = "Directive updated."
		} else {
			h.Directives = append(h.Directives, scfg.Directive{Key: m.pendingDirectiveKey, Value: val})
			m.status = "Directive added."
		}
		m.dirty = true
		m.pendingDirectiveKey = ""
		m.editDirectiveIndex = -1
		m.mode = modeDetail
		m.refreshDetailList()

	case modeInputCustomKey:
		k := strings.TrimSpace(m.keyInput.Value())
		if k == "" {
			m.status = errStyle.Render("Key required.")
			return m, nil
		}
		m.pendingDirectiveKey = k
		m.keyInput.SetValue("")
		m.mode = modeInputDirectiveValue
		m.valueInput.SetValue("")
		m.valueInput.Placeholder = "value (optional)"
		m.valueInput.Focus()
		return m, textinput.Blink

	case modeInputNewHost:
		pat := strings.Fields(m.valueInput.Value())
		if len(pat) == 0 {
			m.status = errStyle.Render("Enter at least one Host pattern.")
			return m, nil
		}
		m.cfg.DefaultHosts = append(m.cfg.DefaultHosts, scfg.HostBlock{Patterns: pat})
		m.dirty = true
		m.rebuildHostList()
		m.selRef = scfg.HostRef{InDefault: true, HostIdx: len(m.cfg.DefaultHosts) - 1}
		m.mode = modeDetail
		m.refreshDetailList()
		m.status = "New host created."

	case modeInputDuplicateHost:
		pat := strings.Fields(m.valueInput.Value())
		if len(pat) == 0 {
			m.status = errStyle.Render("Enter at least one Host pattern.")
			return m, nil
		}
		if err := m.cfg.DuplicateHost(m.selRef, pat); err != nil {
			m.status = errStyle.Render(err.Error())
			return m, nil
		}
		m.dirty = true
		m.rebuildHostList()
		// Move selection to duplicated block (inserted after current)
		ref := m.selRef
		ref.HostIdx++
		if err := m.cfg.ValidateRef(ref); err == nil {
			m.selRef = ref
		}
		m.mode = modeDetail
		m.refreshDetailList()
		m.status = "Host duplicated."
	}
	return m, nil
}

func (m *Model) updateTree(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, key.NewBinding(key.WithKeys("ctrl+c", "q"))):
		return m, tea.Quit

	case key.Matches(msg, key.NewBinding(key.WithKeys("Q"))):
		return m, tea.Quit

	case msg.String() == "?":
		m.mode = modeHelp
		return m, nil

	case msg.String() == "s":
		if err := m.save(); err != nil {
			m.status = errStyle.Render("Save: " + err.Error())
		} else {
			m.status = "Saved."
		}
		return m, nil

	case msg.String() == "r":
		data, err := os.ReadFile(m.path)
		if err != nil && !os.IsNotExist(err) {
			m.status = errStyle.Render("Reload: " + err.Error())
			return m, nil
		}
		var cfg *scfg.Config
		if len(data) == 0 {
			cfg = &scfg.Config{}
		} else {
			cfg, err = scfg.Parse(strings.NewReader(string(data)))
			if err != nil {
				m.status = errStyle.Render("Reload parse: " + err.Error())
				return m, nil
			}
		}
		m.cfg = cfg
		m.dirty = false
		m.rebuildHostList()
		m.status = "Reloaded from disk."
		return m, nil

	case msg.String() == "n":
		m.mode = modeInputNewHost
		m.valueInput.SetValue("")
		m.valueInput.Placeholder = "Host patterns (space-separated)"
		m.valueInput.Focus()
		return m, textinput.Blink

	case msg.String() == "v":
		return m, m.rawEditorCmd()

	case msg.String() == "enter":
		if it, ok := m.hostList.SelectedItem().(hostEntry); ok {
			m.openDetail(it.ref)
			m.detailList.SetWidth(m.width)
			m.detailList.SetHeight(max(6, m.height-3))
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.hostList, cmd = m.hostList.Update(msg)
	return m, cmd
}

func (m *Model) updateDetail(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, key.NewBinding(key.WithKeys("esc"))):
		m.mode = modeTree
		return m, nil

	case msg.String() == "s":
		if err := m.save(); err != nil {
			m.status = errStyle.Render("Save: " + err.Error())
		} else {
			m.status = "Saved."
		}
		return m, nil

	case msg.String() == "a":
		m.openPicker()
		m.pickerList.SetWidth(m.width)
		m.pickerList.SetHeight(max(6, m.height-3))
		return m, nil

	case msg.String() == "k":
		m.editDirectiveIndex = -1
		m.mode = modeInputCustomKey
		m.keyInput.SetValue("")
		m.keyInput.Focus()
		return m, textinput.Blink

	case msg.String() == "e":
		if it, ok := m.detailList.SelectedItem().(dirEntry); ok {
			h := m.cfg.HostAt(m.selRef)
			m.valueInput.SetValue(h.Directives[it.idx].Value)
			m.valueInput.Placeholder = "value"
			m.pendingDirectiveKey = h.Directives[it.idx].Key
			m.mode = modeInputDirectiveValue
			m.editDirectiveIndex = it.idx
			m.valueInput.Focus()
			return m, textinput.Blink
		}

	case msg.String() == "d":
		if it, ok := m.detailList.SelectedItem().(dirEntry); ok {
			h := m.cfg.HostAt(m.selRef)
			h.Directives = append(h.Directives[:it.idx], h.Directives[it.idx+1:]...)
			m.dirty = true
			m.refreshDetailList()
			m.status = "Directive removed."
		}
		return m, nil

	case msg.String() == "D":
		m.mode = modeInputDuplicateHost
		m.valueInput.SetValue(hostTitle(m.cfg.HostAt(m.selRef)) + "-copy")
		m.valueInput.Placeholder = "new Host patterns"
		m.valueInput.Focus()
		return m, textinput.Blink

	case msg.String() == "X":
		m.mode = modeConfirmDeleteHost
		m.status = fmt.Sprintf("Delete host %q? [y/N]", hostTitle(m.cfg.HostAt(m.selRef)))
		return m, nil

	case msg.String() == "v":
		return m, m.rawEditorCmd()
	}

	var cmd tea.Cmd
	m.detailList, cmd = m.detailList.Update(msg)
	return m, cmd
}

func (m *Model) updatePicker(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, key.NewBinding(key.WithKeys("esc"))):
		m.mode = modeDetail
		return m, nil

	case msg.String() == "enter":
		if it, ok := m.pickerList.SelectedItem().(kwEntry); ok {
			m.editDirectiveIndex = -1
			m.pendingDirectiveKey = it.Name
			m.mode = modeInputDirectiveValue
			m.valueInput.SetValue("")
			h := ""
			if it.Hint != "" {
				h = it.Hint
			}
			m.valueInput.Placeholder = h
			m.valueInput.Focus()
			return m, textinput.Blink
		}
	}

	var cmd tea.Cmd
	m.pickerList, cmd = m.pickerList.Update(msg)
	return m, cmd
}

func (m Model) View() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("sshui — "))
	b.WriteString(m.path)
	if m.dirty {
		b.WriteString(errStyle.Render(" *"))
	}
	b.WriteByte('\n')
	if m.cfg.HasInclude {
		b.WriteString(errStyle.Render("Warning: file contains Include; only this file is edited. "))
		b.WriteByte('\n')
	}
	switch m.mode {
	case modeHelp:
		b.WriteString(helpStyle.Render(helpText))
		return b.String()

	case modeConfirmDeleteHost:
		b.WriteString(statusStyle.Render(m.status))
		b.WriteByte('\n')
		return b.String()

	case modeInputDirectiveValue, modeInputNewHost, modeInputDuplicateHost:
		b.WriteString(statusStyle.Render("Enter value, Esc cancel"))
		b.WriteByte('\n')
		if m.mode == modeInputDirectiveValue && m.pendingDirectiveKey != "" {
			b.WriteString(fmt.Sprintf("Directive: %s\n", m.pendingDirectiveKey))
		}
		b.WriteString(m.valueInput.View())
		b.WriteByte('\n')
		if m.status != "" {
			b.WriteString(statusStyle.Render(m.status))
		}
		return b.String()

	case modeInputCustomKey:
		b.WriteString(statusStyle.Render("Custom directive key, Enter to continue"))
		b.WriteByte('\n')
		b.WriteString(m.keyInput.View())
		b.WriteByte('\n')
		return b.String()

	case modeDetail:
		v := m.detailList.View()
		b.WriteString(v)
		b.WriteByte('\n')
		b.WriteString(statusStyle.Render("enter: — | a add | k custom key | e edit value | d del | D dup host | X del host | v raw $EDITOR | s save | esc back"))
		if m.status != "" {
			b.WriteByte('\n')
			b.WriteString(statusStyle.Render(m.status))
		}
		return b.String()

	case modePicker:
		b.WriteString(m.pickerList.View())
		b.WriteByte('\n')
		b.WriteString(statusStyle.Render("enter pick | esc cancel"))
		return b.String()

	default: // tree
		b.WriteString(m.hostList.View())
		b.WriteByte('\n')
		b.WriteString(statusStyle.Render("enter: open | n new host | v raw $EDITOR | s save | r reload | ? help | q quit"))
		if m.status != "" {
			b.WriteByte('\n')
			b.WriteString(statusStyle.Render(m.status))
		}
		return b.String()
	}
}

const helpText = `
sshui — SSH client config TUI

Tree
  enter     Open host directives
  /         Filter hosts
  n         New host (default section)
  v         Edit serialized config in $EDITOR / VISUAL / EDITOR (see ~/.config/sshui/config.toml)
  s         Save to file
  r         Reload from disk (discards unsaved edits)
  ?         This help
  q / Q     Quit

Host detail
  a         Add directive (catalog picker)
  k         Add directive with custom key
  e         Edit selected directive value
  d         Delete selected directive
  D         Duplicate host (new Host patterns)
  X         Delete entire host (confirm)
  v         Raw editor on in-memory buffer (same as tree)
  s         Save
  esc       Back to tree

Optional: ~/.config/sshui/config.toml — ssh_config, editor, theme (default|warm|muted).

Each save writes a hidden backup of the prior on-disk file next to it (e.g. .ssh/.config.bkp for .ssh/config).

Saving still rewrites the main file with stable formatting.
`
