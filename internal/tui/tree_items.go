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

	scfg "github.com/sacckth/sshui/internal/config"
)

const listEllipsis = "…"

// groupHeaderEntry is a section label in the host tree (collapse/expand via z).
type groupHeaderEntry struct {
	label     string
	collapsed bool
}

func (e groupHeaderEntry) Title() string {
	if e.collapsed {
		return "▸ " + e.label
	}
	return "▾ " + e.label
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

const hostListIndent = "  "

func hostListInnerWidth(totalWidth int) int {
	inner := totalWidth - runewidth.StringWidth(hostListIndent)
	if inner < 4 {
		inner = 4
	}
	return inner
}

// HostListColumnHeader aligns with formatHostListLine (single Host column).
func HostListColumnHeader(totalWidth int) string {
	inner := hostListInnerWidth(totalWidth)
	h := runewidth.Truncate("Host", inner, listEllipsis)
	return hostListIndent + h
}

func formatHostListLine(alias string, totalWidth int) string {
	inner := hostListInnerWidth(totalWidth)
	return hostListIndent + runewidth.Truncate(alias, inner, listEllipsis)
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

// buildHostItems builds tree rows. collapsed keys are group labels "(default)" or group name.
// When ignoreCollapse is true (active host filter), all groups are shown expanded for matching.
func buildHostItems(cfg *scfg.Config, totalWidth int, collapsed map[string]bool, ignoreCollapse bool) []list.Item {
	if totalWidth < 12 {
		totalWidth = 48
	}
	if collapsed == nil {
		collapsed = map[string]bool{}
	}
	var items []list.Item

	items = append(items, groupHeaderEntry{label: "(default)", collapsed: collapsed["(default)"]})
	if !collapsed["(default)"] || ignoreCollapse {
		for i := range cfg.DefaultHosts {
			h := &cfg.DefaultHosts[i]
			al := hostAlias(h)
			line := formatHostListLine(al, totalWidth)
			items = append(items, hostRowEntry{
				title: line,
				ref:   scfg.HostRef{InDefault: true, HostIdx: i},
			})
		}
	}

	for gi := range cfg.Groups {
		g := &cfg.Groups[gi]
		items = append(items, groupHeaderEntry{label: g.Name, collapsed: collapsed[g.Name]})
		if collapsed[g.Name] && !ignoreCollapse {
			continue
		}
		for hi := range g.Hosts {
			h := &g.Hosts[hi]
			al := hostAlias(h)
			line := formatHostListLine(al, totalWidth)
			items = append(items, hostRowEntry{
				title: line,
				ref:   scfg.HostRef{InDefault: false, GroupIdx: gi, HostIdx: hi},
			})
		}
	}
	return items
}

func newCompactListDelegate() list.DefaultDelegate {
	d := list.NewDefaultDelegate()
	d.ShowDescription = false
	d.SetSpacing(0)
	d.Styles.FilterMatch = filterMatchStyle
	d.Styles.NormalTitle = d.Styles.NormalTitle.Copy().Padding(0, 0, 0, 0)
	d.Styles.SelectedTitle = listSelectedTitleStyle
	d.Styles.DimmedTitle = d.Styles.DimmedTitle.Copy().Padding(0, 0, 0, 0)
	return d
}

func newDetailListDelegate() list.DefaultDelegate {
	d := list.NewDefaultDelegate()
	d.ShowDescription = true
	d.SetSpacing(0)
	d.Styles.FilterMatch = filterMatchStyle
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

func (h hostTreeDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	if gh, ok := item.(groupHeaderEntry); ok {
		renderGroupHeaderRow(w, m, index, gh)
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

	var (
		matchedRunes []int
		emptyFilter  = m.FilterState() == list.Filtering && m.FilterValue() == ""
		isFiltered   = m.FilterState() == list.Filtering || m.FilterState() == list.FilterApplied
		isSelected   = index == m.Index()
	)

	if isFiltered {
		matchedRunes = m.MatchesForItem(index)
	}

	if emptyFilter {
		title = groupHeaderDimStyle.Render(title)
	} else if isSelected && m.FilterState() != list.Filtering {
		if isFiltered {
			unmatched := groupHeaderSelectedStyle.Inline(true)
			matched := unmatched.Copy().Inherit(filterMatchStyle)
			title = lipgloss.StyleRunes(title, matchedRunes, matched, unmatched)
		}
		title = groupHeaderSelectedStyle.Render(title)
	} else {
		if isFiltered {
			unmatched := groupHeaderNormalStyle.Inline(true)
			matched := unmatched.Copy().Inherit(filterMatchStyle)
			title = lipgloss.StyleRunes(title, matchedRunes, matched, unmatched)
		}
		title = groupHeaderNormalStyle.Render(title)
	}
	fmt.Fprint(w, title)
}
