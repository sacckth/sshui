package tui

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/mattn/go-runewidth"

	"github.com/sacckth/sshui/internal/appcfg"
	scfg "github.com/sacckth/sshui/internal/config"
	"github.com/sacckth/sshui/internal/overlay"
)

const listEllipsis = "…"

// labelMergedMark is shown after a group name when merged view combines OpenSSH and password hosts.
const labelMergedMark = " 🔀"

// passwordGroupHeaderLabel returns the tree label for a password-only group (🔤 letters + 🔑 key + name).
func passwordGroupHeaderLabel(gname string) string {
	return "🔤🔑 " + gname
}

func overlayWantsPasswordTree(ov *overlay.File) bool {
	return ov != nil && (len(ov.PasswordHosts) > 0 || len(ov.Groups) > 0)
}

// overlayHasPasswordGroupName is true if the overlay declares this group (explicit list or any host).
func overlayHasPasswordGroupName(ov *overlay.File, name string) bool {
	if ov == nil || name == "" {
		return false
	}
	for _, g := range ov.Groups {
		if g == name {
			return true
		}
	}
	for i := range ov.PasswordHosts {
		if ov.PasswordHosts[i].Group == name {
			return true
		}
	}
	return false
}

// appendPasswordUngroupedAndNamed appends (password) + named password groups; skip names in emitted if non-nil.
func appendPasswordUngroupedAndNamed(items []list.Item, ov *overlay.File, totalWidth int, emitted map[string]bool, nameLabel func(string) string) []list.Item {
	grouped := ov.GroupedHosts()
	ordered := ov.OrderedGroups()

	ungrouped := grouped[""]
	if len(ungrouped) > 0 {
		items = append(items, groupHeaderEntry{
			label:      "(password)",
			collapsed:  false,
			defaultSec: false,
			groupIdx:   -2,
		})
		n := len(ungrouped)
		for hi, idx := range ungrouped {
			ph := &ov.PasswordHosts[idx]
			prefix := treeHostPrefix(hi, n)
			line := formatHostListLine(prefix, ph.Title(), totalWidth)
			items = append(items, passwordHostRowEntry{title: line, idx: idx})
		}
	}

	for _, gname := range ordered {
		if gname == "" {
			continue
		}
		if emitted != nil && emitted[gname] {
			continue
		}
		hosts := grouped[gname]
		items = append(items, groupHeaderEntry{
			label:      nameLabel(gname),
			collapsed:  false,
			defaultSec: false,
			groupIdx:   -2,
			pwGroup:    gname,
		})
		n := len(hosts)
		for hi, idx := range hosts {
			ph := &ov.PasswordHosts[idx]
			prefix := treeHostPrefix(hi, n)
			line := formatHostListLine(prefix, ph.Title(), totalWidth)
			items = append(items, passwordHostRowEntry{title: line, idx: idx})
		}
	}
	return items
}

// sectionSepEntry is a blank row used as visual spacing between tree sections.
type sectionSepEntry struct{}

func (e sectionSepEntry) Title() string       { return "" }
func (e sectionSepEntry) Description() string { return "" }
func (e sectionSepEntry) FilterValue() string { return "" }

// groupHeaderEntry is a section label in the host tree (collapse/expand via z).
type groupHeaderEntry struct {
	label      string
	collapsed  bool
	defaultSec bool   // true for the (default) pseudo-group
	groupIdx   int    // index into cfg.Groups when !defaultSec; -2 for password groups; -3 for unmanaged
	pwGroup    string // non-empty for named password groups
	unmanaged  bool   // true for read-only hosts from the parent ssh_config
}

func (e groupHeaderEntry) Title() string {
	sign := "- "
	if e.collapsed {
		sign = "+ "
	}
	return sign + e.label
}

func (e groupHeaderEntry) Description() string { return "" }

func (e groupHeaderEntry) FilterValue() string { return e.Title() }

// hostRowEntry is one host row showing Host patterns only (left pane).
type hostRowEntry struct {
	title string
	ref   scfg.HostRef
}

func (e hostRowEntry) Title() string { return e.title }

func (e hostRowEntry) Description() string { return "" }

func (e hostRowEntry) FilterValue() string { return e.title }

// passwordHostRowEntry is a password-overlay host row in the tree.
type passwordHostRowEntry struct {
	title string
	idx   int // index into overlay.File.PasswordHosts
}

func (e passwordHostRowEntry) Title() string       { return e.title }
func (e passwordHostRowEntry) Description() string { return "" }
func (e passwordHostRowEntry) FilterValue() string { return e.title }

// groupPickItem is a target group for MoveHost.
type groupPickItem struct {
	label     string
	toDefault bool
	groupIdx  int
}

func (e groupPickItem) Title() string       { return e.label }
func (e groupPickItem) Description() string { return "" }
func (e groupPickItem) FilterValue() string { return e.label }

func directiveValue(h *scfg.HostBlock, keys ...string) string {
	for _, want := range keys {
		for i := range h.Directives {
			if strings.EqualFold(h.Directives[i].Key, want) {
				return strings.TrimSpace(h.Directives[i].Value)
			}
		}
	}
	return ""
}

func hostAlias(h *scfg.HostBlock) string {
	if len(h.Patterns) == 0 {
		return "(empty)"
	}
	return strings.Join(h.Patterns, " ")
}

// HostConnectivityTitle is the preferred heading for a host (User @ HostName), falling
// back to Host patterns when those directives are missing.
func HostConnectivityTitle(h *scfg.HostBlock) string {
	hn := directiveValue(h, "HostName", "hostname")
	user := directiveValue(h, "User", "user")
	switch {
	case hn != "" && user != "":
		return user + " @ " + hn
	case hn != "":
		return hn
	case user != "":
		return user
	default:
		return hostAlias(h)
	}
}

const (
	treeBranchMid = "├── "
	treeBranchEnd = "└── "
)

func treeHostPrefix(hostIndex, hostCount int) string {
	if hostCount <= 1 {
		return treeBranchEnd
	}
	if hostIndex < hostCount-1 {
		return treeBranchMid
	}
	return treeBranchEnd
}

func formatHostListLine(prefix, alias string, totalWidth int) string {
	pw := runewidth.StringWidth(prefix)
	inner := totalWidth - pw
	if inner < 4 {
		inner = 4
	}
	return prefix + runewidth.Truncate(alias, inner, listEllipsis)
}

func groupDescEditPreview(lines []string) string {
	for _, line := range lines {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(strings.ToLower(t), "#@desc:") {
			return strings.TrimSpace(t[7:])
		}
	}
	return ""
}

// buildHostItemsFiltered builds tree rows filtered by browse mode.
// mainCfg, when non-nil, is displayed as a read-only section above the managed hosts.
func buildHostItemsFiltered(cfg *scfg.Config, ov *overlay.File, browseMode string, totalWidth int, ignoreCollapse bool, mainCfg ...*scfg.Config) []list.Item {
	if totalWidth < 12 {
		totalWidth = 48
	}
	var items []list.Item

	showOpenSSH := browseMode == appcfg.BrowseModeMerged || browseMode == appcfg.BrowseModeOpenSSH
	showPassword := browseMode == appcfg.BrowseModeMerged || browseMode == appcfg.BrowseModePassword
	pwTree := overlayWantsPasswordTree(ov)
	mergedPwLayout := browseMode == appcfg.BrowseModeMerged && showOpenSSH && showPassword && pwTree

	var emittedPw map[string]bool

	// Resolve optional mainCfg from variadic param.
	var mc *scfg.Config
	if len(mainCfg) > 0 {
		mc = mainCfg[0]
	}
	hasMainSection := mc != nil && showOpenSSH && (len(mc.DefaultHosts) > 0 || len(mc.Groups) > 0)

	// --- Main ssh_config section (read-only) ---
	if hasMainSection {
		items = appendMainCfgSection(items, mc, totalWidth, ignoreCollapse)
	}

	// --- Managed ssh_hosts section ---
	if showOpenSSH {
		defLabel := "(default)"
		if hasMainSection {
			defLabel = "✏️  managed"
		}
		defCollapsed := cfg.DefaultHostsCollapsed && !ignoreCollapse
		items = append(items, groupHeaderEntry{
			label:      defLabel,
			collapsed:  defCollapsed,
			defaultSec: true,
			groupIdx:   -1,
		})
		if !defCollapsed || ignoreCollapse {
			n := len(cfg.DefaultHosts)
			for i := range cfg.DefaultHosts {
				h := &cfg.DefaultHosts[i]
				al := hostAlias(h)
				prefix := treeHostPrefix(i, n)
				line := formatHostListLine(prefix, al, totalWidth)
				items = append(items, hostRowEntry{
					title: line,
					ref:   scfg.HostRef{InDefault: true, HostIdx: i},
				})
			}
		}

		if mergedPwLayout {
			grouped := ov.GroupedHosts()
			emittedPw = make(map[string]bool)
			for gi := range cfg.Groups {
				g := &cfg.Groups[gi]
				merge := overlayHasPasswordGroupName(ov, g.Name)
				collapsed := g.CollapsedByDefault && !ignoreCollapse
				lbl := g.Name
				pwG := ""
				if merge {
					lbl = g.Name + labelMergedMark
					pwG = g.Name
					emittedPw[g.Name] = true
				}
				items = append(items, groupHeaderEntry{
					label:      lbl,
					collapsed:  collapsed,
					defaultSec: false,
					groupIdx:   gi,
					pwGroup:    pwG,
				})
				if collapsed && !ignoreCollapse {
					continue
				}
				n := len(g.Hosts)
				for hi := range g.Hosts {
					h := &g.Hosts[hi]
					al := hostAlias(h)
					prefix := treeHostPrefix(hi, n)
					line := formatHostListLine(prefix, al, totalWidth)
					items = append(items, hostRowEntry{
						title: line,
						ref:   scfg.HostRef{InDefault: false, GroupIdx: gi, HostIdx: hi},
					})
				}
				if merge {
					pwIdxs := grouped[g.Name]
					pn := len(pwIdxs)
					for pji, idx := range pwIdxs {
						ph := &ov.PasswordHosts[idx]
						prefix := treeHostPrefix(pji, pn)
						line := formatHostListLine(prefix, ph.Title(), totalWidth)
						items = append(items, passwordHostRowEntry{title: line, idx: idx})
					}
				}
			}
		} else {
			for gi := range cfg.Groups {
				g := &cfg.Groups[gi]
				collapsed := g.CollapsedByDefault && !ignoreCollapse
				items = append(items, groupHeaderEntry{
					label:      g.Name,
					collapsed:  collapsed,
					defaultSec: false,
					groupIdx:   gi,
				})
				if collapsed && !ignoreCollapse {
					continue
				}
				n := len(g.Hosts)
				for hi := range g.Hosts {
					h := &g.Hosts[hi]
					al := hostAlias(h)
					prefix := treeHostPrefix(hi, n)
					line := formatHostListLine(prefix, al, totalWidth)
					items = append(items, hostRowEntry{
						title: line,
						ref:   scfg.HostRef{InDefault: false, GroupIdx: gi, HostIdx: hi},
					})
				}
			}
		}
	} else if showPassword && pwTree {
		items = appendPasswordUngroupedAndNamed(items, ov, totalWidth, nil, passwordGroupHeaderLabel)
	}

	if showPassword && pwTree && mergedPwLayout {
		items = appendPasswordUngroupedAndNamed(items, ov, totalWidth, emittedPw, passwordGroupHeaderLabel)
	}

	return items
}

func newCompactListDelegate() list.DefaultDelegate {
	d := list.NewDefaultDelegate()
	d.ShowDescription = false
	d.SetSpacing(0)
	d.Styles.FilterMatch = filterMatchStyle
	d.Styles.NormalTitle = styleWithPaneBG(d.Styles.NormalTitle.Copy().Padding(0, 0, 0, 0))
	d.Styles.SelectedTitle = listSelectedTitleStyle
	d.Styles.DimmedTitle = styleWithPaneBG(d.Styles.DimmedTitle.Copy().Padding(0, 0, 0, 0))
	return d
}

func newDetailListDelegate() list.DefaultDelegate {
	d := list.NewDefaultDelegate()
	d.ShowDescription = true
	d.SetSpacing(0)
	d.Styles.FilterMatch = filterMatchStyle
	// Zero default left padding and border mismatch so selected rows don't shift horizontally.
	d.Styles.NormalTitle = styleWithPaneBG(d.Styles.NormalTitle.Copy().Padding(0, 0, 0, 0))
	d.Styles.NormalDesc = styleWithPaneBG(d.Styles.NormalDesc.Copy().Padding(0, 0, 0, 0))
	d.Styles.DimmedTitle = styleWithPaneBG(d.Styles.DimmedTitle.Copy().Padding(0, 0, 0, 0))
	d.Styles.DimmedDesc = styleWithPaneBG(d.Styles.DimmedDesc.Copy().Padding(0, 0, 0, 0))
	d.Styles.SelectedTitle = listSelectedTitleStyle
	d.Styles.SelectedDesc = listSelectedDescStyle
	return d
}

// hostTreeDelegate renders group headers with distinct typography; host rows use DefaultDelegate.
type hostTreeDelegate struct {
	inner list.DefaultDelegate
}

func newHostTreeDelegate() hostTreeDelegate {
	return hostTreeDelegate{inner: newCompactListDelegate()}
}

func (h hostTreeDelegate) Height() int { return h.inner.Height() }

func (h hostTreeDelegate) Spacing() int { return h.inner.Spacing() }

func (h hostTreeDelegate) Update(msg tea.Msg, m *list.Model) tea.Cmd { return h.inner.Update(msg, m) }

// renderHostTreeRow draws host rows with selection visible during filter editing (unlike bubbles DefaultDelegate).
// When fromMain is true, unmanaged styles are used instead of the delegate defaults.
func renderHostTreeRow(w io.Writer, m list.Model, index int, row hostRowEntry, s *list.DefaultItemStyles, fromMain bool) {
	title := row.Title()
	if m.Width() <= 0 {
		return
	}
	pl := s.NormalTitle.GetPaddingLeft()
	pr := s.NormalTitle.GetPaddingRight()
	textwidth := m.Width() - pl - pr
	if textwidth < 4 {
		textwidth = 4
	}
	title = ansi.Truncate(title, textwidth, listEllipsis)

	normalStyle := s.NormalTitle
	dimStyle := s.DimmedTitle
	if fromMain {
		normalStyle = unmanagedHostNormalStyle
		dimStyle = unmanagedHostDimStyle
	}

	var (
		matchedRunes []int
		emptyFilter  = m.FilterState() == list.Filtering && m.FilterValue() == ""
		isFiltered   = m.FilterState() == list.Filtering || m.FilterState() == list.FilterApplied
		isSelected   = index == m.Index()
	)
	if isFiltered && index < len(m.VisibleItems()) {
		matchedRunes = m.MatchesForItem(index)
	}

	if emptyFilter {
		_, _ = fmt.Fprint(w, dimStyle.Render(title))
		return
	}
	if isSelected {
		if isFiltered {
			unmatched := s.SelectedTitle.Inline(true)
			matched := unmatched.Copy().Inherit(s.FilterMatch)
			title = lipgloss.StyleRunes(title, matchedRunes, matched, unmatched)
		}
		_, _ = fmt.Fprint(w, s.SelectedTitle.Render(title))
		return
	}
	if isFiltered {
		unmatched := normalStyle.Inline(true)
		matched := unmatched.Copy().Inherit(s.FilterMatch)
		title = lipgloss.StyleRunes(title, matchedRunes, matched, unmatched)
	}
	_, _ = fmt.Fprint(w, normalStyle.Render(title))
}

func (h hostTreeDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	if _, ok := item.(sectionSepEntry); ok {
		_, _ = fmt.Fprint(w, "")
		return
	}
	if gh, ok := item.(groupHeaderEntry); ok {
		renderGroupHeaderRow(w, m, index, gh)
		return
	}
	if row, ok := item.(hostRowEntry); ok {
		renderHostTreeRow(w, m, index, row, &h.inner.Styles, row.ref.FromMain)
		return
	}
	h.inner.Render(w, m, index, item)
}

func renderGroupHeaderRow(w io.Writer, m list.Model, index int, gh groupHeaderEntry) {
	title := gh.Title()
	if m.Width() <= 0 {
		return
	}
	textwidth := uint(m.Width())
	title = ansi.Truncate(title, int(textwidth), listEllipsis)

	normalSt := groupHeaderNormalStyle
	selectedSt := groupHeaderSelectedStyle
	dimSt := groupHeaderDimStyle
	if gh.unmanaged {
		normalSt = unmanagedGroupHeaderNormalStyle
		selectedSt = unmanagedGroupHeaderSelectedStyle
		dimSt = unmanagedGroupHeaderDimStyle
	}

	var (
		matchedRunes []int
		emptyFilter  = m.FilterState() == list.Filtering && m.FilterValue() == ""
		isFiltered   = m.FilterState() == list.Filtering || m.FilterState() == list.FilterApplied
		isSelected   = index == m.Index()
	)

	if isFiltered && index < len(m.VisibleItems()) {
		matchedRunes = m.MatchesForItem(index)
	}

	if emptyFilter {
		title = dimSt.Render(title)
	} else if isSelected {
		if isFiltered {
			unmatched := selectedSt.Inline(true)
			matched := unmatched.Copy().Inherit(filterMatchStyle)
			title = lipgloss.StyleRunes(title, matchedRunes, matched, unmatched)
		}
		title = selectedSt.Render(title)
	} else {
		if isFiltered {
			unmatched := normalSt.Inline(true)
			matched := unmatched.Copy().Inherit(filterMatchStyle)
			title = lipgloss.StyleRunes(title, matchedRunes, matched, unmatched)
		}
		title = normalSt.Render(title)
	}
	fmt.Fprint(w, title)
}

// appendMainCfgSection adds read-only host rows from the parent ssh_config.
func appendMainCfgSection(items []list.Item, mc *scfg.Config, totalWidth int, ignoreCollapse bool) []list.Item {
	if len(mc.DefaultHosts) > 0 {
		defCollapsed := mc.DefaultHostsCollapsed && !ignoreCollapse
		items = append(items, groupHeaderEntry{
			label:      "🔒 unmanaged",
			collapsed:  defCollapsed,
			defaultSec: false,
			groupIdx:   -3,
			unmanaged:  true,
		})
		if !defCollapsed || ignoreCollapse {
			n := len(mc.DefaultHosts)
			for i := range mc.DefaultHosts {
				h := &mc.DefaultHosts[i]
				al := hostAlias(h)
				prefix := treeHostPrefix(i, n)
				line := formatHostListLine(prefix, al, totalWidth)
				items = append(items, hostRowEntry{
					title: line,
					ref:   scfg.HostRef{FromMain: true, InDefault: true, HostIdx: i},
				})
			}
		}
	}

	for gi := range mc.Groups {
		g := &mc.Groups[gi]
		collapsed := g.CollapsedByDefault && !ignoreCollapse
		items = append(items, groupHeaderEntry{
			label:      g.Name + " 🔒",
			collapsed:  collapsed,
			defaultSec: false,
			groupIdx:   -3,
			unmanaged:  true,
		})
		if collapsed && !ignoreCollapse {
			continue
		}
		n := len(g.Hosts)
		for hi := range g.Hosts {
			h := &g.Hosts[hi]
			al := hostAlias(h)
			prefix := treeHostPrefix(hi, n)
			line := formatHostListLine(prefix, al, totalWidth)
			items = append(items, hostRowEntry{
				title: line,
				ref:   scfg.HostRef{FromMain: true, InDefault: false, GroupIdx: gi, HostIdx: hi},
			})
		}
	}

	// Blank row separates unmanaged from managed sections.
	items = append(items, sectionSepEntry{})
	return items
}
