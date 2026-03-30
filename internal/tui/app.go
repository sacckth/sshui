package tui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/atotto/clipboard"
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
	Theme      string
	Editor     string
	ReadOnly   bool   // merged Include view: no save or mutating edits
	MirrorPath string // optional: after save, copy bytes here (expanded abs path)
}

type viewMode int

const (
	modeTree viewMode = iota
	modeDetail
	modePicker
	modeGroupPicker
	modeInputDirectiveValue
	modeInputCustomKey
	modeInputNewHost
	modeInputDuplicateHost
	modeInputNewGroup
	modeInputRenameGroup
	modeInputGroupDesc
	modeHelp
	modeConfirmDeleteHost
	modeConfirmDeleteGroup
	modeActionMenu
	modeInputHostMeta
)

type detailTab int

const (
	detailTabOverview detailTab = iota
	detailTabAll
	detailTabConnectivity
)

// Model is the root Bubble Tea model for sshui.
type Model struct {
	cfg    *scfg.Config
	path   string
	dirty  bool
	width  int
	height int
	mode   viewMode

	hostList        list.Model
	detailList      list.Model
	pickerList      list.Model
	groupPickerList list.Model

	valueInput textinput.Model
	keyInput   textinput.Model

	selRef                 scfg.HostRef
	pendingDirectiveKey    string
	editDirectiveIndex     int // >=0 when editing value; -1 when adding
	status                 string
	editor                 string // from app config; VISUAL/EDITOR used when empty
	confirmReturnMode      viewMode
	returnAfterInput       viewMode
	groupPickerReturnMode  viewMode
	pendingDeleteGroupName string
	editGroupIdx           int

	readOnly         bool
	mirrorPath       string
	detailTab        detailTab
	treePaneFocused  bool
	actionMenuList   list.Model
	actionReturnMode viewMode
}

var (
	titleStyle     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
	statusStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	errStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	helpStyle      = lipgloss.NewStyle().Padding(1, 2)
	colHeaderStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("245"))
	panelBorder    = lipgloss.RoundedBorder()
)

type actionItem struct {
	id   string
	desc string
}

func (e actionItem) Title() string       { return e.id }
func (e actionItem) Description() string { return e.desc }
func (e actionItem) FilterValue() string { return e.id }

// New builds a TUI model for the given config path and parsed config.
func New(cfg *scfg.Config, path string, opts Options) *Model {
	applyTheme(opts.Theme)
	w, h := 80, 24
	hostItems := buildHostItems(cfg, w)
	delegate := newCompactListDelegate()
	l := list.New(hostItems, delegate, w, max(6, h-4))
	l.Title = "SSH hosts"
	l.Styles.Title = titleStyle
	l.SetShowStatusBar(true)
	l.SetFilteringEnabled(true)
	l.DisableQuitKeybindings()

	ti := textinput.New()
	ti.CharLimit = 8192
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
		readOnly:           opts.ReadOnly,
		mirrorPath:         opts.MirrorPath,
		detailTab:          detailTabAll,
		treePaneFocused:    false,
	}
}

func (m *Model) Init() tea.Cmd {
	return nil
}

// InitProgram returns a prepared tea.Program.
func InitProgram(cfg *scfg.Config, path string, opts Options) *tea.Program {
	return tea.NewProgram(New(cfg, path, opts), tea.WithAltScreen())
}

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

func hostTitle(h *scfg.HostBlock) string {
	return hostAlias(h)
}

func (m *Model) rebuildHostList() {
	m.hostList.SetItems(buildHostItems(m.cfg, m.width))
}

func (m *Model) openGroupPicker(returnTo viewMode) {
	m.groupPickerReturnMode = returnTo
	var items []list.Item
	items = append(items, groupPickItem{label: "(default)", toDefault: true, groupIdx: -1})
	for i := range m.cfg.Groups {
		items = append(items, groupPickItem{
			label:     m.cfg.Groups[i].Name,
			toDefault: false,
			groupIdx:  i,
		})
	}
	d := newCompactListDelegate()
	l := list.New(items, d, m.width, m.height-3)
	l.Title = "Move host to group"
	l.Styles.Title = titleStyle
	l.SetShowStatusBar(true)
	l.SetFilteringEnabled(false)
	l.DisableQuitKeybindings()
	m.groupPickerList = l
	m.mode = modeGroupPicker
}

func (m *Model) openDetail(ref scfg.HostRef) {
	m.selRef = ref
	m.detailTab = detailTabAll
	m.treePaneFocused = false
	m.detailList = m.newDetailList()
	m.mode = modeDetail
	m.syncTreeSelection()
	m.layoutDetailPanes()
}

func (m *Model) leftPaneWidth() int {
	if m.width < 56 {
		return max(20, m.width/3)
	}
	lw := m.width * 38 / 100
	if lw < 26 {
		lw = 26
	}
	maxL := m.width / 2
	if lw > maxL {
		lw = maxL
	}
	return lw
}

func (m *Model) rightPaneWidth() int {
	return max(24, m.width-m.leftPaneWidth()-2)
}

func (m *Model) layoutDetailPanes() {
	ph := max(5, m.height-5)
	m.hostList.SetWidth(m.leftPaneWidth())
	m.hostList.SetHeight(ph)
	m.detailList.SetWidth(m.rightPaneWidth())
	m.detailList.SetHeight(ph)
}

func (m *Model) syncTreeSelection() {
	items := m.hostList.Items()
	for i, it := range items {
		if row, ok := it.(hostRowEntry); ok &&
			row.ref.InDefault == m.selRef.InDefault &&
			row.ref.GroupIdx == m.selRef.GroupIdx &&
			row.ref.HostIdx == m.selRef.HostIdx {
			m.hostList.Select(i)
			return
		}
	}
}

func (m *Model) readOnlyBlocked() bool {
	if !m.readOnly {
		return false
	}
	m.status = errStyle.Render("Read-only: Include present (merged view).")
	return true
}

func (m *Model) newDetailList() list.Model {
	h := m.cfg.HostAt(m.selRef)
	var items []list.Item
	switch m.detailTab {
	case detailTabConnectivity:
		for i := range h.Directives {
			d := h.Directives[i]
			if IsConnectivityKey(d.Key) {
				items = append(items, dirEntry{idx: i, d: d})
			}
		}
	case detailTabOverview:
		items = nil
	default: // detailTabAll
		for i := range h.Directives {
			items = append(items, dirEntry{idx: i, d: h.Directives[i]})
		}
	}
	delegate := list.NewDefaultDelegate()
	delegate.ShowDescription = true
	delegate.SetSpacing(0)
	rw := m.width
	rh := m.height - 3
	if m.mode == modeDetail {
		rw = m.rightPaneWidth()
		rh = max(5, m.height-5)
	}
	l := list.New(items, delegate, rw, rh)
	title := detailTabTitle(m.detailTab) + " — " + hostTitle(h)
	if !m.selRef.InDefault && m.selRef.GroupIdx >= 0 && m.selRef.GroupIdx < len(m.cfg.Groups) {
		title += " — " + m.cfg.Groups[m.selRef.GroupIdx].Name
	}
	l.Title = title
	l.Styles.Title = titleStyle
	l.SetShowStatusBar(true)
	l.SetFilteringEnabled(m.detailTab == detailTabAll || m.detailTab == detailTabConnectivity)
	l.DisableQuitKeybindings()
	return l
}

func detailTabTitle(t detailTab) string {
	switch t {
	case detailTabOverview:
		return "Overview"
	case detailTabConnectivity:
		return "Connectivity"
	default:
		return "Directives"
	}
}

func (m *Model) overviewPanel() string {
	h := m.cfg.HostAt(m.selRef)
	var b strings.Builder
	if len(h.HostComments) > 0 {
		b.WriteString(statusStyle.Render("#@host metadata") + "\n")
		for _, line := range h.HostComments {
			b.WriteString(line)
			b.WriteByte('\n')
		}
		b.WriteByte('\n')
	}
	b.WriteString(fmt.Sprintf("Patterns: %s\n", hostAlias(h)))
	b.WriteString(fmt.Sprintf("HostName: %s\n", firstStr(directiveValue(h, "HostName", "hostname"), "—")))
	b.WriteString(fmt.Sprintf("User: %s\n", firstStr(directiveValue(h, "User", "user"), "—")))
	b.WriteString(fmt.Sprintf("Port: %s\n", firstStr(directiveValue(h, "Port", "port"), "—")))
	b.WriteString(fmt.Sprintf("IdentityFile: %s\n", firstStr(directiveValue(h, "IdentityFile", "identityfile"), "—")))
	box := lipgloss.NewStyle().
		Border(panelBorder).
		Padding(0, 1).
		Width(m.rightPaneWidth() - 2).
		Render(strings.TrimSuffix(b.String(), "\n"))
	return box
}

func firstStr(s, def string) string {
	if strings.TrimSpace(s) == "" {
		return def
	}
	return s
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
	delegate.SetSpacing(0)
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
	if m.readOnly {
		return fmt.Errorf("read-only: config uses Include")
	}
	out, err := scfg.String(m.cfg)
	if err != nil {
		return fmt.Errorf("serialize: %w", err)
	}
	body := []byte(out)

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

	if werr := os.WriteFile(m.path, body, 0o600); werr != nil {
		return fmt.Errorf("write config: %w", werr)
	}

	if m.mirrorPath != "" {
		parent := filepath.Dir(m.mirrorPath)
		if mk := os.MkdirAll(parent, 0o755); mk != nil {
			return fmt.Errorf("git mirror mkdir: %w", mk)
		}
		if werr := os.WriteFile(m.mirrorPath, body, 0o600); werr != nil {
			return fmt.Errorf("git mirror write %s: %w", m.mirrorPath, werr)
		}
	}

	m.dirty = false
	return nil
}

// shellProcDoneMsg is sent after ssh/sftp subprocess exits (tea.ExecProcess).
type shellProcDoneMsg struct {
	err error
}

func sshConnectAlias(h *scfg.HostBlock) (string, bool) {
	if len(h.Patterns) != 1 {
		return "", false
	}
	p := strings.TrimSpace(h.Patterns[0])
	if p == "" || strings.ContainsAny(p, "*?!") {
		return "", false
	}
	return p, true
}

func (m *Model) openActionMenu(returnTo viewMode) {
	m.actionReturnMode = returnTo
	if row, ok := m.hostList.SelectedItem().(hostRowEntry); ok && returnTo == modeTree {
		m.selRef = row.ref
	}
	items := []list.Item{
		actionItem{id: "ssh", desc: "SSH session (single alias)"},
		actionItem{id: "sftp", desc: "SFTP session"},
		actionItem{id: "copy", desc: "Copy ssh command"},
		actionItem{id: "cancel", desc: ""},
	}
	d := list.NewDefaultDelegate()
	d.ShowDescription = true
	l := list.New(items, d, min(56, m.width-4), min(8, m.height-4))
	l.Title = "Actions"
	l.Styles.Title = titleStyle
	l.SetFilteringEnabled(false)
	l.DisableQuitKeybindings()
	m.actionMenuList = l
	m.mode = modeActionMenu
}

func (m *Model) sshExecCmd(sftp bool) tea.Cmd {
	h := m.cfg.HostAt(m.selRef)
	alias, ok := sshConnectAlias(h)
	if !ok {
		return func() tea.Msg {
			return shellProcDoneMsg{err: fmt.Errorf("need one non-wildcard Host pattern")}
		}
	}
	var c *exec.Cmd
	if sftp {
		c = exec.Command("sftp", alias)
	} else {
		c = exec.Command("ssh", alias)
	}
	return tea.ExecProcess(c, func(err error) tea.Msg {
		return shellProcDoneMsg{err: err}
	})
}

func (m *Model) reloadFromDisk() {
	data, err := os.ReadFile(m.path)
	if err != nil && !os.IsNotExist(err) {
		m.status = errStyle.Render("Reload: " + err.Error())
		return
	}
	var cfg *scfg.Config
	if len(data) == 0 {
		cfg = &scfg.Config{}
	} else {
		cfg, err = scfg.Parse(strings.NewReader(string(data)))
		if err != nil {
			m.status = errStyle.Render("Reload parse: " + err.Error())
			return
		}
	}
	if cfg.HasInclude {
		m.cfg = scfg.MergeIncludes(m.path, cfg)
		m.readOnly = true
	} else {
		m.cfg = cfg
		m.readOnly = false
	}
	m.dirty = false
	m.rebuildHostList()
	m.status = "Reloaded from disk."
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.hostList.SetWidth(msg.Width)
		m.hostList.SetHeight(max(6, msg.Height-4))
		if m.mode == modeTree {
			m.rebuildHostList()
		}
		if m.mode == modeDetail {
			m.layoutDetailPanes()
			m.detailList = m.newDetailList()
		}
		if m.mode == modePicker {
			m.pickerList.SetWidth(msg.Width)
			m.pickerList.SetHeight(max(6, msg.Height-3))
		}
		if m.mode == modeGroupPicker {
			m.groupPickerList.SetWidth(msg.Width)
			m.groupPickerList.SetHeight(max(6, msg.Height-3))
		}
		if m.mode == modeActionMenu {
			m.actionMenuList.SetWidth(min(56, msg.Width-4))
			m.actionMenuList.SetHeight(min(8, msg.Height-4))
		}
		return m, nil

	case shellProcDoneMsg:
		if m.mode == modeActionMenu {
			m.mode = m.actionReturnMode
			if m.mode == modeDetail {
				m.layoutDetailPanes()
				m.refreshDetailList()
			}
		}
		if msg.err != nil {
			m.status = errStyle.Render(msg.err.Error())
		} else {
			m.status = "Session exited."
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
				if m.readOnly {
					m.status = errStyle.Render("Read-only.")
					m.mode = m.confirmReturnMode
					return m, nil
				}
				m.cfg.DeleteHost(m.selRef)
				m.dirty = true
				m.rebuildHostList()
				m.mode = modeTree
				m.status = "Host deleted."
			case "n", "N", "esc":
				m.status = ""
				m.mode = m.confirmReturnMode
				if m.confirmReturnMode == modeDetail {
					m.refreshDetailList()
				}
			}
			return m, nil

		case modeConfirmDeleteGroup:
			switch msg.String() {
			case "y", "Y":
				if m.readOnly {
					m.status = errStyle.Render("Read-only.")
					m.pendingDeleteGroupName = ""
					m.mode = modeTree
					return m, nil
				}
				if err := m.cfg.DeleteGroupByName(m.pendingDeleteGroupName); err != nil {
					m.status = errStyle.Render(err.Error())
				} else {
					m.dirty = true
					m.rebuildHostList()
					m.status = "Group removed; hosts moved to (default)."
				}
				m.pendingDeleteGroupName = ""
				m.mode = modeTree
			case "n", "N", "esc":
				m.status = ""
				m.pendingDeleteGroupName = ""
				m.mode = modeTree
			}
			return m, nil

		case modeGroupPicker:
			return m.updateGroupPicker(msg)

		case modeActionMenu:
			return m.updateActionMenu(msg)

		case modeInputDirectiveValue, modeInputCustomKey, modeInputNewHost, modeInputDuplicateHost,
			modeInputNewGroup, modeInputRenameGroup, modeInputGroupDesc, modeInputHostMeta:
			switch msg.String() {
			case "esc":
				m.editDirectiveIndex = -1
				m.pendingDirectiveKey = ""
				m.editGroupIdx = -1
				m.mode = m.returnAfterInput
				if m.returnAfterInput == modeDetail {
					if m.cfg.ValidateRef(m.selRef) == nil {
						m.refreshDetailList()
					}
				}
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

	case modeInputNewGroup:
		name := strings.TrimSpace(m.valueInput.Value())
		if err := m.cfg.AddGroup(name); err != nil {
			m.status = errStyle.Render(err.Error())
			return m, nil
		}
		m.dirty = true
		m.rebuildHostList()
		m.mode = modeTree
		m.status = fmt.Sprintf("Created group %q.", name)

	case modeInputRenameGroup:
		name := strings.TrimSpace(m.valueInput.Value())
		if err := m.cfg.RenameGroup(m.editGroupIdx, name); err != nil {
			m.status = errStyle.Render(err.Error())
			return m, nil
		}
		m.dirty = true
		m.rebuildHostList()
		m.editGroupIdx = -1
		m.mode = modeDetail
		if m.cfg.ValidateRef(m.selRef) == nil {
			m.refreshDetailList()
		}
		m.status = fmt.Sprintf("Renamed group to %q.", name)

	case modeInputGroupDesc:
		if err := m.cfg.SetGroupDescription(m.editGroupIdx, m.valueInput.Value()); err != nil {
			m.status = errStyle.Render(err.Error())
			return m, nil
		}
		m.dirty = true
		m.rebuildHostList()
		m.editGroupIdx = -1
		m.mode = modeDetail
		if m.cfg.ValidateRef(m.selRef) == nil {
			m.refreshDetailList()
		}
		m.status = "Group description updated."

	case modeInputHostMeta:
		lines := strings.Split(m.valueInput.Value(), "\n")
		var kept []string
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			if !strings.HasPrefix(line, "#") {
				line = "#@host: " + line
			}
			kept = append(kept, line)
		}
		h := m.cfg.HostAt(m.selRef)
		h.HostComments = kept
		m.dirty = true
		m.mode = modeDetail
		m.refreshDetailList()
		m.layoutDetailPanes()
		m.status = "Host metadata updated."
	}
	return m, nil
}

func (m *Model) updateTree(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Always allow force-quit; otherwise let the list own keys while the filter prompt is open.
	if key.Matches(msg, key.NewBinding(key.WithKeys("ctrl+c"))) {
		return m, tea.Quit
	}
	if m.hostList.SettingFilter() {
		var cmd tea.Cmd
		m.hostList, cmd = m.hostList.Update(msg)
		return m, cmd
	}

	switch {
	case key.Matches(msg, key.NewBinding(key.WithKeys("q", "Q"))):
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
		m.reloadFromDisk()
		return m, nil

	case msg.String() == "A":
		if _, ok := m.hostList.SelectedItem().(hostRowEntry); ok {
			m.openActionMenu(modeTree)
			m.actionMenuList.SetWidth(min(56, m.width-4))
			m.actionMenuList.SetHeight(min(8, m.height-4))
			return m, nil
		}

	case msg.String() == "n":
		if m.readOnlyBlocked() {
			return m, nil
		}
		m.returnAfterInput = modeTree
		m.mode = modeInputNewHost
		m.valueInput.SetValue("")
		m.valueInput.Placeholder = "Host patterns (space-separated)"
		m.valueInput.Focus()
		return m, textinput.Blink

	case msg.String() == "v":
		if m.readOnlyBlocked() {
			return m, nil
		}
		return m, m.rawEditorCmd()

	case msg.String() == "enter":
		if it, ok := m.hostList.SelectedItem().(hostRowEntry); ok {
			m.openDetail(it.ref)
			return m, nil
		}
		return m, nil

	case msg.String() == "x":
		if m.readOnlyBlocked() {
			return m, nil
		}
		if row, ok := m.hostList.SelectedItem().(hostRowEntry); ok {
			m.selRef = row.ref
			m.confirmReturnMode = modeTree
			m.mode = modeConfirmDeleteHost
			m.status = fmt.Sprintf("Delete host %q? [y/N]", hostTitle(m.cfg.HostAt(m.selRef)))
			return m, nil
		}

	case msg.String() == "g":
		if m.readOnlyBlocked() {
			return m, nil
		}
		if row, ok := m.hostList.SelectedItem().(hostRowEntry); ok {
			m.selRef = row.ref
			m.openGroupPicker(modeTree)
			m.groupPickerList.SetWidth(m.width)
			m.groupPickerList.SetHeight(max(6, m.height-3))
			return m, nil
		}

	case msg.String() == "c":
		if m.readOnlyBlocked() {
			return m, nil
		}
		m.returnAfterInput = modeTree
		m.mode = modeInputNewGroup
		m.valueInput.SetValue("")
		m.valueInput.Placeholder = "new group name (not (default))"
		m.valueInput.Focus()
		return m, textinput.Blink

	case msg.String() == "D":
		if m.readOnlyBlocked() {
			return m, nil
		}
		if gh, ok := m.hostList.SelectedItem().(groupHeaderEntry); ok {
			if gh.label == "(default)" {
				m.status = errStyle.Render("(default) cannot be deleted.")
				return m, nil
			}
			m.pendingDeleteGroupName = gh.label
			m.mode = modeConfirmDeleteGroup
			m.status = fmt.Sprintf("Delete group %q and move its hosts to (default)? [y/N]", gh.label)
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.hostList, cmd = m.hostList.Update(msg)
	return m, cmd
}

func (m *Model) updateDetail(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if key.Matches(msg, key.NewBinding(key.WithKeys("ctrl+c"))) {
		return m, tea.Quit
	}

	if m.treePaneFocused {
		if m.hostList.SettingFilter() {
			var cmd tea.Cmd
			m.hostList, cmd = m.hostList.Update(msg)
			return m, cmd
		}
		switch {
		case msg.String() == "tab":
			m.treePaneFocused = false
			return m, nil
		case msg.String() == "enter":
			if it, ok := m.hostList.SelectedItem().(hostRowEntry); ok {
				m.selRef = it.ref
				m.detailTab = detailTabAll
				m.treePaneFocused = false
				m.refreshDetailList()
				m.layoutDetailPanes()
			}
			return m, nil
		case key.Matches(msg, key.NewBinding(key.WithKeys("esc"))):
			m.mode = modeTree
			m.rebuildHostList()
			return m, nil
		}
		var cmd tea.Cmd
		m.hostList, cmd = m.hostList.Update(msg)
		return m, cmd
	}

	if m.detailTab == detailTabAll || m.detailTab == detailTabConnectivity {
		if m.detailList.SettingFilter() {
			var cmd tea.Cmd
			m.detailList, cmd = m.detailList.Update(msg)
			return m, cmd
		}
	}

	switch {
	case msg.String() == "tab":
		m.treePaneFocused = true
		m.syncTreeSelection()
		return m, nil
	case msg.String() == "t":
		m.detailTab = (m.detailTab + 1) % 3
		m.refreshDetailList()
		m.layoutDetailPanes()
		return m, nil
	case key.Matches(msg, key.NewBinding(key.WithKeys("esc"))):
		m.mode = modeTree
		m.rebuildHostList()
		return m, nil
	case msg.String() == "s":
		if err := m.save(); err != nil {
			m.status = errStyle.Render("Save: " + err.Error())
		} else {
			m.status = "Saved."
		}
		return m, nil
	case msg.String() == "A":
		m.openActionMenu(modeDetail)
		m.actionMenuList.SetWidth(min(56, m.width-4))
		m.actionMenuList.SetHeight(min(8, m.height-4))
		return m, nil
	case msg.String() == "i":
		if m.readOnlyBlocked() {
			return m, nil
		}
		m.returnAfterInput = modeDetail
		m.mode = modeInputHostMeta
		h := m.cfg.HostAt(m.selRef)
		m.valueInput.SetValue(strings.Join(h.HostComments, "\n"))
		m.valueInput.Placeholder = "lines (prefix # or plain text → #@host:)"
		m.valueInput.Focus()
		return m, textinput.Blink
	}

	if m.detailTab == detailTabOverview {
		var cmd tea.Cmd
		m.detailList, cmd = m.detailList.Update(msg)
		return m, cmd
	}

	switch {
	case msg.String() == "a":
		if m.readOnlyBlocked() {
			return m, nil
		}
		m.returnAfterInput = modeDetail
		m.openPicker()
		m.pickerList.SetWidth(m.rightPaneWidth())
		m.pickerList.SetHeight(max(6, m.height-5))
		return m, nil
	case msg.String() == "k":
		if m.readOnlyBlocked() {
			return m, nil
		}
		m.returnAfterInput = modeDetail
		m.editDirectiveIndex = -1
		m.mode = modeInputCustomKey
		m.keyInput.SetValue("")
		m.keyInput.Focus()
		return m, textinput.Blink
	case msg.String() == "e":
		if it, ok := m.detailList.SelectedItem().(dirEntry); ok {
			if m.readOnlyBlocked() {
				return m, nil
			}
			m.returnAfterInput = modeDetail
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
		if m.readOnlyBlocked() {
			return m, nil
		}
		if it, ok := m.detailList.SelectedItem().(dirEntry); ok {
			h := m.cfg.HostAt(m.selRef)
			h.Directives = append(h.Directives[:it.idx], h.Directives[it.idx+1:]...)
			m.dirty = true
			m.refreshDetailList()
			m.status = "Directive removed."
		}
		return m, nil
	case msg.String() == "D":
		if m.readOnlyBlocked() {
			return m, nil
		}
		m.returnAfterInput = modeDetail
		m.mode = modeInputDuplicateHost
		m.valueInput.SetValue(hostTitle(m.cfg.HostAt(m.selRef)) + "-copy")
		m.valueInput.Placeholder = "new Host patterns"
		m.valueInput.Focus()
		return m, textinput.Blink
	case msg.String() == "X":
		if m.readOnlyBlocked() {
			return m, nil
		}
		m.confirmReturnMode = modeDetail
		m.mode = modeConfirmDeleteHost
		m.status = fmt.Sprintf("Delete host %q? [y/N]", hostTitle(m.cfg.HostAt(m.selRef)))
		return m, nil
	case msg.String() == "v":
		if m.readOnlyBlocked() {
			return m, nil
		}
		return m, m.rawEditorCmd()
	case msg.String() == "g":
		if m.readOnlyBlocked() {
			return m, nil
		}
		m.openGroupPicker(modeDetail)
		m.groupPickerList.SetWidth(m.width)
		m.groupPickerList.SetHeight(max(6, m.height-3))
		return m, nil
	case msg.String() == "m":
		if m.selRef.InDefault {
			m.status = errStyle.Render("Host is in (default); use g to move it to a named group.")
			return m, nil
		}
		if m.readOnlyBlocked() {
			return m, nil
		}
		m.returnAfterInput = modeDetail
		m.editGroupIdx = m.selRef.GroupIdx
		m.mode = modeInputRenameGroup
		m.valueInput.SetValue(m.cfg.Groups[m.selRef.GroupIdx].Name)
		m.valueInput.Placeholder = "group name"
		m.valueInput.Focus()
		return m, textinput.Blink
	case msg.String() == "o":
		if m.selRef.InDefault {
			m.status = errStyle.Render("Host is in (default); no group description.")
			return m, nil
		}
		if m.readOnlyBlocked() {
			return m, nil
		}
		m.returnAfterInput = modeDetail
		m.editGroupIdx = m.selRef.GroupIdx
		m.mode = modeInputGroupDesc
		m.valueInput.SetValue(groupDescEditPreview(m.cfg.Groups[m.selRef.GroupIdx].Descriptions))
		m.valueInput.Placeholder = "one-line description (empty clears #@desc)"
		m.valueInput.Focus()
		return m, textinput.Blink
	}

	var cmd tea.Cmd
	m.detailList, cmd = m.detailList.Update(msg)
	return m, cmd
}

func (m *Model) updateActionMenu(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if key.Matches(msg, key.NewBinding(key.WithKeys("ctrl+c"))) {
		return m, tea.Quit
	}
	if msg.String() == "esc" {
		m.mode = m.actionReturnMode
		if m.mode == modeDetail {
			m.layoutDetailPanes()
			m.refreshDetailList()
		}
		return m, nil
	}
	if msg.String() == "enter" {
		if it, ok := m.actionMenuList.SelectedItem().(actionItem); ok {
			switch it.id {
			case "cancel":
				m.mode = m.actionReturnMode
				if m.mode == modeDetail {
					m.layoutDetailPanes()
					m.refreshDetailList()
				}
				return m, nil
			case "ssh":
				return m, m.sshExecCmd(false)
			case "sftp":
				return m, m.sshExecCmd(true)
			case "copy":
				alias, ok := sshConnectAlias(m.cfg.HostAt(m.selRef))
				if !ok {
					m.status = errStyle.Render("Need one non-wildcard Host pattern.")
					return m, nil
				}
				cmd := "ssh " + alias
				if err := clipboard.WriteAll(cmd); err != nil {
					m.status = errStyle.Render(err.Error())
				} else {
					m.status = "Copied: " + cmd
				}
				m.mode = m.actionReturnMode
				if m.mode == modeDetail {
					m.layoutDetailPanes()
					m.refreshDetailList()
				}
				return m, nil
			}
		}
	}
	var cmd tea.Cmd
	m.actionMenuList, cmd = m.actionMenuList.Update(msg)
	return m, cmd
}

func (m *Model) updatePicker(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if key.Matches(msg, key.NewBinding(key.WithKeys("ctrl+c"))) {
		return m, tea.Quit
	}
	if m.pickerList.SettingFilter() {
		var cmd tea.Cmd
		m.pickerList, cmd = m.pickerList.Update(msg)
		return m, cmd
	}

	switch {
	case key.Matches(msg, key.NewBinding(key.WithKeys("esc"))):
		m.mode = modeDetail
		return m, nil

	case msg.String() == "enter":
		if it, ok := m.pickerList.SelectedItem().(kwEntry); ok {
			m.returnAfterInput = modeDetail
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

func (m *Model) updateGroupPicker(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, key.NewBinding(key.WithKeys("esc"))):
		m.mode = m.groupPickerReturnMode
		m.status = ""
		if m.groupPickerReturnMode == modeDetail && m.cfg.ValidateRef(m.selRef) == nil {
			m.refreshDetailList()
		}
		return m, nil
	case msg.String() == "enter":
		if it, ok := m.groupPickerList.SelectedItem().(groupPickItem); ok {
			if m.readOnly {
				m.status = errStyle.Render("Read-only.")
				m.mode = m.groupPickerReturnMode
				return m, nil
			}
			if err := m.cfg.MoveHost(m.selRef, it.toDefault, it.groupIdx); err != nil {
				m.status = errStyle.Render(err.Error())
			} else {
				m.dirty = true
				m.rebuildHostList()
				m.status = fmt.Sprintf("Moved host to %q.", it.label)
			}
			m.mode = modeTree
			return m, nil
		}
	}
	var cmd tea.Cmd
	m.groupPickerList, cmd = m.groupPickerList.Update(msg)
	return m, cmd
}

func (m Model) View() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("sshui — "))
	b.WriteString(m.path)
	if m.readOnly {
		b.WriteString(errStyle.Render(" [read-only]"))
	}
	if m.dirty {
		b.WriteString(errStyle.Render(" *"))
	}
	b.WriteByte('\n')
	if m.cfg.HasInclude && !m.readOnly {
		b.WriteString(errStyle.Render("Warning: file contains Include; only this file is edited. "))
		b.WriteByte('\n')
	}
	if m.cfg.HasInclude && m.readOnly {
		b.WriteString(statusStyle.Render("Include: merged view (browse only). "))
		b.WriteByte('\n')
	}
	switch m.mode {
	case modeHelp:
		b.WriteString(helpStyle.Render(helpText))
		return b.String()

	case modeConfirmDeleteHost, modeConfirmDeleteGroup:
		box := lipgloss.NewStyle().Border(panelBorder).Padding(1, 2).Width(min(56, m.width-4)).Render(m.status + "\n\n[y/N]")
		b.WriteString(lipgloss.Place(m.width, max(8, m.height-2), lipgloss.Center, lipgloss.Center, box))
		return b.String()

	case modeActionMenu:
		menu := lipgloss.NewStyle().Border(panelBorder).Padding(0, 1).Render(m.actionMenuList.View())
		b.WriteString(lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, menu))
		return b.String()

	case modeInputDirectiveValue, modeInputNewHost, modeInputDuplicateHost, modeInputNewGroup, modeInputRenameGroup, modeInputGroupDesc, modeInputHostMeta:
		switch m.mode {
		case modeInputNewGroup:
			b.WriteString(statusStyle.Render("New group name, Esc cancel"))
		case modeInputRenameGroup:
			b.WriteString(statusStyle.Render("Rename group, Esc cancel"))
		case modeInputGroupDesc:
			b.WriteString(statusStyle.Render("Group #@desc line (empty clears), Esc cancel"))
		case modeInputHostMeta:
			b.WriteString(statusStyle.Render("Host #@host lines, Esc cancel"))
		default:
			b.WriteString(statusStyle.Render("Enter value, Esc cancel"))
		}
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
		ph := max(5, m.height-5)
		left := lipgloss.NewStyle().Width(m.leftPaneWidth()).Height(ph).Render(m.hostList.View())
		var right string
		if m.detailTab == detailTabOverview {
			right = lipgloss.NewStyle().Width(m.rightPaneWidth()).Height(ph).Render(m.overviewPanel())
		} else {
			right = lipgloss.NewStyle().Width(m.rightPaneWidth()).Height(ph).Render(m.detailList.View())
		}
		b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, left, " ", right))
		b.WriteByte('\n')
		focus := "detail"
		if m.treePaneFocused {
			focus = "tree"
		}
		b.WriteString(statusStyle.Render(fmt.Sprintf("focus %s | tab switch | t tab view | i host meta | A actions | a add | k e d D g m o X v s | esc tree", focus)))
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

	case modeGroupPicker:
		b.WriteString(m.groupPickerList.View())
		b.WriteByte('\n')
		escHint := "tree"
		if m.groupPickerReturnMode == modeDetail {
			escHint = "host detail"
		}
		b.WriteString(statusStyle.Render("enter: move host here | esc: back to " + escHint))
		return b.String()

	default: // tree
		b.WriteString(colHeaderStyle.Render(HostListColumnHeader(m.width)))
		b.WriteByte('\n')
		b.WriteString(m.hostList.View())
		b.WriteByte('\n')
		b.WriteString(statusStyle.Render("enter open | A actions | n host | c group | D del grp | x del host | g move | v raw | s r ? q"))
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
  enter     Open host (group rows are section headers only)
  Column header row shows: Host (patterns) | HostName | User
  /         Filter; while filter is open, letter keys go to filter
  A         Actions: ssh / sftp / copy command (host row)
  n         New host under (default)
  c         Create new empty group
  D         Delete group (when a group header is selected)
  x         Delete host (confirm)
  g         Move selected host to group / (default)
  v         Raw $EDITOR buffer
  s / r     Save / reload
  ? / q     Help / quit

Host detail (split: tree | detail)
  tab       Focus tree vs detail pane
  t         Cycle tab: Overview → All directives → Connectivity
  i         Edit #@host metadata lines (multiline)
  A         Actions menu (ssh / sftp / copy)
  a / k     Add directive (picker / custom key)
  e / d     Edit value / delete directive
  D         Duplicate host
  g         Move host to another group
  m / o     Rename group / edit #@desc
  X         Delete host (confirm)
  v / s     Raw editor / save
  esc       Back to tree

Include: configs with Include are read-only; included files are merged for browsing.

CLI: sshui list | sshui show HOST [--json] | sshui dump [--json] [--check] | sshui completion bash|zsh|fish

Optional: ~/.config/sshui/config.toml — ssh_config, editor, theme, ssh_config_git_mirror (copy-on-save path).

NO_COLOR=1 disables ANSI styling.

Each save writes a hidden .bkp beside the config; optional mirror path gets the same bytes (0600).
`
