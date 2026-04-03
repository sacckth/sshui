package tui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/sacckth/sshui/internal/appcfg"
	scfg "github.com/sacckth/sshui/internal/config"
	"github.com/sacckth/sshui/internal/lockfile"
	"github.com/sacckth/sshui/internal/overlay"
	"github.com/sacckth/sshui/internal/sshkeywords"
)

// Options carries theme and editor preferences (from ~/.config/sshui/config.toml).
type Options struct {
	Version       string
	Theme         string
	Editor        string
	ReadOnly      bool   // true: merged Include browse (see help); false: single-file editable
	MirrorPath    string // optional: after save, copy bytes here (expanded abs path)
	AppConfigPath string // absolute path to sshui config.toml (not SSH config)

	SSHHostsPath       string         // absolute path to the sshui ssh_hosts file
	MainSSHConfigPath  string         // absolute path to the user's main ssh_config (for export wizard)
	MainConfig         *scfg.Config   // parsed main ssh_config snapshot (bridge-stripped); nil if same as ssh_hosts or unreadable
	OverlayPath        string         // absolute path to password_hosts.toml
	Overlay            *overlay.File  // pre-loaded password overlay (nil = none)
	BrowseMode         string         // initial browse mode: merged | openssh | password
	AppConfig          *appcfg.Config // pointer for saving wizard flags
	ExportWizardNeeded bool           // show the setup wizard on startup (config.toml missing)
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
	modeAppCfgView
	modeSSHConnectPending
	modeSSHConnectWildcardInput
	modeExportWizard
	modeNewHostTypePicker
	modeInputNewPasswordHost
	modeInputPasswordField
	modeNewGroupTypePicker
	modeInputNewPasswordGroup
	modePasswordDetail
	modePasswordDetailEdit
	modePasswordDetailInput
	modeLockConflict
	modeConfirmImport // y/N: remove imported host from main ssh_config?
)

type detailTab int

const (
	detailTabOverview detailTab = iota
	detailTabAll
	detailTabConnectivity
)

// Model is the root Bubble Tea model for sshui.
type Model struct {
	cfg     *scfg.Config
	path    string
	version string
	dirty   bool
	width   int
	height  int
	mode    viewMode

	hostList        list.Model
	detailList      list.Model
	pwDetailList    list.Model // password host fields (browse preview + password detail)
	pickerList      list.Model
	groupPickerList list.Model

	valueInput textinput.Model
	keyInput   textinput.Model

	selRef                    scfg.HostRef
	pendingDirectiveKey       string
	editDirectiveIndex        int // >=0 when editing value; -1 when adding
	status                    string
	editor                    string // from app config; VISUAL/EDITOR used when empty
	confirmReturnMode         viewMode
	returnAfterInput          viewMode
	groupPickerReturnMode     viewMode
	pendingDeleteGroupName    string
	pendingDeletePwIdx        int  // >=0: confirm deletes overlay PasswordHosts[idx]; -1: OpenSSH host delete
	pendingDeleteOverlayGroup bool // true: confirm deletes password overlay group (not ssh config)
	editGroupIdx              int

	readOnly         bool
	mirrorPath       string
	detailTab        detailTab
	treePaneFocused  bool
	actionMenuList   list.Model
	actionReturnMode viewMode

	appConfigPath    string
	themeName        string
	appCfgReturnMode viewMode
	appCfgViewport   viewport.Model

	helpViewport   viewport.Model
	helpReturnMode viewMode

	sshConnectReturnMode       viewMode
	sshConnectPendingCancelled bool
	sshConnectPendingAlias     string
	sshConnectPendingNote      string // e.g. multi-pattern warning on pending / wildcard screens
	sshConnectPendingSFTP      bool
	sshConnectWildKind         sshWildKind
	sshConnectWildPrefix       string
	sshConnectWildSuffix       string
	sshConnectWildPattern      string // first Host pattern (for prompts)
	sshPostWildKind            sshPostWildKind

	// Browse mode: merged, openssh, password
	browseMode string
	// Edit mode: false = read-only browse/inspect; true = mutations enabled (lock held)
	editMode bool
	// Password overlay
	overlayData       *overlay.File
	overlayPath       string
	sshHostsPath      string
	mainSSHConfigPath string
	mainCfg           *scfg.Config // read-only snapshot of the parent ssh_config (bridge-stripped); nil when same as ssh_hosts
	appConfig         *appcfg.Config
	// Lock state per file
	lockedSSHHosts bool
	lockedOverlay  bool
	// Setup wizard
	exportWizardNeeded bool
	wizardStep         int // 0=ssh_config, 1=ssh_hosts, 2=overlay, 3=copy question
	// Import from main ssh_config
	pendingImportRef scfg.HostRef // the FromMain ref being imported

	// Password host detail
	pwSelIdx       int    // index into overlayData.PasswordHosts
	pwEditFieldKey string // toml field id while editing (hostname, patterns, …)
}

var (
	titleStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
	statusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	errStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	helpStyle   = lipgloss.NewStyle().Padding(1, 2)
	panelBorder = lipgloss.RoundedBorder()
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

	browseMode := opts.BrowseMode
	if browseMode == "" {
		browseMode = appcfg.BrowseModeMerged
	}

	m := &Model{
		cfg:                cfg,
		path:               path,
		version:            opts.Version,
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
		appConfigPath:      opts.AppConfigPath,
		themeName:          opts.Theme,

		browseMode:         browseMode,
		overlayData:        opts.Overlay,
		overlayPath:        opts.OverlayPath,
		sshHostsPath:       opts.SSHHostsPath,
		mainSSHConfigPath:  opts.MainSSHConfigPath,
		mainCfg:            opts.MainConfig,
		appConfig:          opts.AppConfig,
		exportWizardNeeded: opts.ExportWizardNeeded,
		pwSelIdx:           -1,
		pendingDeletePwIdx: -1,
	}
	lw := m.leftPaneWidth()
	hostItems := buildHostItemsFiltered(cfg, m.overlayData, m.browseMode, lw, false, m.mainCfg)
	l := list.New(hostItems, newHostTreeDelegate(), lw, max(6, h-4))
	l.Title = m.logoLine()
	l.Styles.Title = titleStyle
	l.SetShowStatusBar(true)
	l.SetFilteringEnabled(true)
	l.DisableQuitKeybindings()
	m.hostList = l
	patchFilterAcceptKeys(&m.hostList)
	applyPaneChromeToList(&m.hostList)
	for i, it := range m.hostList.Items() {
		if row, ok := it.(hostRowEntry); ok {
			m.selRef = row.ref
			m.hostList.Select(i)
			break
		}
	}
	// detailList must exist before layoutDetailPanes — a zero list.Model panics on SetSize.
	m.detailList = m.newDetailList()
	m.refreshPasswordDetailList()
	m.layoutDetailPanes()
	return m
}

func (m *Model) Init() tea.Cmd {
	if m.exportWizardNeeded {
		m.mode = modeExportWizard
		m.wizardStep = 0
		m.wizardPrepareStep()
		return textinput.Blink
	}
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
func (e dirEntry) FilterValue() string { return e.d.Key }

// pwFieldEntry is one editable row in the password host detail list.
type pwFieldEntry struct {
	key   string // display label
	field string // hostname, patterns, user, port, askpass, askpass_require, display, group
	val   string // shown as description
}

func (e pwFieldEntry) Title() string       { return e.key }
func (e pwFieldEntry) Description() string { return e.val }
func (e pwFieldEntry) FilterValue() string { return e.key }

type kwEntry sshkeywords.Entry

func (e kwEntry) Title() string       { return e.Name }
func (e kwEntry) Description() string { return e.Hint }
func (e kwEntry) FilterValue() string { return e.Name }

func hostTitle(h *scfg.HostBlock) string {
	return hostAlias(h)
}

func (m *Model) ignoreCollapseForActiveFilter() bool {
	if m.hostList.FilterState() == list.Unfiltered {
		return false
	}
	return strings.TrimSpace(m.hostList.FilterValue()) != ""
}

type hostListFilterSnap struct {
	state list.FilterState
	value string
}

func (m *Model) hostListFilterSnap() hostListFilterSnap {
	return hostListFilterSnap{state: m.hostList.FilterState(), value: m.hostList.FilterValue()}
}

// skipRebuildWhenApplyingFilter avoids rebuildHostList on Enter: bubbles' SetItems clears
// filteredItems and refilters asynchronously, so VisibleItems() is empty for a frame and
// syncTreeSelection would call Select() with a full-list index (wrong for filtered lists).
func skipRebuildWhenApplyingFilter(before, after hostListFilterSnap) bool {
	return before.state == list.Filtering && after.state == list.FilterApplied && before.value == after.value
}

func (m *Model) updateHostList(msg tea.Msg) tea.Cmd {
	before := m.hostListFilterSnap()
	var cmd tea.Cmd
	m.hostList, cmd = m.hostList.Update(msg)
	// SetItems schedules async refilter; selection must run after matches exist.
	if _, ok := msg.(list.FilterMatchesMsg); ok && m.hostList.FilterState() != list.Unfiltered {
		m.syncTreeSelection()
	}
	after := m.hostListFilterSnap()
	if after != before && !skipRebuildWhenApplyingFilter(before, after) {
		rbCmd := m.rebuildHostList()
		cmd = tea.Batch(cmd, rbCmd)
	}
	return cmd
}

// rebuildHostList replaces host list items. When a filter is active, SetItems clears matches until
// FilterMatchesMsg is processed — callers must return the returned tea.Cmd so refilter runs.
func (m *Model) rebuildHostList() tea.Cmd {
	lw := m.leftPaneWidth()
	items := buildHostItemsFiltered(m.cfg, m.overlayData, m.browseMode, lw, m.ignoreCollapseForActiveFilter(), m.mainCfg)
	cmd := m.hostList.SetItems(items)
	if m.hostList.FilterState() == list.Unfiltered {
		m.syncTreeSelection()
	}
	return cmd
}

// resyncSelectionAfterStructureChange fixes selRef and list cursor after cfg shape changes.
func (m *Model) resyncSelectionAfterStructureChange() {
	if m.resolveValidateRef(m.selRef) == nil {
		m.syncTreeSelection()
		m.refreshDetailList()
		m.refreshPasswordDetailList()
		return
	}
	var iterate []list.Item
	if m.hostList.FilterState() != list.Unfiltered {
		iterate = m.hostList.VisibleItems()
	} else {
		iterate = m.hostList.Items()
	}
	for i, it := range iterate {
		if row, ok := it.(hostRowEntry); ok {
			m.selRef = row.ref
			m.hostList.Select(i)
			break
		}
	}
	m.refreshDetailList()
	m.refreshPasswordDetailList()
}

// toggleIncludeEditMode switches between merged read-only browse (all Include files) and
// editable single-file view (main path only). No-op if the file has no Include.
// Returns a command when the host list was rebuilt (needed if a filter is active).
func (m *Model) toggleIncludeEditMode() tea.Cmd {
	if !m.cfg.HasInclude {
		m.status = statusStyle.Render("No Include in this file.")
		return nil
	}
	if m.readOnly {
		data, err := os.ReadFile(m.path)
		if err != nil && !os.IsNotExist(err) {
			m.status = errStyle.Render("Read: " + err.Error())
			return nil
		}
		var cfg *scfg.Config
		if len(data) == 0 {
			cfg = &scfg.Config{}
		} else {
			cfg, err = scfg.Parse(strings.NewReader(string(data)))
			if err != nil {
				m.status = errStyle.Render("Parse: " + err.Error())
				return nil
			}
		}
		m.cfg = cfg
		m.readOnly = false
		m.dirty = false
		rb := m.rebuildHostList()
		m.layoutDetailPanes()
		m.resyncSelectionAfterStructureChange()
		m.status = fmt.Sprintf("Writable: only %s (included hosts hidden). Save writes this file. Press W or r for merged read-only browse.", m.path)
		return rb
	}
	if m.dirty {
		m.status = errStyle.Render("Write (w) or reload (r) before switching back to merged Include view.")
		return nil
	}
	data, err := os.ReadFile(m.path)
	if err != nil && !os.IsNotExist(err) {
		m.status = errStyle.Render("Read: " + err.Error())
		return nil
	}
	var cfg *scfg.Config
	if len(data) == 0 {
		cfg = &scfg.Config{}
	} else {
		cfg, err = scfg.Parse(strings.NewReader(string(data)))
		if err != nil {
			m.status = errStyle.Render("Parse: " + err.Error())
			return nil
		}
	}
	if !cfg.HasInclude {
		m.status = errStyle.Render("Include was removed on disk; staying in single-file view.")
		m.cfg = cfg
		m.readOnly = false
		rb := m.rebuildHostList()
		m.layoutDetailPanes()
		m.resyncSelectionAfterStructureChange()
		return rb
	}
	m.cfg = scfg.MergeIncludes(m.path, cfg)
	m.readOnly = true
	m.dirty = false
	rb := m.rebuildHostList()
	m.layoutDetailPanes()
	m.resyncSelectionAfterStructureChange()
	m.status = "Merged read-only browse (all Include files). W = edit main file only."
	return rb
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
	applyPaneChromeToList(&l)
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
	// One column for pane separator (│).
	return max(24, m.width-m.leftPaneWidth()-1)
}

func (m *Model) layoutDetailPanes() {
	ph := max(5, m.height-5)
	m.hostList.SetWidth(m.leftPaneWidth())
	m.hostList.SetHeight(ph)
	rw := m.rightPaneWidth()
	m.detailList.SetWidth(rw)
	m.detailList.SetHeight(ph)
	m.pwDetailList.SetWidth(rw)
	m.pwDetailList.SetHeight(ph)
}

// syncBrowsePreview updates selRef/pwSelIdx and the right-pane preview when the
// cursor moves in tree (browse) mode.
func (m *Model) syncBrowsePreview() {
	if m.mode != modeTree {
		return
	}
	switch it := m.hostList.SelectedItem().(type) {
	case passwordHostRowEntry:
		m.pwSelIdx = it.idx
		m.refreshPasswordDetailList()
	case hostRowEntry:
		if m.selRef == it.ref {
			return
		}
		m.selRef = it.ref
		m.pwSelIdx = -1
		m.refreshDetailList()
		m.refreshPasswordDetailList()
	case groupHeaderEntry:
		m.pwSelIdx = -1
		m.refreshPasswordDetailList()
	}
}

func (m *Model) syncTreeSelection() {
	// list.Select expects an index into VisibleItems(), not the underlying Items() slice.
	var items []list.Item
	if m.hostList.FilterState() != list.Unfiltered {
		items = m.hostList.VisibleItems()
	} else {
		items = m.hostList.Items()
	}
	for i, it := range items {
		if row, ok := it.(hostRowEntry); ok &&
			row.ref.InDefault == m.selRef.InDefault &&
			row.ref.GroupIdx == m.selRef.GroupIdx &&
			row.ref.HostIdx == m.selRef.HostIdx {
			m.hostList.Select(i)
			return
		}
	}
	for i, it := range items {
		if row, ok := it.(hostRowEntry); ok {
			m.hostList.Select(i)
			m.selRef = row.ref
			return
		}
	}
	if len(items) > 0 {
		m.hostList.Select(0)
	}
}

// hostConfig returns the *Config that ref belongs to.
func (m *Model) hostConfig(ref scfg.HostRef) *scfg.Config {
	if ref.FromMain && m.mainCfg != nil {
		return m.mainCfg
	}
	return m.cfg
}

// resolveHostAt returns the HostBlock for ref, dispatching to mainCfg when FromMain.
func (m *Model) resolveHostAt(ref scfg.HostRef) *scfg.HostBlock {
	return m.hostConfig(ref).HostAt(ref)
}

// resolveValidateRef validates ref against the correct config.
func (m *Model) resolveValidateRef(ref scfg.HostRef) error {
	return m.hostConfig(ref).ValidateRef(ref)
}

// mainRefBlocked returns true and sets a status message when the selected ref
// points into the read-only main ssh_config. Callers should abort the mutation.
func (m *Model) mainRefBlocked() bool {
	if !m.selRef.FromMain {
		return false
	}
	m.status = errStyle.Render("Read-only host from ssh_config — use Import (A menu) to move it to managed.")
	return true
}

func (m *Model) readOnlyBlocked() bool {
	if !m.readOnly {
		return false
	}
	m.status = "Read-only: merged Include view — Press W to edit the main file only, or ? for help."
	return true
}

// cycleBrowseMode rotates merged → openssh → password.
func (m *Model) cycleBrowseMode() tea.Cmd {
	switch m.browseMode {
	case appcfg.BrowseModeMerged:
		m.browseMode = appcfg.BrowseModeOpenSSH
	case appcfg.BrowseModeOpenSSH:
		m.browseMode = appcfg.BrowseModePassword
	default:
		m.browseMode = appcfg.BrowseModeMerged
	}
	m.status = "Browse: " + m.browseMode
	if m.appConfig != nil {
		m.appConfig.Hosts.BrowseMode = m.browseMode
		_ = appcfg.Save(m.appConfig)
	}
	rb := m.rebuildHostList()
	m.resyncSelectionAfterStructureChange()
	return rb
}

// enterEditMode acquires locks and switches to edit mode.
func (m *Model) enterEditMode() {
	if m.editMode {
		return
	}
	if m.sshHostsPath != "" {
		if err := lockfile.Acquire(m.sshHostsPath); err != nil {
			if lockfile.IsLocked(err) {
				m.status = errStyle.Render("Lock: " + err.Error())
				return
			}
		}
		m.lockedSSHHosts = true
	}
	if m.overlayPath != "" {
		if err := lockfile.Acquire(m.overlayPath); err != nil {
			if lockfile.IsLocked(err) {
				m.status = errStyle.Render("Overlay lock: " + err.Error())
				if m.lockedSSHHosts {
					_ = lockfile.Release(m.sshHostsPath)
					m.lockedSSHHosts = false
				}
				return
			}
		}
		m.lockedOverlay = true
	}
	m.editMode = true
}

// exitEditMode releases locks and returns to read mode.
func (m *Model) exitEditMode() {
	if !m.editMode {
		return
	}
	m.editMode = false
	if m.lockedSSHHosts {
		_ = lockfile.Release(m.sshHostsPath)
		m.lockedSSHHosts = false
	}
	if m.lockedOverlay {
		_ = lockfile.Release(m.overlayPath)
		m.lockedOverlay = false
	}
}

// editModeBlocked ensures edit mode is active before mutating; acquires locks.
// Also blocks when the selected host is from the read-only main ssh_config.
func (m *Model) editModeBlocked() bool {
	if m.readOnlyBlocked() {
		return true
	}
	if m.mainRefBlocked() {
		return true
	}
	if !m.editMode {
		m.enterEditMode()
	}
	return !m.editMode
}

func (m *Model) newDetailList() list.Model {
	rw := m.width
	rh := m.height - 3
	if m.mode == modeDetail || m.mode == modeTree {
		rw = m.rightPaneWidth()
		rh = max(5, m.height-5)
	}
	delegate := newDetailListDelegate()
	m.applyDetailListDelegateBG(&delegate)

	var items []list.Item
	var title string
	if err := m.resolveValidateRef(m.selRef); err != nil {
		title = detailTabTitle(m.detailTab) + " — (select a host)"
	} else {
		h := m.resolveHostAt(m.selRef)
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
		c := m.hostConfig(m.selRef)
		if !m.selRef.InDefault && m.selRef.GroupIdx >= 0 && m.selRef.GroupIdx < len(c.Groups) {
			title += " — " + c.Groups[m.selRef.GroupIdx].Name
		}
		if m.selRef.FromMain {
			title += " 🔒"
		}
	}
	l := list.New(items, delegate, rw, rh)
	l.Title = title
	l.Styles.Title = titleStyle
	l.SetShowStatusBar(true)
	l.SetFilteringEnabled(false)
	l.DisableQuitKeybindings()
	applyPaneChromeToList(&l)
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
	rw := max(1, m.rightPaneWidth()-2)
	boxStyle := m.rightPaneBoxStyle(rw)
	if m.resolveValidateRef(m.selRef) != nil {
		return boxStyle.Render(statusStyle.Render("No host selected."))
	}
	h := m.resolveHostAt(m.selRef)
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
	return boxStyle.Render(strings.TrimSuffix(b.String(), "\n"))
}

func firstStr(s, def string) string {
	if strings.TrimSpace(s) == "" {
		return def
	}
	return s
}

// rightPaneBoxStyle is the bordered panel style for the right pane (overview / inactive vs active detail).
func (m *Model) rightPaneBoxStyle(contentWidth int) lipgloss.Style {
	st := paneRightStyle.Copy().Border(panelBorder).Padding(0, 1).Width(contentWidth)
	if m.mode == modeDetail && !m.treePaneFocused && paneFillActive {
		return paneRightActiveStyle.Copy().Border(panelBorder).Padding(0, 1).Width(contentWidth)
	}
	return st
}

func (m *Model) detailListItemBG() lipgloss.TerminalColor {
	if !paneFillActive {
		return lipgloss.NoColor{}
	}
	if m.mode == modeDetail && !m.treePaneFocused {
		return paneRightActiveFill
	}
	return paneFill
}

func (m *Model) applyDetailListDelegateBG(d *list.DefaultDelegate) {
	bg := m.detailListItemBG()
	if !paneFillActive {
		return
	}
	d.Styles.NormalTitle = d.Styles.NormalTitle.Copy().Padding(0, 0, 0, 0).Background(bg)
	d.Styles.NormalDesc = d.Styles.NormalDesc.Copy().Padding(0, 0, 0, 0).Background(bg)
	d.Styles.DimmedTitle = d.Styles.DimmedTitle.Copy().Padding(0, 0, 0, 0).Background(bg)
	d.Styles.DimmedDesc = d.Styles.DimmedDesc.Copy().Padding(0, 0, 0, 0).Background(bg)
	d.Styles.SelectedTitle = lipgloss.NewStyle().Bold(true).Background(bg).Foreground(lipgloss.Color("255"))
	d.Styles.SelectedDesc = lipgloss.NewStyle().Bold(true).Background(bg).Foreground(lipgloss.Color("252"))
}

func (m *Model) toggleFoldAt(gh groupHeaderEntry) {
	if gh.groupIdx == -3 && m.mainCfg != nil {
		if strings.HasPrefix(gh.label, "🔒 unmanaged") {
			m.mainCfg.DefaultHostsCollapsed = !m.mainCfg.DefaultHostsCollapsed
		} else {
			for gi := range m.mainCfg.Groups {
				if strings.HasPrefix(gh.label, m.mainCfg.Groups[gi].Name) {
					m.mainCfg.Groups[gi].CollapsedByDefault = !m.mainCfg.Groups[gi].CollapsedByDefault
					break
				}
			}
		}
		return
	}
	if gh.defaultSec {
		m.cfg.DefaultHostsCollapsed = !m.cfg.DefaultHostsCollapsed
	} else if gh.groupIdx >= 0 && gh.groupIdx < len(m.cfg.Groups) {
		g := &m.cfg.Groups[gh.groupIdx]
		g.CollapsedByDefault = !g.CollapsedByDefault
	}
	if !m.readOnly {
		m.dirty = true
	}
}

func (m *Model) refreshDetailList() {
	m.detailList = m.newDetailList()
}

// pickKwEntryAsNewDirective starts value input for a new directive chosen from the keyword picker.
func (m *Model) pickKwEntryAsNewDirective(it kwEntry) (tea.Model, tea.Cmd) {
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

func (m *Model) openPicker() {
	m.editDirectiveIndex = -1
	entries := sshkeywords.Catalog
	items := make([]list.Item, len(entries))
	for i := range entries {
		items[i] = kwEntry(entries[i])
	}
	delegate := newDetailListDelegate()
	l := list.New(items, delegate, m.width, m.height-3)
	l.Title = "Add directive (type to filter)"
	l.Styles.Title = titleStyle
	l.SetShowStatusBar(true)
	l.SetFilteringEnabled(true)
	l.DisableQuitKeybindings()
	patchFilterAcceptKeys(&l)
	applyPaneChromeToList(&l)
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

// sshConnectReadyMsg is sent after the short "Connecting…" delay before spawning ssh.
type sshConnectReadyMsg struct{}

// actionMenuOuterWidth is the framed actions modal width.
func actionMenuOuterWidth(termW int) int {
	return min(56, termW-4)
}

// actionMenuListHeight is the bubbles list height: one line is reserved for the host subtitle
// rendered above the list body (outside the list, inside the frame).
func actionMenuListHeight(termH int) int {
	box := min(8, max(5, termH-4))
	return max(4, box-1)
}

// actionMenuTargetText is the host line shown under "Actions".
func (m *Model) actionMenuTargetText() string {
	if m.resolveValidateRef(m.selRef) != nil {
		return "Host: —"
	}
	return "Host: " + hostAlias(m.resolveHostAt(m.selRef))
}

func actionMenuTargetLineStyle() lipgloss.Style {
	if paneFillActive {
		return statusStyle.Copy().Background(paneFill)
	}
	return statusStyle
}

// renderActionMenuBox draws the actions list with the same pane fill as split view (border, padded
// lines) so list help and JoinVertical gaps are not on the terminal default background.
func (m *Model) renderActionMenuBox() string {
	lw := actionMenuOuterWidth(m.width)
	ll := m.actionMenuList
	titleBlock := ll.Styles.TitleBar.Render(ll.Styles.Title.Render("Actions"))
	targetLine := actionMenuTargetLineStyle().Render(m.actionMenuTargetText())
	raw := titleBlock + "\n" + targetLine + "\n" + ll.View()
	if paneFillActive {
		raw = padVisualLines(raw, lw)
	}
	box := lipgloss.NewStyle().Border(panelBorder).Padding(0, 1).Width(lw)
	if paneFillActive {
		box = box.Background(paneFill)
	}
	return box.Render(raw)
}

func (m *Model) openActionMenu(returnTo viewMode) {
	m.actionReturnMode = returnTo
	if row, ok := m.hostList.SelectedItem().(hostRowEntry); ok && returnTo == modeTree {
		m.selRef = row.ref
	}
	items := []list.Item{
		actionItem{id: "ssh", desc: "SSH session (uses first Host pattern)"},
		actionItem{id: "sftp", desc: "SFTP session"},
		actionItem{id: "copy", desc: "Copy ssh command"},
	}
	if m.selRef.FromMain {
		items = append(items, actionItem{id: "import", desc: "Import host to managed ssh_hosts file"})
	}
	items = append(items, actionItem{id: "cancel", desc: ""})
	d := list.NewDefaultDelegate()
	d.ShowDescription = true
	d.Styles.NormalTitle = styleWithPaneBG(d.Styles.NormalTitle)
	d.Styles.NormalDesc = styleWithPaneBG(d.Styles.NormalDesc)
	d.Styles.SelectedTitle = listSelectedTitleStyle
	d.Styles.SelectedDesc = listSelectedDescStyle
	lw := actionMenuOuterWidth(m.width)
	lh := actionMenuListHeight(m.height)
	l := list.New(items, d, lw, lh)
	l.Title = "Actions"
	l.Styles.Title = titleStyle
	l.SetShowTitle(false)
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)
	l.DisableQuitKeybindings()
	applyPaneChromeToList(&l)
	m.actionMenuList = l
	m.mode = modeActionMenu
}

func (m *Model) sshExecCmd() tea.Cmd {
	target := strings.TrimSpace(m.sshConnectPendingAlias)
	if target == "" {
		return func() tea.Msg {
			return shellProcDoneMsg{err: fmt.Errorf("empty ssh target")}
		}
	}

	// Check if this target matches a password overlay host.
	if m.overlayData != nil {
		if ph := m.overlayData.MatchHost(target); ph != nil {
			args := buildPasswordSSHArgs(ph)
			c := exec.Command("ssh", args...)
			c.Env = append(os.Environ(), ph.AskpassEnv()...)
			return tea.ExecProcess(c, func(err error) tea.Msg {
				return shellProcDoneMsg{err: err}
			})
		}
	}

	sftp := m.sshConnectPendingSFTP
	var c *exec.Cmd
	if sftp {
		c = exec.Command("sftp", target)
	} else {
		c = exec.Command("ssh", target)
	}
	return tea.ExecProcess(c, func(err error) tea.Msg {
		return shellProcDoneMsg{err: err}
	})
}

// startSSHConnectPending shows a short gate before ssh/sftp; Esc cancels (tick still arrives but is ignored).
func (m *Model) startSSHConnectPending(returnMode viewMode, target, note string, sftp bool) (*Model, tea.Cmd) {
	m.sshConnectReturnMode = returnMode
	m.sshConnectPendingCancelled = false
	m.sshConnectPendingAlias = strings.TrimSpace(target)
	m.sshConnectPendingNote = note
	m.sshConnectPendingSFTP = sftp
	m.mode = modeSSHConnectPending
	return m, tea.Tick(500*time.Millisecond, func(time.Time) tea.Msg { return sshConnectReadyMsg{} })
}

func (m *Model) beginInteractiveSSH(returnMode viewMode, post sshPostWildKind) (*Model, tea.Cmd) {
	h := m.resolveHostAt(m.selRef)
	plan := sshConnectPlanFromHost(h)
	if plan.Invalid {
		return m, func() tea.Msg {
			return shellProcDoneMsg{err: fmt.Errorf("need at least one Host pattern")}
		}
	}
	note := sshConnectMultiPatternNote(plan)
	if plan.NeedWildcard {
		return m.enterSSHWildcardPrompt(returnMode, plan, post, note)
	}
	if post == postWildCopyCmd {
		return m.finishSSHCopy(plan.DirectTarget)
	}
	sftp := post == postWildConnectSFTP
	return m.startSSHConnectPending(returnMode, plan.DirectTarget, note, sftp)
}

func (m *Model) enterSSHWildcardPrompt(returnMode viewMode, plan sshConnectPlan, post sshPostWildKind, note string) (*Model, tea.Cmd) {
	m.sshConnectReturnMode = returnMode
	m.sshPostWildKind = post
	m.sshConnectWildKind = plan.WildKind
	m.sshConnectWildPrefix = plan.LitPrefix
	m.sshConnectWildSuffix = plan.LitSuffix
	m.sshConnectWildPattern = plan.FirstPattern
	m.sshConnectPendingNote = note
	m.valueInput.SetValue("")
	m.valueInput.Placeholder = sshWildcardValuePlaceholder(plan.WildKind)
	m.valueInput.Width = max(12, min(50, m.width-10))
	m.valueInput.Focus()
	m.mode = modeSSHConnectWildcardInput
	return m, textinput.Blink
}

func (m *Model) submitSSHWildcardConnect() (tea.Model, tea.Cmd) {
	target := composeSSHWildcardTarget(m.sshConnectWildKind, m.sshConnectWildPrefix, m.sshConnectWildSuffix, m.valueInput.Value())
	if strings.TrimSpace(target) == "" {
		m.status = errStyle.Render("Enter a hostname.")
		return m, nil
	}
	switch m.sshPostWildKind {
	case postWildCopyCmd:
		return m.finishSSHCopy(target)
	case postWildConnectSSH:
		return m.startSSHConnectPending(m.sshConnectReturnMode, target, m.sshConnectPendingNote, false)
	case postWildConnectSFTP:
		return m.startSSHConnectPending(m.sshConnectReturnMode, target, m.sshConnectPendingNote, true)
	default:
		m.status = errStyle.Render("Internal: unknown SSH action.")
		return m, nil
	}
}

func (m *Model) finishSSHCopy(target string) (*Model, tea.Cmd) {
	cmdStr := "ssh " + strings.TrimSpace(target)
	if err := clipboard.WriteAll(cmdStr); err != nil {
		m.status = errStyle.Render(err.Error())
	} else {
		m.status = "Copied: " + cmdStr
	}
	m.sshConnectPendingNote = ""
	m.mode = m.actionReturnMode
	if m.mode == modeDetail {
		m.layoutDetailPanes()
		m.refreshDetailList()
	} else if m.mode == modeTree {
		m.layoutDetailPanes()
		m.refreshDetailList()
	}
	return m, nil
}

func (m *Model) restoreSSHConnectReturnLayout() {
	if m.sshConnectReturnMode == modeDetail {
		m.layoutDetailPanes()
		m.refreshDetailList()
	} else if m.sshConnectReturnMode == modeTree {
		m.layoutDetailPanes()
		m.refreshDetailList()
	} else if m.sshConnectReturnMode == modeActionMenu {
		m.actionMenuList.SetWidth(actionMenuOuterWidth(m.width))
		m.actionMenuList.SetHeight(actionMenuListHeight(m.height))
	}
}

func (m *Model) reloadFromDisk() tea.Cmd {
	data, err := os.ReadFile(m.path)
	if err != nil && !os.IsNotExist(err) {
		m.status = errStyle.Render("Reload: " + err.Error())
		return nil
	}
	var cfg *scfg.Config
	if len(data) == 0 {
		cfg = &scfg.Config{}
	} else {
		cfg, err = scfg.Parse(strings.NewReader(string(data)))
		if err != nil {
			m.status = errStyle.Render("Reload parse: " + err.Error())
			return nil
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
	m.reloadMainCfg()
	rb := m.rebuildHostList()
	m.layoutDetailPanes()
	m.syncBrowsePreview()
	m.status = "Reloaded from disk."
	return rb
}

// reloadMainCfg refreshes the read-only mainCfg snapshot from disk.
func (m *Model) reloadMainCfg() {
	if m.mainSSHConfigPath == "" || m.mainSSHConfigPath == m.path {
		m.mainCfg = nil
		return
	}
	data, err := os.ReadFile(m.mainSSHConfigPath)
	if err != nil || len(data) == 0 {
		m.mainCfg = nil
		return
	}
	parsed, err := scfg.Parse(strings.NewReader(string(data)))
	if err != nil {
		m.mainCfg = nil
		return
	}
	baseDir := filepath.Dir(m.mainSSHConfigPath)
	m.mainCfg = scfg.StripBridgeIncludes(parsed, m.sshHostsPath, baseDir)
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		switch m.mode {
		case modeTree, modeDetail:
			m.layoutDetailPanes()
			rb := m.rebuildHostList()
			m.detailList = m.newDetailList()
			m.refreshPasswordDetailList()
			return m, rb
		default:
			m.hostList.SetWidth(msg.Width)
			m.hostList.SetHeight(max(6, msg.Height-4))
		}
		if m.mode == modePicker {
			m.pickerList.SetWidth(m.rightPaneWidth())
			m.pickerList.SetHeight(max(6, m.height-5))
		}
		if m.mode == modeGroupPicker {
			m.groupPickerList.SetWidth(msg.Width)
			m.groupPickerList.SetHeight(max(6, msg.Height-3))
		}
		if m.mode == modeActionMenu {
			m.actionMenuList.SetWidth(actionMenuOuterWidth(msg.Width))
			m.actionMenuList.SetHeight(actionMenuListHeight(msg.Height))
		}
		if m.mode == modeAppCfgView {
			m.layoutAppCfgViewport()
		}
		if m.mode == modeHelp {
			m.layoutHelpViewport()
		}
		if m.mode == modeSSHConnectPending || m.mode == modeSSHConnectWildcardInput {
			m.layoutDetailPanes()
		}
		if m.mode == modePasswordDetail || m.mode == modePasswordDetailEdit {
			m.layoutDetailPanes()
			m.refreshPasswordDetailList()
		}
		return m, nil

	// Bubbles schedules filter reflow asynchronously; the message must reach the list that
	// requested it. Otherwise picker filtering updates the host list (and the picker never narrows).
	case list.FilterMatchesMsg:
		switch m.mode {
		case modePicker:
			var cmd tea.Cmd
			m.pickerList, cmd = m.pickerList.Update(msg)
			return m, cmd
		case modePasswordDetail:
			var cmd tea.Cmd
			m.pwDetailList, cmd = m.pwDetailList.Update(msg)
			return m, cmd
		case modeTree, modeDetail:
			return m, m.updateHostList(msg)
		default:
			return m, nil
		}

	case sshConnectReadyMsg:
		if m.mode != modeSSHConnectPending {
			m.sshConnectPendingCancelled = false
			return m, nil
		}
		if m.sshConnectPendingCancelled {
			m.sshConnectPendingCancelled = false
			m.status = ""
			m.mode = m.sshConnectReturnMode
			m.restoreSSHConnectReturnLayout()
			return m, nil
		}
		m.mode = m.sshConnectReturnMode
		m.restoreSSHConnectReturnLayout()
		return m, m.sshExecCmd()

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

	case appConfigEditorFinishedMsg:
		return m.handleAppConfigEditorFinished(msg)

	case appConfigEditorErrMsg:
		m.status = errStyle.Render(msg.err.Error())
		return m, nil

	case tea.KeyMsg:
		switch m.mode {
		case modeHelp:
			if key.Matches(msg, key.NewBinding(key.WithKeys("ctrl+c"))) {
				return m, tea.Quit
			}
			if key.Matches(msg, key.NewBinding(key.WithKeys("esc"))) {
				m.mode = m.helpReturnMode
				m.layoutDetailPanes()
				if m.mode == modeDetail {
					m.refreshDetailList()
				}
				return m, nil
			}
			var vcmd tea.Cmd
			m.helpViewport, vcmd = m.helpViewport.Update(msg)
			return m, vcmd

		case modeExportWizard:
			return m.handleExportWizard(msg)

		case modeNewHostTypePicker:
			switch msg.String() {
			case "o", "O", "enter":
				return m, m.startNewOpenSSHHost()
			case "p", "P":
				return m, m.startNewPasswordHost()
			case "esc":
				m.mode = modeTree
				return m, nil
			}
			return m, nil

		case modeNewGroupTypePicker:
			switch msg.String() {
			case "o", "O", "enter":
				return m, m.startNewOpenSSHGroup()
			case "p", "P":
				return m, m.startNewPasswordGroup()
			case "esc":
				m.mode = modeTree
				return m, nil
			}
			return m, nil

		case modePasswordDetail:
			switch msg.String() {
			case "esc":
				if m.editMode {
					m.exitEditMode()
					m.status = ""
					return m, nil
				}
				m.pwSelIdx = -1
				m.mode = modeTree
				m.layoutDetailPanes()
				return m, nil
			case "w":
				if err := m.saveOverlay(); err != nil {
					m.status = errStyle.Render("Save overlay: " + err.Error())
				} else {
					m.status = "Password overlay saved."
				}
				return m, nil
			case "s":
				if m.overlayData != nil && m.pwSelIdx >= 0 {
					return m.connectPasswordHost(m.pwSelIdx, modePasswordDetail)
				}
			case "e", "enter":
				if it, ok := m.pwDetailList.SelectedItem().(pwFieldEntry); ok {
					if m.editModeBlocked() {
						return m, nil
					}
					m.returnAfterInput = modePasswordDetail
					m.startPasswordFieldEdit(it)
					return m, textinput.Blink
				}
			case "x", "X":
				if m.editModeBlocked() {
					return m, nil
				}
				if m.overlayData == nil || m.pwSelIdx < 0 || m.pwSelIdx >= len(m.overlayData.PasswordHosts) {
					return m, nil
				}
				m.pendingDeletePwIdx = m.pwSelIdx
				m.confirmReturnMode = modePasswordDetail
				m.mode = modeConfirmDeleteHost
				m.status = fmt.Sprintf("Delete password host %q? [y/N]", m.overlayData.PasswordHosts[m.pwSelIdx].Title())
				return m, nil
			}
			var pwcmd tea.Cmd
			m.pwDetailList, pwcmd = m.pwDetailList.Update(msg)
			return m, pwcmd

		case modeSSHConnectPending:
			switch msg.String() {
			case "esc":
				m.sshConnectPendingCancelled = true
				m.mode = m.sshConnectReturnMode
				m.status = ""
				m.restoreSSHConnectReturnLayout()
				return m, nil
			}
			return m, nil

		case modeSSHConnectWildcardInput:
			switch msg.String() {
			case "esc":
				m.sshConnectPendingNote = ""
				m.sshPostWildKind = postWildNone
				m.mode = m.sshConnectReturnMode
				m.status = ""
				m.restoreSSHConnectReturnLayout()
				return m, nil
			case "enter":
				return m.submitSSHWildcardConnect()
			}
			var wcmd tea.Cmd
			m.valueInput, wcmd = m.valueInput.Update(msg)
			return m, wcmd

		case modeConfirmDeleteHost:
			switch msg.String() {
			case "y", "Y":
				if m.readOnly {
					m.status = errStyle.Render("Read-only.")
					m.pendingDeletePwIdx = -1
					m.mode = m.confirmReturnMode
					if m.confirmReturnMode == modeDetail {
						m.layoutDetailPanes()
						m.refreshDetailList()
					} else if m.confirmReturnMode == modeTree {
						m.layoutDetailPanes()
						m.refreshDetailList()
					} else if m.confirmReturnMode == modePasswordDetail {
						m.layoutDetailPanes()
						m.refreshPasswordDetailList()
					}
					return m, nil
				}
				if m.pendingDeletePwIdx >= 0 {
					if m.editModeBlocked() {
						m.pendingDeletePwIdx = -1
						return m, nil
					}
					if m.overlayData == nil || m.pendingDeletePwIdx >= len(m.overlayData.PasswordHosts) {
						m.status = errStyle.Render("Invalid password host selection.")
						m.pendingDeletePwIdx = -1
						m.mode = m.confirmReturnMode
						return m, nil
					}
					idx := m.pendingDeletePwIdx
					m.overlayData.PasswordHosts = append(m.overlayData.PasswordHosts[:idx], m.overlayData.PasswordHosts[idx+1:]...)
					m.pendingDeletePwIdx = -1
					if err := m.saveOverlay(); err != nil {
						m.status = errStyle.Render("Save overlay: " + err.Error())
						m.mode = m.confirmReturnMode
						return m, nil
					}
					ret := m.confirmReturnMode
					m.mode = ret
					m.layoutDetailPanes()
					rb := m.rebuildHostList()
					if ret == modePasswordDetail {
						if len(m.overlayData.PasswordHosts) == 0 {
							m.pwSelIdx = -1
							m.mode = modeTree
							m.refreshPasswordDetailList()
						} else {
							if m.pwSelIdx > idx {
								m.pwSelIdx--
							}
							if m.pwSelIdx >= len(m.overlayData.PasswordHosts) {
								m.pwSelIdx = len(m.overlayData.PasswordHosts) - 1
							}
							m.refreshPasswordDetailList()
						}
					} else {
						m.pwSelIdx = -1
						m.syncBrowsePreview()
						m.refreshDetailList()
						m.refreshPasswordDetailList()
					}
					m.status = "Password host removed."
					return m, rb
				}
				if m.selRef.FromMain {
					m.status = errStyle.Render("Cannot delete host from main ssh_config — use Import first.")
					m.mode = m.confirmReturnMode
					return m, nil
				}
				m.cfg.DeleteHost(m.selRef)
				m.dirty = true
				m.pendingDeletePwIdx = -1
				m.mode = modeTree
				m.layoutDetailPanes()
				rb := m.rebuildHostList()
				m.syncBrowsePreview()
				m.refreshDetailList()
				m.status = "Host deleted."
				return m, rb
			case "n", "N", "esc":
				m.status = ""
				m.pendingDeletePwIdx = -1
				m.mode = m.confirmReturnMode
				if m.confirmReturnMode == modeDetail {
					m.layoutDetailPanes()
					m.refreshDetailList()
				} else if m.confirmReturnMode == modeTree {
					m.layoutDetailPanes()
					m.refreshDetailList()
				} else if m.confirmReturnMode == modePasswordDetail {
					m.layoutDetailPanes()
					m.refreshPasswordDetailList()
				}
			}
			return m, nil

		case modeConfirmDeleteGroup:
			switch msg.String() {
			case "y", "Y":
				if m.readOnly {
					m.status = errStyle.Render("Read-only.")
					m.pendingDeleteGroupName = ""
					m.pendingDeleteOverlayGroup = false
					m.mode = modeTree
					m.layoutDetailPanes()
					return m, nil
				}
				if m.pendingDeleteOverlayGroup {
					if m.editModeBlocked() {
						m.pendingDeleteGroupName = ""
						m.pendingDeleteOverlayGroup = false
						return m, nil
					}
					if m.overlayData == nil {
						m.status = errStyle.Render("No password overlay.")
						m.pendingDeleteGroupName = ""
						m.pendingDeleteOverlayGroup = false
						m.mode = modeTree
						return m, nil
					}
					name := m.pendingDeleteGroupName
					if err := m.overlayData.DeletePasswordGroup(name); err != nil {
						m.status = errStyle.Render(err.Error())
					} else if err := m.saveOverlay(); err != nil {
						m.status = errStyle.Render("Save overlay: " + err.Error())
					} else {
						m.status = fmt.Sprintf("Password group %q removed (hosts ungrouped).", name)
					}
					m.pendingDeleteGroupName = ""
					m.pendingDeleteOverlayGroup = false
					m.mode = modeTree
					m.layoutDetailPanes()
					rb := m.rebuildHostList()
					m.refreshDetailList()
					m.refreshPasswordDetailList()
					return m, rb
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
				m.pendingDeleteOverlayGroup = false
				m.mode = modeTree
				m.layoutDetailPanes()
				rb := m.rebuildHostList()
				m.refreshDetailList()
				m.refreshPasswordDetailList()
				return m, rb
			case "n", "N", "esc":
				m.status = ""
				m.pendingDeleteGroupName = ""
				m.pendingDeleteOverlayGroup = false
				m.mode = modeTree
				m.layoutDetailPanes()
			}
			return m, nil

		case modeConfirmImport:
			switch msg.String() {
			case "y", "Y", "enter":
				if m.mainCfg != nil && m.mainCfg.ValidateRef(m.pendingImportRef) == nil {
					m.mainCfg.DeleteHost(m.pendingImportRef)
					if err := m.saveMainConfig(); err != nil {
						m.status = errStyle.Render("Remove from main: " + err.Error())
					} else {
						m.reloadMainCfg()
						m.status = "Host imported and removed from main ssh_config."
					}
				}
				m.mode = modeTree
				rb := m.rebuildHostList()
				m.layoutDetailPanes()
				m.syncTreeSelection()
				m.refreshDetailList()
				return m, rb
			case "n", "N", "esc":
				m.status = "Host imported (kept in main ssh_config too)."
				m.mode = modeTree
				rb := m.rebuildHostList()
				m.layoutDetailPanes()
				m.syncTreeSelection()
				m.refreshDetailList()
				return m, rb
			}
			return m, nil

		case modeGroupPicker:
			return m.updateGroupPicker(msg)

		case modeActionMenu:
			return m.updateActionMenu(msg)

		case modeInputDirectiveValue, modeInputCustomKey, modeInputNewHost, modeInputNewPasswordHost, modeInputDuplicateHost,
			modeInputNewGroup, modeInputNewPasswordGroup, modeInputRenameGroup, modeInputGroupDesc, modeInputHostMeta, modeInputPasswordField:
			switch msg.String() {
			case "esc":
				m.editDirectiveIndex = -1
				m.pendingDirectiveKey = ""
				m.editGroupIdx = -1
				m.pwEditFieldKey = ""
				m.mode = m.returnAfterInput
				if m.returnAfterInput == modeDetail {
					if m.resolveValidateRef(m.selRef) == nil {
						m.refreshDetailList()
					}
				}
				if m.returnAfterInput == modePasswordDetail {
					m.refreshPasswordDetailList()
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

		if m.mode == modeAppCfgView {
			switch msg.String() {
			case "esc", "q":
				m.mode = m.appCfgReturnMode
				if m.mode == modeDetail {
					m.layoutDetailPanes()
					m.refreshDetailList()
				} else {
					m.layoutDetailPanes()
				}
				return m, nil
			}
			var cmd tea.Cmd
			m.appCfgViewport, cmd = m.appCfgViewport.Update(msg)
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

	if m.mode == modeAppCfgView {
		var cmd tea.Cmd
		m.appCfgViewport, cmd = m.appCfgViewport.Update(msg)
		return m, cmd
	}

	if m.mode == modeHelp {
		var cmd tea.Cmd
		m.helpViewport, cmd = m.helpViewport.Update(msg)
		return m, cmd
	}

	cmd := m.updateHostList(msg)
	return m, cmd
}

func (m *Model) submitInput() (tea.Model, tea.Cmd) {
	switch m.mode {
	case modeInputDirectiveValue:
		val := strings.TrimSpace(m.valueInput.Value())
		h := m.resolveHostAt(m.selRef)
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
		rb := m.rebuildHostList()
		m.selRef = scfg.HostRef{InDefault: true, HostIdx: len(m.cfg.DefaultHosts) - 1}
		m.mode = modeDetail
		m.refreshDetailList()
		m.status = "New host created."
		return m, rb

	case modeInputNewPasswordHost:
		host := strings.TrimSpace(m.valueInput.Value())
		if host == "" {
			m.status = errStyle.Render("Enter the hostname or IP for ssh (patterns default to this; edit in detail).")
			return m, nil
		}
		if m.overlayData == nil {
			m.overlayData = &overlay.File{Version: 1}
		}
		pat := []string{host}
		m.overlayData.PasswordHosts = append(m.overlayData.PasswordHosts, overlay.PasswordHost{
			Group:    m.selectedPwGroup(),
			Hostname: host,
			Patterns: pat,
		})
		if err := m.saveOverlay(); err != nil {
			m.status = errStyle.Render("Save overlay: " + err.Error())
			m.mode = modeTree
			return m, nil
		}
		newIdx := len(m.overlayData.PasswordHosts) - 1
		rb := m.rebuildHostList()
		m.openPasswordDetail(newIdx)
		m.layoutDetailPanes()
		m.status = "New password host created — e/enter to edit patterns, askpass, …"
		return m, rb

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
		rb := m.rebuildHostList()
		// Move selection to duplicated block (inserted after current)
		ref := m.selRef
		ref.HostIdx++
		if err := m.cfg.ValidateRef(ref); err == nil {
			m.selRef = ref
		}
		m.mode = modeDetail
		m.refreshDetailList()
		m.status = "Host duplicated."
		return m, rb

	case modeInputNewGroup:
		name := strings.TrimSpace(m.valueInput.Value())
		if err := m.cfg.AddGroup(name); err != nil {
			m.status = errStyle.Render(err.Error())
			return m, nil
		}
		m.dirty = true
		m.mode = modeTree
		m.layoutDetailPanes()
		rb := m.rebuildHostList()
		m.refreshDetailList()
		m.status = fmt.Sprintf("Created group %q.", name)
		return m, rb

	case modeInputNewPasswordGroup:
		name := strings.TrimSpace(m.valueInput.Value())
		if m.overlayData == nil {
			m.overlayData = &overlay.File{Version: 1}
		}
		if err := m.overlayData.AddGroup(name); err != nil {
			m.status = errStyle.Render(err.Error())
			return m, nil
		}
		if err := m.saveOverlay(); err != nil {
			m.status = errStyle.Render("Save overlay: " + err.Error())
			m.mode = modeTree
			return m, nil
		}
		m.mode = modeTree
		m.layoutDetailPanes()
		rb := m.rebuildHostList()
		m.status = fmt.Sprintf("Created password group %q.", name)
		return m, rb

	case modeInputPasswordField:
		if m.overlayData == nil || m.pwSelIdx < 0 || m.pwSelIdx >= len(m.overlayData.PasswordHosts) {
			m.status = errStyle.Render("No password host selected.")
			m.mode = m.returnAfterInput
			return m, nil
		}
		ph := &m.overlayData.PasswordHosts[m.pwSelIdx]
		v := strings.TrimSpace(m.valueInput.Value())
		switch m.pwEditFieldKey {
		case "hostname":
			if v == "" {
				m.status = errStyle.Render("Hostname required.")
				return m, nil
			}
			ph.Hostname = v
		case "patterns":
			pats := strings.Fields(m.valueInput.Value())
			if len(pats) == 0 {
				m.status = errStyle.Render("At least one pattern (space-separated).")
				return m, nil
			}
			ph.Patterns = pats
		case "user":
			ph.User = v
		case "port":
			if v == "" {
				ph.Port = 0
			} else {
				p, err := strconv.Atoi(v)
				if err != nil || p < 0 || p > 65535 {
					m.status = errStyle.Render("Invalid port (0–65535 or empty for 22).")
					return m, nil
				}
				ph.Port = p
			}
		case "askpass":
			ph.Askpass = strings.TrimSpace(m.valueInput.Value())
		case "askpass_require":
			ph.AskpassRequire = strings.TrimSpace(m.valueInput.Value())
		case "display":
			ph.Display = strings.TrimSpace(m.valueInput.Value())
		case "group":
			ph.Group = v
		default:
			m.status = errStyle.Render("Unknown field.")
			m.mode = m.returnAfterInput
			m.pwEditFieldKey = ""
			return m, nil
		}
		if err := m.saveOverlay(); err != nil {
			m.status = errStyle.Render("Save overlay: " + err.Error())
			return m, nil
		}
		m.mode = m.returnAfterInput
		m.pwEditFieldKey = ""
		m.refreshPasswordDetailList()
		m.layoutDetailPanes()
		rb := m.rebuildHostList()
		m.status = "Password host updated."
		return m, rb

	case modeInputRenameGroup:
		name := strings.TrimSpace(m.valueInput.Value())
		if err := m.cfg.RenameGroup(m.editGroupIdx, name); err != nil {
			m.status = errStyle.Render(err.Error())
			return m, nil
		}
		m.dirty = true
		rb := m.rebuildHostList()
		m.editGroupIdx = -1
		m.mode = modeDetail
		if m.resolveValidateRef(m.selRef) == nil {
			m.refreshDetailList()
		}
		m.status = fmt.Sprintf("Renamed group to %q.", name)
		return m, rb

	case modeInputGroupDesc:
		if err := m.cfg.SetGroupDescription(m.editGroupIdx, m.valueInput.Value()); err != nil {
			m.status = errStyle.Render(err.Error())
			return m, nil
		}
		m.dirty = true
		rb := m.rebuildHostList()
		m.editGroupIdx = -1
		m.mode = modeDetail
		if m.resolveValidateRef(m.selRef) == nil {
			m.refreshDetailList()
		}
		m.status = "Group description updated."
		return m, rb

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
		h := m.resolveHostAt(m.selRef)
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
		if tryFilterListArrowNav(&m.hostList, msg) {
			m.syncBrowsePreview()
			return m, nil
		}
		if isEnterKey(msg) {
			cmd := m.updateHostList(msg)
			switch it := m.hostList.SelectedItem().(type) {
			case hostRowEntry:
				m.openDetail(it.ref)
			case passwordHostRowEntry:
				m.openPasswordDetail(it.idx)
			}
			m.syncBrowsePreview()
			return m, cmd
		}
		cmd := m.updateHostList(msg)
		m.syncBrowsePreview()
		return m, cmd
	}

	switch {
	case key.Matches(msg, key.NewBinding(key.WithKeys("q", "Q"))):
		return m, tea.Quit

	case msg.String() == "z":
		if gh, ok := m.hostList.SelectedItem().(groupHeaderEntry); ok {
			m.toggleFoldAt(gh)
			rb := m.rebuildHostList()
			m.syncBrowsePreview()
			return m, rb
		}

	case msg.String() == "B":
		return m, m.cycleBrowseMode()

	case msg.String() == "I":
		m.mode = modeExportWizard
		m.wizardStep = 0
		m.wizardPrepareStep()
		return m, textinput.Blink

	case msg.String() == "$":
		return m, m.openAppConfigEditor()

	case msg.String() == "&":
		return m.openAppConfigView(modeTree)

	case msg.String() == "?":
		m.openHelp(modeTree)
		return m, nil

	case msg.String() == "w":
		if err := m.save(); err != nil {
			m.status = errStyle.Render("Save: " + err.Error())
		} else {
			m.status = "Saved."
		}
		return m, nil

	case msg.String() == "s":
		if row, ok := m.hostList.SelectedItem().(hostRowEntry); ok {
			m.selRef = row.ref
			if m.resolveValidateRef(m.selRef) == nil {
				return m.beginInteractiveSSH(modeTree, postWildConnectSSH)
			}
		}
		if pw, ok := m.hostList.SelectedItem().(passwordHostRowEntry); ok {
			return m.connectPasswordHost(pw.idx, modeTree)
		}

	case msg.String() == "r":
		return m, m.reloadFromDisk()

	case msg.String() == "W":
		return m, m.toggleIncludeEditMode()

	case msg.String() == "A":
		if _, ok := m.hostList.SelectedItem().(hostRowEntry); ok {
			m.openActionMenu(modeTree)
			m.actionMenuList.SetWidth(actionMenuOuterWidth(m.width))
			m.actionMenuList.SetHeight(actionMenuListHeight(m.height))
			return m, nil
		}

	case msg.String() == "n":
		if m.editModeBlocked() {
			return m, nil
		}
		switch m.browseMode {
		case appcfg.BrowseModeOpenSSH:
			return m, m.startNewOpenSSHHost()
		case appcfg.BrowseModePassword:
			return m, m.startNewPasswordHost()
		default:
			m.mode = modeNewHostTypePicker
			return m, nil
		}

	case msg.String() == "v":
		if m.editModeBlocked() {
			return m, nil
		}
		return m, m.rawEditorCmd()

	case msg.String() == "enter":
		if it, ok := m.hostList.SelectedItem().(hostRowEntry); ok {
			m.openDetail(it.ref)
			return m, nil
		}
		if pw, ok := m.hostList.SelectedItem().(passwordHostRowEntry); ok {
			m.openPasswordDetail(pw.idx)
			return m, nil
		}
		return m, nil

	case msg.String() == "x":
		if m.editModeBlocked() {
			return m, nil
		}
		if row, ok := m.hostList.SelectedItem().(hostRowEntry); ok {
			m.pendingDeletePwIdx = -1
			m.selRef = row.ref
			m.confirmReturnMode = modeTree
			m.mode = modeConfirmDeleteHost
			m.status = fmt.Sprintf("Delete host %q? [y/N]", hostTitle(m.resolveHostAt(m.selRef)))
			return m, nil
		}
		if pw, ok := m.hostList.SelectedItem().(passwordHostRowEntry); ok {
			if m.overlayData == nil || pw.idx < 0 || pw.idx >= len(m.overlayData.PasswordHosts) {
				return m, nil
			}
			m.pendingDeletePwIdx = pw.idx
			m.confirmReturnMode = modeTree
			m.mode = modeConfirmDeleteHost
			m.status = fmt.Sprintf("Delete password host %q? [y/N]", m.overlayData.PasswordHosts[pw.idx].Title())
			return m, nil
		}

	case msg.String() == "g":
		if m.editModeBlocked() {
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
		if m.editModeBlocked() {
			return m, nil
		}
		switch m.browseMode {
		case appcfg.BrowseModeOpenSSH:
			return m, m.startNewOpenSSHGroup()
		case appcfg.BrowseModePassword:
			return m, m.startNewPasswordGroup()
		default:
			m.mode = modeNewGroupTypePicker
			return m, nil
		}

	case msg.String() == "D":
		if m.editModeBlocked() {
			return m, nil
		}
		if gh, ok := m.hostList.SelectedItem().(groupHeaderEntry); ok {
			if gh.label == "(default)" {
				m.status = errStyle.Render("(default) cannot be deleted.")
				return m, nil
			}
			if gh.label == "(password)" {
				m.status = errStyle.Render("(password) section cannot be deleted here.")
				return m, nil
			}
			var delName string
			overlayGroup := false
			switch {
			case gh.groupIdx == -2 && gh.pwGroup != "":
				overlayGroup = true
				delName = gh.pwGroup
			case gh.groupIdx >= 0 && gh.groupIdx < len(m.cfg.Groups):
				delName = m.cfg.Groups[gh.groupIdx].Name
			default:
				m.status = errStyle.Render("Cannot delete this group header.")
				return m, nil
			}
			m.pendingDeleteOverlayGroup = overlayGroup
			m.pendingDeleteGroupName = delName
			m.mode = modeConfirmDeleteGroup
			if overlayGroup {
				m.status = fmt.Sprintf("Delete password group %q (hosts become ungrouped)? [y/N]", delName)
			} else {
				m.status = fmt.Sprintf("Delete group %q and move its hosts to (default)? [y/N]", delName)
			}
			return m, nil
		}
	}

	cmd := m.updateHostList(msg)
	m.syncBrowsePreview()
	return m, cmd
}

func (m *Model) updateDetail(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if key.Matches(msg, key.NewBinding(key.WithKeys("ctrl+c"))) {
		return m, tea.Quit
	}
	if msg.String() == "W" {
		rb := m.toggleIncludeEditMode()
		m.layoutDetailPanes()
		m.refreshDetailList()
		return m, rb
	}

	if m.treePaneFocused {
		if m.hostList.SettingFilter() {
			if tryFilterListArrowNav(&m.hostList, msg) {
				switch it := m.hostList.SelectedItem().(type) {
				case hostRowEntry:
					m.selRef = it.ref
					m.refreshDetailList()
				case passwordHostRowEntry:
					m.pwSelIdx = it.idx
				}
				return m, nil
			}
			if isEnterKey(msg) {
				cmd := m.updateHostList(msg)
				switch it := m.hostList.SelectedItem().(type) {
				case hostRowEntry:
					m.selRef = it.ref
					m.detailTab = detailTabAll
					m.treePaneFocused = false
					m.refreshDetailList()
					m.layoutDetailPanes()
				case passwordHostRowEntry:
					m.openPasswordDetail(it.idx)
				}
				return m, cmd
			}
			cmd := m.updateHostList(msg)
			return m, cmd
		}
		switch {
		case msg.String() == "tab":
			m.treePaneFocused = false
			m.refreshDetailList()
			return m, nil
		case msg.String() == "z":
			if gh, ok := m.hostList.SelectedItem().(groupHeaderEntry); ok {
				m.toggleFoldAt(gh)
				return m, m.rebuildHostList()
			}
		case msg.String() == "w":
			if err := m.save(); err != nil {
				m.status = errStyle.Render("Save: " + err.Error())
			} else {
				m.status = "Saved."
			}
			return m, nil
		case msg.String() == "s":
			if row, ok := m.hostList.SelectedItem().(hostRowEntry); ok {
				m.selRef = row.ref
				if m.resolveValidateRef(m.selRef) == nil {
					return m.beginInteractiveSSH(modeDetail, postWildConnectSSH)
				}
			}
			if pw, ok := m.hostList.SelectedItem().(passwordHostRowEntry); ok {
				return m.connectPasswordHost(pw.idx, modeDetail)
			}
		case msg.String() == "$":
			return m, m.openAppConfigEditor()
		case msg.String() == "&":
			return m.openAppConfigView(modeDetail)
		case msg.String() == "enter":
			switch it := m.hostList.SelectedItem().(type) {
			case hostRowEntry:
				m.selRef = it.ref
				m.detailTab = detailTabAll
				m.treePaneFocused = false
				m.refreshDetailList()
				m.layoutDetailPanes()
			case passwordHostRowEntry:
				m.openPasswordDetail(it.idx)
			}
			return m, nil
		case key.Matches(msg, key.NewBinding(key.WithKeys("esc"))):
			m.mode = modeTree
			m.layoutDetailPanes()
			return m, m.rebuildHostList()
		}
		cmd := m.updateHostList(msg)
		m.syncBrowsePreview()
		return m, cmd
	}

	switch {
	case msg.String() == "tab":
		m.treePaneFocused = true
		m.syncTreeSelection()
		m.refreshDetailList()
		return m, nil
	case msg.String() == "?":
		m.openHelp(modeDetail)
		return m, nil
	case msg.String() == "t":
		m.detailTab = (m.detailTab + 1) % 3
		m.refreshDetailList()
		m.layoutDetailPanes()
		return m, nil
	case key.Matches(msg, key.NewBinding(key.WithKeys("esc"))):
		if m.editMode {
			m.exitEditMode()
			m.status = ""
			return m, nil
		}
		m.mode = modeTree
		m.layoutDetailPanes()
		return m, m.rebuildHostList()
	case msg.String() == "$":
		return m, m.openAppConfigEditor()
	case msg.String() == "&":
		return m.openAppConfigView(modeDetail)
	case msg.String() == "w":
		if err := m.save(); err != nil {
			m.status = errStyle.Render("Save: " + err.Error())
		} else {
			m.status = "Saved."
		}
		return m, nil
	case msg.String() == "s":
		if m.resolveValidateRef(m.selRef) == nil {
			return m.beginInteractiveSSH(modeDetail, postWildConnectSSH)
		}
	case msg.String() == "A":
		m.openActionMenu(modeDetail)
		m.actionMenuList.SetWidth(actionMenuOuterWidth(m.width))
		m.actionMenuList.SetHeight(actionMenuListHeight(m.height))
		return m, nil
	case msg.String() == "i":
		if m.editModeBlocked() {
			return m, nil
		}
		m.returnAfterInput = modeDetail
		m.mode = modeInputHostMeta
		h := m.resolveHostAt(m.selRef)
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
		if m.editModeBlocked() {
			return m, nil
		}
		m.returnAfterInput = modeDetail
		m.openPicker()
		m.pickerList.SetWidth(m.rightPaneWidth())
		m.pickerList.SetHeight(max(6, m.height-5))
		return m, nil
	case msg.String() == "k":
		if m.editModeBlocked() {
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
			if m.editModeBlocked() {
				return m, nil
			}
			m.returnAfterInput = modeDetail
			h := m.resolveHostAt(m.selRef)
			m.valueInput.SetValue(h.Directives[it.idx].Value)
			m.valueInput.Placeholder = "value"
			m.pendingDirectiveKey = h.Directives[it.idx].Key
			m.mode = modeInputDirectiveValue
			m.editDirectiveIndex = it.idx
			m.valueInput.Focus()
			return m, textinput.Blink
		}
	case msg.String() == "d":
		if m.editModeBlocked() {
			return m, nil
		}
		if it, ok := m.detailList.SelectedItem().(dirEntry); ok {
			h := m.resolveHostAt(m.selRef)
			h.Directives = append(h.Directives[:it.idx], h.Directives[it.idx+1:]...)
			m.dirty = true
			m.refreshDetailList()
			m.status = "Directive removed."
		}
		return m, nil
	case msg.String() == "D":
		if m.editModeBlocked() {
			return m, nil
		}
		m.returnAfterInput = modeDetail
		m.mode = modeInputDuplicateHost
		m.valueInput.SetValue(hostTitle(m.resolveHostAt(m.selRef)) + "-copy")
		m.valueInput.Placeholder = "new Host patterns"
		m.valueInput.Focus()
		return m, textinput.Blink
	case msg.String() == "X":
		if m.editModeBlocked() {
			return m, nil
		}
		m.pendingDeletePwIdx = -1
		m.confirmReturnMode = modeDetail
		m.mode = modeConfirmDeleteHost
		m.status = fmt.Sprintf("Delete host %q? [y/N]", hostTitle(m.resolveHostAt(m.selRef)))
		return m, nil
	case msg.String() == "v":
		if m.editModeBlocked() {
			return m, nil
		}
		return m, m.rawEditorCmd()
	case msg.String() == "g":
		if m.editModeBlocked() {
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
		if m.editModeBlocked() {
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
		if m.editModeBlocked() {
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
				return m.beginInteractiveSSH(modeActionMenu, postWildConnectSSH)
			case "sftp":
				return m.beginInteractiveSSH(modeActionMenu, postWildConnectSFTP)
			case "copy":
				return m.beginInteractiveSSH(modeActionMenu, postWildCopyCmd)
			case "import":
				return m.startImportFromMain()
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
		if tryFilterListArrowNav(&m.pickerList, msg) {
			return m, nil
		}
		// Enter: choose the highlighted match and add that directive (no separate "apply filter" step).
		if isEnterKey(msg) && strings.TrimSpace(m.pickerList.FilterValue()) != "" {
			if len(m.pickerList.VisibleItems()) > 0 {
				if kw, ok := m.pickerList.SelectedItem().(kwEntry); ok {
					return m.pickKwEntryAsNewDirective(kw)
				}
			}
		}
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
			return m.pickKwEntryAsNewDirective(it)
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
		if m.groupPickerReturnMode == modeDetail && m.resolveValidateRef(m.selRef) == nil {
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
			rb := m.rebuildHostList()
			if m.mode == modeDetail {
				m.refreshDetailList()
			} else {
				m.syncBrowsePreview()
				m.refreshDetailList()
			}
			return m, rb
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

func paneSepLines(height int) string {
	if height < 1 {
		height = 1
	}
	var buf strings.Builder
	for i := 0; i < height; i++ {
		if i > 0 {
			buf.WriteByte('\n')
		}
		buf.WriteString("│")
	}
	return buf.String()
}

func (m *Model) joinSplitPanes(leftBody, rightBody string, paneHeight int, rightPaneActive bool) string {
	lw := m.leftPaneWidth()
	rw := m.rightPaneWidth()
	leftBody = padVisualLines(leftBody, lw)
	rightBody = padVisualLines(rightBody, rw)
	leftBody = ensureMinVisualLines(leftBody, lw, paneHeight)
	rightBody = ensureMinVisualLines(rightBody, rw, paneHeight)
	leftBox := paneLeftStyle.Width(lw).Height(paneHeight).Render(leftBody)
	rightStyle := paneRightStyle
	if rightPaneActive {
		rightStyle = paneRightActiveStyle
	}
	rightBox := rightStyle.Width(rw).Height(paneHeight).Render(rightBody)
	sep := paneSepStyle.Render(paneSepLines(paneHeight))
	return lipgloss.JoinHorizontal(lipgloss.Top, leftBox, sep, rightBox)
}

const dragonBraille = "" +
	"⠀⠀⠀⠀⠀⠀⠀⠀⠀⢀⣀⣠⣤⣤⣤⣀⡀⠀⠀⠀⠀⠀⠀⠀⠀⠀\n" +
	"⠀⠀⠀⠀⠀⠀⢀⣠⣾⣿⣿⣿⣿⣿⣿⣿⣿⣷⣄⡀⠀⠀⠀⠀⠀⠀\n" +
	"⠀⠀⠀⠀⢀⣴⣿⣿⣿⡿⠟⠛⠉⠉⠛⠻⣿⣿⣿⣷⣄⠀⠀⠀⠀⠀\n" +
	"⠀⠀⠀⣴⣿⣿⣿⠟⠁⠀⠀⠀⠀⠀⠀⠀⠀⠙⢿⣿⣿⣦⠀⠀⠀⠀\n" +
	"⠀⠀⣼⣿⣿⡿⠁⠀⠀⣰⣿⣿⣿⣷⡀⠀⠀⠀⠈⢿⣿⣿⣧⠀⠀⠀\n" +
	"⠀⢸⣿⣿⡿⠀⠀⠀⠀⣿⣿⣿⣿⣿⣿⠀⠀⠀⠀⠀⢿⣿⣿⡇⠀⠀\n" +
	"⠀⣿⣿⣿⠁⠀⠀⠀⠀⠘⠿⣿⣿⠿⠃⠀⠀⣀⡀⠀⠈⣿⣿⣿⠀⠀\n" +
	"⢸⣿⣿⡇⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⣼⣿⡇⠀⠀⢸⣿⣿⡇⠀\n" +
	"⢸⣿⣿⡇⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠙⠟⠁⠀⠀⢸⣿⣿⡇⠀\n" +
	"⠀⣿⣿⣿⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⣿⣿⣿⠀⠀\n" +
	"⠀⠸⣿⣿⣧⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⣼⣿⣿⠇⠀⠀\n" +
	"⠀⠀⠹⣿⣿⣷⡀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⢀⣾⣿⣿⠏⠀⠀⠀\n" +
	"⠀⠀⠀⠙⢿⣿⣿⣦⡀⠀⠀⠀⠀⠀⠀⠀⢀⣴⣿⣿⡿⠃⠀⠀⠀⠀\n" +
	"⠀⠀⠀⠀⠀⠙⠻⣿⣿⣶⣤⣀⣀⣀⣤⣶⣿⣿⠟⠋⠀⠀⠀⠀⠀⠀\n" +
	"⠀⠀⠀⠀⠀⠀⠀⠀⠉⠛⠻⠿⠿⠿⠟⠛⠉⠀⠀⠀⠀⠀⠀⠀⠀⠀"

func (m *Model) logoLine() string {
	ver := m.version
	if ver != "" {
		ver = " v" + ver
	}
	return fmt.Sprintf("🐉 sshui%s", ver)
}

func (m *Model) logoBanner() string {
	ver := m.version
	if ver != "" {
		ver = " v" + ver
	}
	return dragonBraille + "\n" + titleStyle.Render("sshui"+ver)
}

func (m *Model) writeStatusFooter(b *strings.Builder, statusLine string) {
	b.WriteString(footerRuleStyle.Render(strings.Repeat("─", max(1, m.width))))
	b.WriteByte('\n')
	b.WriteString(statusLine)
	if m.status != "" {
		b.WriteByte('\n')
		b.WriteString(m.footerStatusRender())
	}
}

func (m Model) View() string {
	if m.mode == modeAppCfgView {
		return m.viewAppConfigScreen()
	}
	var b strings.Builder
	b.WriteString(m.titleBarLine())
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
		titleSt := titleStyle
		hintSt := statusStyle
		if paneFillActive {
			titleSt = titleSt.Copy().Background(paneFill)
			hintSt = hintSt.Copy().Background(paneFill)
		}
		title := titleSt.Render(m.logoLine() + " — help")
		hint := hintSt.Render("Esc — close help (only Esc exits) · ↑↓ PgUp/PgDn Space scroll")
		body := m.helpViewport.View()
		box := lipgloss.JoinVertical(lipgloss.Left, title, "", body, "", hint)
		innerW := min(max(48, m.width-4), m.width-2)
		framed := lipgloss.NewStyle().Border(panelBorder).Padding(0, 1).Width(innerW).Render(box)
		headerLines := strings.Count(b.String(), "\n") + 1
		placeH := max(1, m.height-headerLines)
		var placeOpts []lipgloss.WhitespaceOption
		if paneFillActive {
			placeOpts = append(placeOpts, lipgloss.WithWhitespaceBackground(paneFill))
		}
		b.WriteString(lipgloss.Place(m.width, placeH, lipgloss.Center, lipgloss.Top, framed, placeOpts...))
		return b.String()

	case modeConfirmDeleteHost, modeConfirmDeleteGroup, modeConfirmImport:
		box := lipgloss.NewStyle().Border(panelBorder).Padding(1, 2).Width(min(56, m.width-4)).Render(m.status)
		b.WriteString(lipgloss.Place(m.width, max(8, m.height-2), lipgloss.Center, lipgloss.Center, box))
		return b.String()

	case modeExportWizard:
		b.WriteString(m.wizardView())
		return b.String()

	case modePasswordDetail, modePasswordDetailEdit:
		ph := max(5, m.height-5)
		left := lipgloss.JoinVertical(lipgloss.Left, m.hostList.View())
		right := m.pwDetailList.View()
		b.WriteString(m.joinSplitPanes(left, right, ph, true))
		b.WriteByte('\n')
		editHint := m.editModeSuffix()
		m.writeStatusFooter(&b, statusStyle.Render(fmt.Sprintf(
			"e/enter edit · w save overlay · s ssh · x delete host · esc back%s%s",
			editHint, m.browseModeSuffix(),
		)))
		return b.String()

	case modeSSHConnectPending:
		body := fmt.Sprintf("Connecting to %s…\n\n", m.sshConnectPendingAlias)
		if m.sshConnectPendingNote != "" {
			body += statusStyle.Render(m.sshConnectPendingNote) + "\n\n"
		}
		body += statusStyle.Render("Esc — cancel")
		box := lipgloss.NewStyle().Border(panelBorder).Padding(1, 2).Width(min(56, m.width-4)).Render(body)
		b.WriteString(lipgloss.Place(m.width, max(8, m.height-2), lipgloss.Center, lipgloss.Center, box))
		return b.String()

	case modeSSHConnectWildcardInput:
		var lines []string
		if m.sshConnectPendingNote != "" {
			lines = append(lines, statusStyle.Render(m.sshConnectPendingNote))
		}
		switch m.sshConnectWildKind {
		case sshWildSuffixStar:
			lines = append(lines, fmt.Sprintf("Host pattern %q ends with * — fixed part:", m.sshConnectWildPattern))
			lines = append(lines, statusStyle.Render("Type the rest after this prefix, then Enter (Esc cancels)."))
			prefix := titleStyle.Render(m.sshConnectWildPrefix)
			inputLine := lipgloss.JoinHorizontal(lipgloss.Top, prefix, m.valueInput.View())
			lines = append(lines, inputLine)
		case sshWildPrefixStar:
			lines = append(lines, fmt.Sprintf("Host pattern %q starts with * — fixed suffix:", m.sshConnectWildPattern))
			lines = append(lines, statusStyle.Render("Type the part before the suffix, then Enter (Esc cancels)."))
			suf := titleStyle.Render(m.sshConnectWildSuffix)
			inputLine := lipgloss.JoinHorizontal(lipgloss.Top, m.valueInput.View(), suf)
			lines = append(lines, inputLine)
		default:
			lines = append(lines, fmt.Sprintf("Host pattern %q needs a concrete hostname.", m.sshConnectWildPattern))
			lines = append(lines, statusStyle.Render("Enter the name you pass to ssh, then Enter (Esc cancels)."))
			lines = append(lines, m.valueInput.View())
		}
		lines = append(lines, "", statusStyle.Render("Enter — continue · Esc — cancel"))
		boxBody := strings.Join(lines, "\n")
		box := lipgloss.NewStyle().Border(panelBorder).Padding(1, 2).Width(min(56, m.width-4)).Render(boxBody)
		b.WriteString(lipgloss.Place(m.width, max(8, m.height-2), lipgloss.Center, lipgloss.Center, box))
		return b.String()

	case modeNewHostTypePicker:
		body := titleStyle.Render("New host — choose type") + "\n\n"
		body += statusStyle.Render("  o — OpenSSH host  (default)") + "\n"
		body += statusStyle.Render("  p — Password host (overlay)") + "\n\n"
		body += statusStyle.Render("  Esc — cancel")
		box := lipgloss.NewStyle().Border(panelBorder).Padding(1, 2).Width(min(44, m.width-4)).Render(body)
		b.WriteString(lipgloss.Place(m.width, max(8, m.height-2), lipgloss.Center, lipgloss.Center, box))
		return b.String()

	case modeNewGroupTypePicker:
		body := titleStyle.Render("New group — choose type") + "\n\n"
		body += statusStyle.Render("  o — OpenSSH group  (default)") + "\n"
		body += statusStyle.Render("  p — Password group (overlay)") + "\n\n"
		body += statusStyle.Render("  Esc — cancel")
		box := lipgloss.NewStyle().Border(panelBorder).Padding(1, 2).Width(min(44, m.width-4)).Render(body)
		b.WriteString(lipgloss.Place(m.width, max(8, m.height-2), lipgloss.Center, lipgloss.Center, box))
		return b.String()

	case modeActionMenu:
		menu := m.renderActionMenuBox()
		headerLines := strings.Count(b.String(), "\n") + 1
		placeH := max(1, m.height-headerLines)
		var placeOpts []lipgloss.WhitespaceOption
		if paneFillActive {
			placeOpts = append(placeOpts, lipgloss.WithWhitespaceBackground(paneFill))
		}
		b.WriteString(lipgloss.Place(m.width, placeH, lipgloss.Center, lipgloss.Center, menu, placeOpts...))
		return b.String()

	case modeInputDirectiveValue, modeInputNewHost, modeInputNewPasswordHost, modeInputDuplicateHost, modeInputNewGroup, modeInputNewPasswordGroup, modeInputRenameGroup, modeInputGroupDesc, modeInputHostMeta, modeInputPasswordField:
		switch m.mode {
		case modeInputNewGroup:
			b.WriteString(statusStyle.Render("New group name, Esc cancel"))
		case modeInputRenameGroup:
			b.WriteString(statusStyle.Render("Rename group, Esc cancel"))
		case modeInputGroupDesc:
			b.WriteString(statusStyle.Render("Group #@desc line (empty clears), Esc cancel"))
		case modeInputHostMeta:
			b.WriteString(statusStyle.Render("Host #@host lines, Esc cancel"))
		case modeInputNewPasswordHost:
			b.WriteString(statusStyle.Render("New password host: hostname or IP first (patterns default to this) · Esc cancel"))
		case modeInputNewPasswordGroup:
			b.WriteString(statusStyle.Render("New password group name, Esc cancel"))
		case modeInputPasswordField:
			b.WriteString(statusStyle.Render("Edit field · Enter save · Esc cancel"))
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
		left := lipgloss.JoinVertical(lipgloss.Left, m.hostList.View())
		var right string
		if m.detailTab == detailTabOverview {
			right = m.overviewPanel()
		} else {
			right = m.detailList.View()
		}
		b.WriteString(m.joinSplitPanes(left, right, ph, m.mode == modeDetail && !m.treePaneFocused))
		b.WriteByte('\n')
		focus := "detail"
		if m.treePaneFocused {
			focus = "tree"
		}
		m.writeStatusFooter(&b, statusStyle.Render(fmt.Sprintf(
			"focus %s | tab | t tabs | W Include | i meta | A actions | s ssh | a k add | e d D g m o X | v w | $ cfg | & view cfg | z fold | esc tree%s%s",
			focus, m.editModeSuffix(), m.browseModeSuffix(),
		)))
		return b.String()

	case modePicker:
		b.WriteString(m.pickerList.View())
		b.WriteByte('\n')
		b.WriteString(statusStyle.Render("/ filter · ↑↓ pick · enter add · esc cancel"))
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
		left := m.hostList.View()
		var right string
		if _, onPw := m.hostList.SelectedItem().(passwordHostRowEntry); onPw {
			right = m.pwDetailList.View()
		} else if _, onHost := m.hostList.SelectedItem().(hostRowEntry); onHost && m.resolveValidateRef(m.selRef) == nil {
			right = m.detailList.View()
		} else {
			right = paneRightStyle.Copy().
				Padding(1, 1).
				Foreground(lipgloss.Color("245")).
				Render("Select a host row to preview directives.\n\nTip: c creates a group; header: z fold, D delete group.")
		}
		b.WriteString(m.joinSplitPanes(left, right, ph, false))
		b.WriteByte('\n')
		m.writeStatusFooter(&b, statusStyle.Render(fmt.Sprintf(
			"enter editor | B browse | I setup | W Include | A | s ssh | n host | c group | z fold | D del grp | x host | g move | v raw | $ cfg | / filter | w r ? q%s%s",
			m.editModeSuffix(), m.browseModeSuffix(),
		)))
		return b.String()
	}
}

// --- Password host detail ---

func (m *Model) startNewOpenSSHHost() tea.Cmd {
	m.returnAfterInput = modeTree
	m.mode = modeInputNewHost
	m.valueInput.SetValue("")
	m.valueInput.Placeholder = "Host patterns (space-separated)"
	m.valueInput.Focus()
	return textinput.Blink
}

func (m *Model) startNewPasswordHost() tea.Cmd {
	m.returnAfterInput = modeTree
	m.mode = modeInputNewPasswordHost
	m.valueInput.SetValue("")
	hint := "Hostname or IP (ssh connects here; patterns default to this)"
	if g := m.selectedPwGroup(); g != "" {
		hint = fmt.Sprintf("Hostname — group %q · patterns default to hostname", g)
	}
	m.valueInput.Placeholder = hint
	m.valueInput.Width = min(60, max(40, m.width-8))
	m.valueInput.Focus()
	return textinput.Blink
}

// selectedPwGroup returns the password group name for the currently selected
// password group header, or "" if not on one.
func (m *Model) selectedPwGroup() string {
	if gh, ok := m.hostList.SelectedItem().(groupHeaderEntry); ok && gh.groupIdx == -2 {
		return gh.pwGroup
	}
	if pw, ok := m.hostList.SelectedItem().(passwordHostRowEntry); ok {
		if m.overlayData != nil && pw.idx >= 0 && pw.idx < len(m.overlayData.PasswordHosts) {
			return m.overlayData.PasswordHosts[pw.idx].Group
		}
	}
	return ""
}

func (m *Model) openPasswordDetail(idx int) {
	if m.overlayData == nil || idx < 0 || idx >= len(m.overlayData.PasswordHosts) {
		return
	}
	m.pwSelIdx = idx
	m.refreshPasswordDetailList()
	m.mode = modePasswordDetail
}

func (m *Model) connectPasswordHost(idx int, returnMode viewMode) (*Model, tea.Cmd) {
	if m.overlayData == nil || idx < 0 || idx >= len(m.overlayData.PasswordHosts) {
		return m, nil
	}
	ph := &m.overlayData.PasswordHosts[idx]
	target := ph.Hostname
	if target == "" {
		target = ph.Title()
	}
	m.sshConnectReturnMode = returnMode
	m.sshConnectPendingCancelled = false
	m.sshConnectPendingAlias = target
	m.sshConnectPendingNote = ""
	m.sshConnectPendingSFTP = false
	m.mode = modeSSHConnectPending
	return m, tea.Tick(500*time.Millisecond, func(time.Time) tea.Msg { return sshConnectReadyMsg{} })
}

func buildPasswordSSHArgs(ph *overlay.PasswordHost) []string {
	var args []string
	if ph.Port > 0 && ph.Port != 22 {
		args = append(args, "-p", fmt.Sprintf("%d", ph.Port))
	}
	target := ph.Hostname
	if ph.User != "" {
		target = ph.User + "@" + target
	}
	args = append(args, target)
	return args
}

func (m *Model) refreshPasswordDetailList() {
	rw := m.rightPaneWidth()
	rh := max(5, m.height-5)
	if m.mode == modePasswordDetail || m.mode == modePasswordDetailEdit {
		rw = m.rightPaneWidth()
		rh = max(5, m.height-5)
	}
	delegate := newDetailListDelegate()
	m.applyDetailListDelegateBG(&delegate)
	var items []list.Item
	title := "Password host"
	if m.overlayData != nil && m.pwSelIdx >= 0 && m.pwSelIdx < len(m.overlayData.PasswordHosts) {
		ph := &m.overlayData.PasswordHosts[m.pwSelIdx]
		title = "Password: " + ph.Title()
		portDesc := "(default 22)"
		if ph.Port > 0 {
			portDesc = fmt.Sprintf("%d", ph.Port)
		}
		items = append(items, pwFieldEntry{key: "hostname", field: "hostname", val: ph.Hostname})
		items = append(items, pwFieldEntry{key: "patterns", field: "patterns", val: ph.PatternsString()})
		items = append(items, pwFieldEntry{key: "user", field: "user", val: ph.User})
		items = append(items, pwFieldEntry{key: "port", field: "port", val: portDesc})
		items = append(items, pwFieldEntry{key: "askpass", field: "askpass", val: ph.Askpass})
		items = append(items, pwFieldEntry{key: "askpass_require", field: "askpass_require", val: ph.AskpassRequire})
		items = append(items, pwFieldEntry{key: "display", field: "display", val: ph.Display})
		items = append(items, pwFieldEntry{key: "group", field: "group", val: ph.Group})
	} else {
		title = "Password host — select a row"
	}
	l := list.New(items, delegate, rw, rh)
	l.Title = title
	l.Styles.Title = titleStyle
	l.SetShowStatusBar(true)
	l.SetFilteringEnabled(false)
	l.DisableQuitKeybindings()
	applyPaneChromeToList(&l)
	m.pwDetailList = l
}

func (m *Model) startPasswordFieldEdit(it pwFieldEntry) {
	ph := &m.overlayData.PasswordHosts[m.pwSelIdx]
	m.mode = modeInputPasswordField
	m.pwEditFieldKey = it.field
	m.valueInput.SetValue("")
	m.valueInput.Placeholder = ""
	switch it.field {
	case "hostname":
		m.valueInput.SetValue(ph.Hostname)
		m.valueInput.Placeholder = "hostname or IP for ssh"
	case "patterns":
		m.valueInput.SetValue(ph.PatternsString())
		m.valueInput.Placeholder = "space-separated Host patterns (ssh aliases)"
	case "user":
		m.valueInput.SetValue(ph.User)
		m.valueInput.Placeholder = "ssh user (optional)"
	case "port":
		if ph.Port > 0 {
			m.valueInput.SetValue(fmt.Sprintf("%d", ph.Port))
		}
		m.valueInput.Placeholder = "port (empty = 22)"
	case "askpass":
		m.valueInput.SetValue(ph.Askpass)
		m.valueInput.Placeholder = "path to SSH_ASKPASS script"
	case "askpass_require":
		m.valueInput.SetValue(ph.AskpassRequire)
		m.valueInput.Placeholder = "e.g. force (optional)"
	case "display":
		m.valueInput.SetValue(ph.Display)
		m.valueInput.Placeholder = "DISPLAY for askpass (optional)"
	case "group":
		m.valueInput.SetValue(ph.Group)
		m.valueInput.Placeholder = "overlay group name (optional)"
	}
	m.valueInput.Width = min(64, max(36, m.width-8))
	m.valueInput.Focus()
}

func (m *Model) saveOverlay() error {
	if m.overlayData == nil || m.overlayPath == "" {
		return fmt.Errorf("no overlay configured")
	}
	return overlay.Save(m.overlayPath, m.overlayData)
}

// --- Setup wizard ---
//
// Steps:  0 — SSH config path  (main ~/.ssh/config)
//         1 — SSH hosts path   (sshui-managed file)
//         2 — Overlay path     (password_hosts.toml)
//         3 — Copy question    (y / n / q)

var wizardLabels = [3]string{
	"Main SSH config",
	"sshui SSH hosts file",
	"Password overlay",
}

func (m *Model) wizardPrepareStep() {
	if m.wizardStep >= 3 {
		return
	}
	var val string
	switch m.wizardStep {
	case 0:
		val = m.mainSSHConfigPath
	case 1:
		val = m.sshHostsPath
	case 2:
		val = m.overlayPath
	}
	m.valueInput.SetValue(val)
	m.valueInput.CursorEnd()
	m.valueInput.Focus()
	m.valueInput.Placeholder = ""
	m.valueInput.Width = min(58, m.width-12)
}

func (m *Model) wizardView() string {
	title := m.logoBanner() + " — Setup"

	var lines []string
	lines = append(lines, "")
	lines = append(lines, statusStyle.Render(fmt.Sprintf("Config will be saved to: %s", m.appConfigPath)))
	lines = append(lines, "")

	for i := 0; i < 3; i++ {
		var val string
		switch i {
		case 0:
			val = m.mainSSHConfigPath
		case 1:
			val = m.sshHostsPath
		case 2:
			val = m.overlayPath
		}

		prefix := "  "
		if i == m.wizardStep {
			prefix = "> "
		}
		label := fmt.Sprintf("%s%d. %s", prefix, i+1, wizardLabels[i])

		if i < m.wizardStep {
			lines = append(lines, statusStyle.Render(fmt.Sprintf("%s: %s", label, val)))
		} else if i == m.wizardStep {
			lines = append(lines, fmt.Sprintf("%s:", label))
			lines = append(lines, "     "+m.valueInput.View())
		} else {
			lines = append(lines, statusStyle.Render(fmt.Sprintf("%s: %s", label, val)))
		}
	}

	lines = append(lines, "")

	copyLabel := "  4. Move hosts from main SSH config?"
	if m.wizardStep < 3 {
		lines = append(lines, statusStyle.Render(copyLabel))
	} else {
		lines = append(lines, fmt.Sprintf("> 4. Move hosts from main SSH config?"))
		lines = append(lines, "")
		lines = append(lines, statusStyle.Render("     y — Yes, move hosts and add Include"))
		lines = append(lines, statusStyle.Render("     n — No, just add Include"))
		lines = append(lines, statusStyle.Render("     q — Quit"))
	}

	lines = append(lines, "")
	if m.wizardStep < 3 {
		lines = append(lines, statusStyle.Render("Enter to confirm  •  Esc to go back"))
	}

	body := title + "\n" + strings.Join(lines, "\n")
	boxW := min(70, m.width-4)
	box := lipgloss.NewStyle().Border(panelBorder).Padding(1, 2).Width(boxW).Render(body)

	placeH := max(1, m.height-2)
	return lipgloss.Place(m.width, placeH, lipgloss.Center, lipgloss.Center, box)
}

func (m *Model) handleExportWizard(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.wizardStep == 3 {
		switch msg.String() {
		case "y", "Y":
			return m.wizardFinish(true)
		case "n", "N":
			return m.wizardFinish(false)
		case "q", "Q":
			m.exportWizardNeeded = false
			m.mode = modeTree
			m.status = "Setup wizard cancelled."
			return m, nil
		case "esc":
			m.wizardStep = 2
			m.wizardPrepareStep()
			return m, textinput.Blink
		}
		return m, nil
	}

	switch msg.String() {
	case "enter":
		val := strings.TrimSpace(m.valueInput.Value())
		if val == "" {
			return m, nil
		}
		expanded, err := appcfg.ExpandPath(val)
		if err != nil {
			m.status = errStyle.Render("Invalid path: " + err.Error())
			return m, nil
		}
		switch m.wizardStep {
		case 0:
			m.mainSSHConfigPath = expanded
		case 1:
			m.sshHostsPath = expanded
			m.path = expanded
		case 2:
			m.overlayPath = expanded
		}
		m.wizardStep++
		if m.wizardStep < 3 {
			m.wizardPrepareStep()
			return m, textinput.Blink
		}
		m.valueInput.Blur()
		return m, nil

	case "esc":
		if m.wizardStep == 0 {
			m.exportWizardNeeded = false
			m.mode = modeTree
			m.status = "Setup wizard cancelled."
			return m, nil
		}
		m.wizardStep--
		m.wizardPrepareStep()
		return m, textinput.Blink
	}

	var cmd tea.Cmd
	m.valueInput, cmd = m.valueInput.Update(msg)
	return m, cmd
}

func (m *Model) wizardFinish(copyHosts bool) (tea.Model, tea.Cmd) {
	if err := scfg.EnsureSSHHostsFile(m.sshHostsPath); err != nil {
		m.status = errStyle.Render("Create ssh_hosts: " + err.Error())
		m.mode = modeTree
		return m, nil
	}

	mainRewritten := false
	if copyHosts {
		data := m.readMainConfig()
		if data != "" {
			mainCfg, err := scfg.Parse(strings.NewReader(data))
			if err != nil {
				m.status = errStyle.Render("Parse main config: " + err.Error())
			} else if err := scfg.ExportHostsTo(mainCfg, m.sshHostsPath, filepath.Dir(m.mainSSHConfigPath)); err != nil {
				m.status = errStyle.Render("Export hosts: " + err.Error())
			} else if err := scfg.ReplaceMainSSHConfigWithManagedInclude(m.mainSSHConfigPath, m.sshHostsPath); err != nil {
				m.status = errStyle.Render("Rewrite main ssh_config: " + err.Error())
			} else {
				m.status = "Hosts moved from " + m.mainSSHConfigPath
				mainRewritten = true
			}
		}
	}

	if !mainRewritten {
		if err := scfg.AppendInclude(m.mainSSHConfigPath, m.sshHostsPath); err != nil {
			if m.status == "" {
				m.status = errStyle.Render("Include: " + err.Error())
			}
		}
	}

	// Persist wizard choices into config.toml with sensible defaults.
	if m.appConfig != nil {
		m.appConfig.SSHConfig = m.mainSSHConfigPath
		m.appConfig.Hosts.SSHHostsPath = m.sshHostsPath
		m.appConfig.Hosts.PasswordOverlay = m.overlayPath

		if m.appConfig.Editor == "" {
			m.appConfig.Editor = wizardDefaultEditor()
		}
		if m.appConfig.Theme == "" {
			m.appConfig.Theme = "default"
		}
		if m.appConfig.Hosts.BrowseMode == "" {
			m.appConfig.Hosts.BrowseMode = appcfg.BrowseModeMerged
		}

		m.editor = m.appConfig.Editor
		m.themeName = m.appConfig.Theme
		m.browseMode = m.appConfig.Hosts.BrowseMode

		if err := appcfg.Save(m.appConfig); err != nil {
			if m.status == "" {
				m.status = errStyle.Render("Save config: " + err.Error())
			}
		}
	}

	if m.status == "" {
		if copyHosts {
			m.status = "Setup complete — hosts moved, Include added."
		} else {
			m.status = "Setup complete — Include added."
		}
	}

	m.exportWizardNeeded = false
	m.mode = modeTree
	return m, m.reloadFromDisk()
}

func (m *Model) readMainConfig() string {
	path := m.mainSSHConfigPath
	if path == "" {
		path = m.path
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(data)
}

func wizardDefaultEditor() string {
	if v := os.Getenv("VISUAL"); v != "" {
		return v
	}
	if v := os.Getenv("EDITOR"); v != "" {
		return v
	}
	return "vi"
}

// --- Import from main ssh_config ---

func (m *Model) startImportFromMain() (tea.Model, tea.Cmd) {
	if !m.selRef.FromMain || m.mainCfg == nil {
		m.status = errStyle.Render("No main-config host selected.")
		m.mode = m.actionReturnMode
		return m, nil
	}
	if m.mainCfg.ValidateRef(m.selRef) != nil {
		m.status = errStyle.Render("Invalid host reference.")
		m.mode = m.actionReturnMode
		return m, nil
	}
	hb := m.mainCfg.HostAt(m.selRef)
	clone := cloneHostBlockForImport(*hb)
	m.cfg.DefaultHosts = append(m.cfg.DefaultHosts, clone)
	m.dirty = true
	if err := m.save(); err != nil {
		m.status = errStyle.Render("Save managed: " + err.Error())
		m.mode = modeTree
		return m, nil
	}
	m.pendingImportRef = m.selRef
	m.selRef = scfg.HostRef{InDefault: true, HostIdx: len(m.cfg.DefaultHosts) - 1}
	alias := hostAlias(&clone)
	m.status = fmt.Sprintf("Imported %q — remove from main ssh_config? [Y/n]", alias)
	m.mode = modeConfirmImport
	return m, nil
}

func cloneHostBlockForImport(h scfg.HostBlock) scfg.HostBlock {
	out := scfg.HostBlock{
		HostComments: append([]string(nil), h.HostComments...),
		Patterns:     append([]string(nil), h.Patterns...),
		Directives:   make([]scfg.Directive, len(h.Directives)),
	}
	copy(out.Directives, h.Directives)
	return out
}

// saveMainConfig writes the in-memory mainCfg to disk with a backup.
func (m *Model) saveMainConfig() error {
	if m.mainCfg == nil || m.mainSSHConfigPath == "" {
		return fmt.Errorf("no main config to save")
	}
	out, err := scfg.String(m.mainCfg)
	if err != nil {
		return fmt.Errorf("serialize main config: %w", err)
	}
	body := []byte(out)
	prev, err := os.ReadFile(m.mainSSHConfigPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read main config for backup: %w", err)
	}
	if err == nil {
		bkp := hiddenBackupPath(m.mainSSHConfigPath)
		if werr := os.WriteFile(bkp, prev, 0o600); werr != nil {
			return fmt.Errorf("backup main config %s: %w", bkp, werr)
		}
	}
	return os.WriteFile(m.mainSSHConfigPath, body, 0o600)
}

// browseModeSuffix returns a short string for the footer to indicate the current browse mode.
func (m *Model) browseModeSuffix() string {
	switch m.browseMode {
	case appcfg.BrowseModeOpenSSH:
		return " [openssh]"
	case appcfg.BrowseModePassword:
		return " [password]"
	default:
		return ""
	}
}

// editModeSuffix returns a short footer string for the edit mode.
func (m *Model) editModeSuffix() string {
	if m.editMode {
		return " [editing]"
	}
	return " [read-only]"
}

const helpText = `
sshui - SSH client config TUI

Browse (split view)
  enter     Open full editor or password host detail
  /         Filter host list
  z         Collapse/expand group
  B         Cycle browse mode: merged > openssh > password (persisted)
  A         Actions: ssh / sftp / copy / import (on ssh_config hosts)
  n         New host (OpenSSH or password, depends on browse mode)
  c         Create group (OpenSSH or password, depends on browse mode)
  D         Delete group (when a group header row is selected)
  x         Delete host (confirm)
  g         Move selected host to group / (default)
  v         Raw $EDITOR buffer
  $ / &     Edit / view sshui app config
  s         SSH connect (password hosts use SSH_ASKPASS overlay)
  w / r     Write (save) / reload
  W         Toggle Include view: merged read-only / editable main file only
  I         Re-run setup wizard (move hosts, Include, config.toml)
  ?         Open this help
  q         Quit

Main ssh_config + managed ssh_hosts (dual tree)
  When config.toml sets ssh_config to a different file than ssh_hosts_path, the tree
  shows two sections separated by a blank line:
    🔒 unmanaged   Read-only hosts from the parent ssh_config (e.g. ~/.ssh/config).
                   Shown in a dimmer color. Groups keep their names with a 🔒 suffix.
    📝 managed      Editable hosts in ssh_hosts (the primary edit target).
  Hosts in the unmanaged section cannot be edited, deleted, or moved directly.
  To move a host into managed, select it, press A → Import. sshui copies the host
  into ssh_hosts and prompts to remove it from the main file (default: yes).

Edit mode
  sshui starts in read-only mode. Mutating actions enter edit mode and acquire a
  lock (.sshui.swp) to prevent concurrent modifications. Esc on host detail exits
  edit mode first, then exits the detail view. Footer shows [read-only] or [editing].

Browse modes (B key)
  merged    OpenSSH hosts + password overlay hosts in one tree (default)
  openssh   Only hosts from the ssh_hosts file
  password  Only password-overlay hosts from password_hosts.toml
  Merged tree: same group name in both → one section with 🔀 after the name; password-only
              groups show 🔤🔑 before the name (letters + key).

Password hosts (overlay)
  Password-authenticated hosts live in password_hosts.toml (TOML, not ssh_config).
  New host (n): enter hostname or IP first (ssh target); patterns default to that string.
  Browse: the right pane lists fields; press enter to open password detail, then e or enter
  on a row to edit patterns, user, port, askpass, askpass_require, display, or group.
  w saves the overlay; s runs ssh with SSH_ASKPASS env. See docs/ASKPASS.md for vault recipes.

Password host detail
  e / enter  Edit highlighted field · w save overlay · s ssh · esc back (exits edit mode first)

Include / read-only (merged view)
  If your config has Include, sshui may start in merged read-only mode. Press W to
  switch to writable single-file view. r reloads from disk.

Host detail (split: tree | detail)
  tab       Focus tree vs detail pane
  t         Cycle tab: Overview > All > Connectivity
  i         Edit #@host metadata lines
  A         Actions menu (ssh / sftp / copy / import)
  s         SSH connect
  a / k     Add directive (picker / custom key)
  e / d     Edit value / delete directive
  D         Duplicate host
  g         Move host to another group
  m / o     Rename group / edit #@desc
  X         Delete host (confirm)
  v / w     Raw editor / write (save)
  z         Fold group on tree
  esc       Exit edit mode (if editing), then back to tree

Paths
  ssh_hosts:         ~/.config/sshui/ssh_hosts (OpenSSH format, primary edit target)
  password overlay:  ~/.config/sshui/password_hosts.toml
  app config:        ~/.config/sshui/config.toml
  main ssh_config:   ~/.ssh/config (Include ssh_hosts appended on first run)

CLI: sshui list | sshui show HOST [--json] | sshui dump [--json] [--check]
     sshui completion bash|zsh|fish

NO_COLOR=1 disables ANSI styling. Each save writes a hidden .bkp beside the config.
`
