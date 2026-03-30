package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/mattn/go-runewidth"

	scfg "github.com/sacckth/sshui/internal/config"
)

// groupHeaderEntry is a non-selectable section label in the host tree.
type groupHeaderEntry struct {
	label string
}

func (e groupHeaderEntry) Title() string       { return "▸ " + e.label }
func (e groupHeaderEntry) Description() string { return "" }
func (e groupHeaderEntry) FilterValue() string { return e.label }

// hostRowEntry is one host row showing Host patterns only (left pane).
type hostRowEntry struct {
	title  string
	ref    scfg.HostRef
	filter string
}

func (e hostRowEntry) Title() string       { return e.title }
func (e hostRowEntry) Description() string { return "" }
func (e hostRowEntry) FilterValue() string { return e.filter }

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
	h := runewidth.Truncate("Host", inner, "…")
	return hostListIndent + h
}

func formatHostListLine(alias string, totalWidth int) string {
	inner := hostListInnerWidth(totalWidth)
	return hostListIndent + runewidth.Truncate(alias, inner, "…")
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

func buildHostItems(cfg *scfg.Config, totalWidth int) []list.Item {
	if totalWidth < 12 {
		totalWidth = 48
	}
	var items []list.Item

	items = append(items, groupHeaderEntry{label: "(default)"})
	for i := range cfg.DefaultHosts {
		h := &cfg.DefaultHosts[i]
		hn := directiveValue(h, "HostName", "hostname")
		user := directiveValue(h, "User", "user")
		al := hostAlias(h)
		line := formatHostListLine(al, totalWidth)
		filter := strings.ToLower(al + " " + hn + " " + user + " default")
		items = append(items, hostRowEntry{
			title:  line,
			ref:    scfg.HostRef{InDefault: true, HostIdx: i},
			filter: filter,
		})
	}

	for gi := range cfg.Groups {
		g := &cfg.Groups[gi]
		items = append(items, groupHeaderEntry{label: g.Name})
		for hi := range g.Hosts {
			h := &g.Hosts[hi]
			hn := directiveValue(h, "HostName", "hostname")
			user := directiveValue(h, "User", "user")
			al := hostAlias(h)
			line := formatHostListLine(al, totalWidth)
			filter := strings.ToLower(al + " " + hn + " " + user + " " + g.Name)
			items = append(items, hostRowEntry{
				title:  line,
				ref:    scfg.HostRef{InDefault: false, GroupIdx: gi, HostIdx: hi},
				filter: filter,
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
	d.Styles.SelectedTitle = d.Styles.SelectedTitle.Copy().Padding(0, 0, 0, 0)
	d.Styles.DimmedTitle = d.Styles.DimmedTitle.Copy().Padding(0, 0, 0, 0)
	return d
}
