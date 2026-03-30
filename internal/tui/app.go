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
	ReadOnly   bool   // true: merged Include browse (see help); false: single-file editable
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

	ti := textinput.New()
	ti.CharLimit = 8192
	ti.Width = 60

	ki := textinput.New()
	ki.CharLimit = 128
	ki.Width = 40
	ki.Placeholder = "DirectiveKey (custom / future keywords)"

	m := &Model{
		cfg:                cfg,
		path:               path,
		width:              w,
		height:             h,
		valueInput:         ti,
		keyInput:           ki,
		editDirectiveIndex: -1,
		editor:             opts.Editor,
		readOnly:           opts.ReadOnly,
		mirrorPath:         opts.MirrorPath,
		detailTab:          detailTabAll,
		treePaneFocused:    false,
	}
	lw := m.leftPaneWidth()
	hostItems := buildHostItems(cfg, lw)
	delegate := newCompactListDelegate()
	l := list.New(hostItems, delegate, lw, max(6, h-4))
	l.Title = "Hosts by group   c=new group   on header: D=delete"
	l.Styles.Title = titleStyle
	l.SetShowStatusBar(true)
	l.SetFilteringEnabled(true)
	l.DisableQuitKeybindings()
	m.hostList = l
	for i, it := range m.hostList.Items() {
		if row, ok := it.(hostRowEntry); ok {
			m.selRef = row.ref
			m.hostList.Select(i)
			break
		}
	}
	// detailList must exist before layoutDetailPanes — a zero list.Model panics on SetSize.
	m.detailList = m.newDetailList()
	m.layoutDetailPanes()
	return m
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
	m.hostList.SetItems(buildHostItems(m.cfg, m.leftPaneWidth()))
}

// resyncSelectionAfterStructureChange fixes selRef and list cursor after cfg shape changes.
func (m *Model) resyncSelectionAfterStructureChange() {
	if m.cfg.ValidateRef(m.selRef) == nil {
		m.syncTreeSelection()
		m.refreshDetailList()
		return
	}
	for i, it := range m.hostList.Items() {
		if row, ok := it.(hostRowEntry); ok {
			m.selRef = row.ref
			m.hostList.Select(i)
			break
		}
	}
	m.refreshDetailList()
}

// toggleIncludeEditMode switches between merged read-only browse (all Include files) and
// editable single-file view (main path only). No-op if the file has no Include.
func (m *Model) toggleIncludeEditMode() {
	if !m.cfg.HasInclude {
		m.status = statusStyle.Render("No Include in this file.")
		return
	}
	if m.readOnly {
		data, err := os.ReadFile(m.path)
		if err != nil && !os.IsNotExist(err) {
			m.status = errStyle.Render("Read: " + err.Error())
			return
		}
		var cfg *scfg.Config
		if len(data) == 0 {
			cfg = &scfg.Config{}
		} else {
			cfg, err = scfg.Parse(strings.NewReader(string(data)))
			if err != nil {
				m.status = errStyle.Render("Parse: " + err.Error())
				return
			}
		}
		m.cfg = cfg
		m.readOnly = false
		m.dirty = false
		m.rebuildHostList()
		m.layoutDetailPanes()
		m.resyncSelectionAfterStructureChange()
		m.status = fmt.Sprintf("Writable: only %s (included hosts hidden). Save writes this file. Press W or r for merged read-only browse.", m.path)
		return
	}
	if m.dirty {
		m.status = errStyle.Render("Save (s) or reload (r) before switching back to merged Include view.")
		return
	}
	data, err := os.ReadFile(m.path)
	if err != nil && !os.IsNotExist(err) {
		m.status = errStyle.Render("Read: " + err.Error())
		return
	}
	var cfg *scfg.Config
	if len(data) == 0 {
		cfg = &scfg.Config{}
	} else {
		cfg, err = scfg.Parse(strings.NewReader(string(data)))
		if err != nil {
			m.status = errStyle.Render("Parse: " + err.Error())
			return
		}
	}
	if !cfg.HasInclude {
		m.status = errStyle.Render("Include was removed on disk; staying in single-file view.")
		m.cfg = cfg
		m.readOnly = false
		m.rebuildHostList()
		m.layoutDetailPanes()
		m.resyncSelectionAfterStructureChange()
		return
	}
	m.cfg = scfg.MergeIncludes(m.path, cfg)
	m.readOnly = true
	m.dirty = false
	m.rebuildHostList()
	m.layoutDetailPanes()
	m.resyncSelectionAfterStructureChange()
	m.status = "Merged read-only browse (all Include files). W = edit main file only."
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
	// Browse (tree) draws a 1-line column header above the list inside the left pane.
	if m.mode == modeTree {
		m.hostList.SetHeight(max(4, ph-1))
	} else {
		m.hostList.SetHeight(ph)
	}
	m.detailList.SetWidth(m.rightPaneWidth())
	m.detailList.SetHeight(ph)
}

// syncBrowsePreview updates selRef and the directive preview when the cursor is on a
// host row in tree (browse) mode.
func (m *Model) syncBrowsePreview() {
	if m.mode != modeTree {
		return
	}
	row, ok := m.hostList.SelectedItem().(hostRowEntry)
	if !ok {
		return
	}
	if m.selRef == row.ref {
		return
	}
	m.selRef = row.ref
	m.refreshDetailList()
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
	m.status = "Read-only: merged Include view — Press W to edit the main file only, or ? for help."
	return true
}

func (m *Model) newDetailList() list.Model {
	rw := m.width
	rh := m.height - 3
	if m.mode == modeDetail || m.mode == modeTree {
		rw = m.rightPaneWidth()
		rh = max(5, m.height-5)
	}
	delegate := list.NewDefaultDelegate()
	delegate.ShowDescription = true
	delegate.SetSpacing(0)
	delegate.Styles.FilterMatch = filterMatchStyle

	var items []list.Item
	var title string
	if err := m.cfg.ValidateRef(m.selRef); err != nil {
		title = detailTabTitle(m.detailTab) + " — (select a host)"
	} else {
		h := m.cfg.HostAt(m.selRef)
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
		sub := HostConnectivityTitle(h)
		title = detailTabTitle(m.detailTab) + " — " + sub
		if !m.selRef.InDefault && m.selRef.GroupIdx >= 0 && m.selRef.GroupIdx < len(m.cfg.Groups) {
			title += " — " + m.cfg.Groups[m.selRef.GroupIdx].Name
		}
	}
	l := list.New(items, delegate, rw, rh)
	l.Title = title
	l.Styles.Title = titleStyle
	l.SetShowStatusBar(true)
	filterOK := m.cfg.ValidateRef(m.selRef) == nil
	l.SetFilteringEnabled(filterOK && (m.detailTab == detailTabAll || m.detailTab == detailTabConnectivity))
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
	if m.cfg.ValidateRef(m.selRef) != nil {
		return lipgloss.NewStyle().
			Border(panelBorder).
			Padding(0, 1).
			Width(m.rightPaneWidth() - 2).
			Render(statusStyle.Render("No host selected."))
	}
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
	delegate.Styles.FilterMatch = filterMatchStyle
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
		return fmt.Errorf("read-only merged Include view (press W to edit main file only)")
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
	m.layoutDetailPanes()
	m.syncBrowsePreview()
	m.status = "Reloaded from disk."
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		switch m.mode {
		case modeTree, modeDetail:
			m.layoutDetailPanes()
			m.rebuildHostList()
			m.detailList = m.newDetailList()
		default:
			m.hostList.SetWidth(msg.Width)
			m.hostList.SetHeight(max(6, msg.Height-4))
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
			} else if m.mode == modeTree {
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
			m.layoutDetailPanes()
			m.refreshDetailList()
			return m, nil

		case modeConfirmDeleteHost:
			switch msg.String() {
			case "y", "Y":
				if m.readOnly {
					m.status = errStyle.Render("Read-only.")
					m.mode = m.confirmReturnMode
					if m.confirmReturnMode == modeDetail {
						m.layoutDetailPanes()
						m.refreshDetailList()
					} else if m.confirmReturnMode == modeTree {
						m.layoutDetailPanes()
						m.refreshDetailList()
					}
					return m, nil
				}
				m.cfg.DeleteHost(m.selRef)
				m.dirty = true
				m.mode = modeTree
				m.layoutDetailPanes()
				m.rebuildHostList()
				m.syncBrowsePreview()
				m.refreshDetailList()
				m.status = "Host deleted."
			case "n", "N", "esc":
				m.status = ""
				m.mode = m.confirmReturnMode
				if m.confirmReturnMode == modeDetail {
					m.layoutDetailPanes()
					m.refreshDetailList()
				} else if m.confirmReturnMode == modeTree {
					m.layoutDetailPanes()
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
					m.layoutDetailPanes()
					return m, nil
				}
				gi := -1
				for i := range m.cfg.Groups {
					if m.cfg.Groups[i].Name == m.pendingDeleteGroupName {
						gi = i
						break
					}
				}
				nDef := len(m.cfg.DefaultHosts)
				oldRef := m.selRef
				name := m.pendingDeleteGroupName
				if err := m.cfg.DeleteGroupByName(name); err != nil {
					m.status = errStyle.Render(err.Error())
				} else {
					m.dirty = true
					m.status = "Group removed; hosts moved to (default)."
					if gi >= 0 {
						if !oldRef.InDefault && oldRef.GroupIdx == gi {
							m.selRef = scfg.HostRef{InDefault: true, HostIdx: nDef + oldRef.HostIdx}
						} else if !oldRef.InDefault && oldRef.GroupIdx > gi {
							m.selRef.GroupIdx--
						}
					}
				}
				m.pendingDeleteGroupName = ""
				m.mode = modeTree
				m.layoutDetailPanes()
				m.rebuildHostList()
				m.syncTreeSelection()
				m.refreshDetailList()
			case "n", "N", "esc":
				m.status = ""
				m.pendingDeleteGroupName = ""
				m.mode = modeTree
				m.layoutDetailPanes()
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
		m.mode = modeTree
		m.layoutDetailPanes()
		m.rebuildHostList()
		m.refreshDetailList()
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
		m.syncBrowsePreview()
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

	case msg.String() == "W":
		m.toggleIncludeEditMode()
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
	m.syncBrowsePreview()
	return m, cmd
}

func (m *Model) updateDetail(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if key.Matches(msg, key.NewBinding(key.WithKeys("ctrl+c"))) {
		return m, tea.Quit
	}
	if msg.String() == "W" {
		m.toggleIncludeEditMode()
		m.layoutDetailPanes()
		m.refreshDetailList()
		return m, nil
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
			m.layoutDetailPanes()
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
		m.layoutDetailPanes()
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
		} else if m.mode == modeTree {
			m.layoutDetailPanes()
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
				} else if m.mode == modeTree {
					m.layoutDetailPanes()
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
				} else if m.mode == modeTree {
					m.layoutDetailPanes()
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
		m.layoutDetailPanes()
		m.refreshDetailList()
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
		m.layoutDetailPanes()
		if m.groupPickerReturnMode == modeDetail && m.cfg.ValidateRef(m.selRef) == nil {
			m.refreshDetailList()
		}
		if m.groupPickerReturnMode == modeTree {
			m.refreshDetailList()
		}
		return m, nil
	case msg.String() == "enter":
		if it, ok := m.groupPickerList.SelectedItem().(groupPickItem); ok {
			if m.readOnly {
				m.status = errStyle.Render("Read-only.")
				m.mode = m.groupPickerReturnMode
				m.layoutDetailPanes()
				if m.groupPickerReturnMode == modeDetail {
					m.refreshDetailList()
				}
				if m.groupPickerReturnMode == modeTree {
					m.refreshDetailList()
				}
				return m, nil
			}
			if err := m.cfg.MoveHost(m.selRef, it.toDefault, it.groupIdx); err != nil {
				m.status = errStyle.Render(err.Error())
			} else {
				m.dirty = true
				m.status = fmt.Sprintf("Moved host to %q.", it.label)
				if it.toDefault {
					m.selRef = scfg.HostRef{InDefault: true, HostIdx: len(m.cfg.DefaultHosts) - 1}
				} else {
					g := it.groupIdx
					m.selRef = scfg.HostRef{InDefault: false, GroupIdx: g, HostIdx: len(m.cfg.Groups[g].Hosts) - 1}
				}
			}
			m.mode = m.groupPickerReturnMode
			m.layoutDetailPanes()
			m.rebuildHostList()
			m.syncTreeSelection()
			if m.mode == modeDetail {
				m.refreshDetailList()
			} else {
				m.syncBrowsePreview()
				m.refreshDetailList()
			}
			return m, nil
		}
	}
	var cmd tea.Cmd
	m.groupPickerList, cmd = m.groupPickerList.Update(msg)
	return m, cmd
}

// footerStatusRender applies list-footer styling: green for read-only Include hints, yellow
// warning for writable Include hints, passes through already-styled (ANSI) strings, else dim status.
func (m *Model) footerStatusRender() string {
	if m.status == "" {
		return ""
	}
	s := m.status
	if len(s) > 0 && s[0] == '\x1b' {
		return s
	}
	if m.readOnly && m.cfg.HasInclude {
		if strings.HasPrefix(s, "Read-only: merged Include view") ||
			strings.HasPrefix(s, "Merged read-only browse") {
			return readOnlyBannerStyle.Render(s)
		}
	}
	if !m.readOnly && m.cfg.HasInclude && strings.HasPrefix(s, "Writable:") {
		return writableWarnStyle.Render(s)
	}
	return statusStyle.Render(s)
}

func (m Model) View() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("sshui — "))
	b.WriteString(m.path)
	if m.readOnly {
		b.WriteString(readOnlyBannerStyle.Render(" [read-only merged Include]"))
	}
	if m.dirty {
		b.WriteString(errStyle.Render(" *"))
	}
	b.WriteByte('\n')
	if m.cfg.HasInclude && !m.readOnly {
		b.WriteString(writableWarnStyle.Render(
			"Include: editing main file only (hosts from included files are hidden). Save writes this path. Press W or r for merged read-only browse.",
		))
		b.WriteByte('\n')
	}
	if m.cfg.HasInclude && m.readOnly {
		b.WriteString(readOnlyBannerStyle.Render(fmt.Sprintf(
			"Include: merged read-only — main file plus all Include targets are shown in one tree; save is disabled (cannot write multiple files). Press W to edit only %s. r reloads from disk.",
			m.path,
		)))
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
			b.WriteString(m.footerStatusRender())
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
		b.WriteString(statusStyle.Render(fmt.Sprintf("focus %s | tab switch | t tab view | W Include mode | i host meta | A actions | a add | k e d D g m o X v s | esc tree", focus)))
		if m.status != "" {
			b.WriteByte('\n')
			b.WriteString(m.footerStatusRender())
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

	default: // tree (browse): hosts by group | directive preview
		ph := max(5, m.height-5)
		lw := m.leftPaneWidth()
		header := colHeaderStyle.Width(lw).Render(HostListColumnHeader(lw))
		left := lipgloss.NewStyle().Width(lw).Height(ph).Render(lipgloss.JoinVertical(lipgloss.Left, header, m.hostList.View()))
		var right string
		if _, onHost := m.hostList.SelectedItem().(hostRowEntry); onHost && m.cfg.ValidateRef(m.selRef) == nil {
			right = lipgloss.NewStyle().Width(m.rightPaneWidth()).Height(ph).Render(m.detailList.View())
		} else {
			right = lipgloss.NewStyle().
				Width(m.rightPaneWidth()).
				Height(ph).
				Padding(1, 1).
				Foreground(lipgloss.Color("245")).
				Render("Select a host row to preview directives.\n\nTip: c creates a group; with a group header selected, D deletes that group.")
		}
		b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, left, " ", right))
		b.WriteByte('\n')
		b.WriteString(statusStyle.Render("enter full editor | W Include mode | A actions | n host | c new group | D del grp (header) | x del host | g move | v raw | s r ? q"))
		if m.status != "" {
			b.WriteByte('\n')
			b.WriteString(m.footerStatusRender())
		}
		return b.String()
	}
}

const helpText = `
sshui — SSH client config TUI

Browse (split view)
  Left: Host patterns only, grouped under (default) and #@group sections
  Right: directive preview for the selected host row
  enter     Open full editor (tabs, add/remove directives, …)
  /         Filter host list; filter matches use highlight (not underline)
  A         Actions: ssh / sftp / copy command (host row)
  n         New host under (default)
  c         Create new empty group (also shown in the host list title)
  D         Delete group (when a group header row is selected)
  x         Delete host (confirm)
  g         Move selected host to group / (default)
  v         Raw $EDITOR buffer
  s / r     Save / reload
  W         Toggle Include view: merged read-only browse ↔ editable main file only (when file has Include)
  ? / q     Help / quit

Include / read-only (merged view)
  If your config contains an Include directive, sshui may start in read-only merged mode: it loads
  the main file you opened plus every file matched by Include and shows extra groups like
  include:filename so you can browse all hosts in one place. Saving is disabled because one Save
  could not safely rewrite every included file.

  To edit your main config anyway: press W. You switch to a writable single-file view: only Host
  blocks from that path are shown; save (s) still writes only that path. Included definitions are
  hidden until you press W again (if you have no unsaved changes) or r (reload from disk), which
  restores merged read-only browse if Include is still present.

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

CLI: sshui list | sshui show HOST [--json] | sshui dump [--json] [--check] | sshui completion bash|zsh|fish

Optional: ~/.config/sshui/config.toml — ssh_config, editor, theme, ssh_config_git_mirror (copy-on-save path).

NO_COLOR=1 disables ANSI styling.

Each save writes a hidden .bkp beside the config; optional mirror path gets the same bytes (0600).
`
