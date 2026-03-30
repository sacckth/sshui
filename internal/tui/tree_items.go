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

// hostRowEntry is one host row with Host patterns, HostName, and User columns.
type hostRowEntry struct {
	title  string
	ref    scfg.HostRef
	filter string
}

func (e hostRowEntry) Title() string             { return e.title }
func (e hostRowEntry) Description() string       { return "" }
func (e hostRowEntry) FilterValue() string       { return e.filter }

// groupPickItem is a target group for MoveHost.
type groupPickItem struct {
	label     string
	toDefault bool
	groupIdx  int
}

func (e groupPickItem) Title() string             { return e.label }
func (e groupPickItem) Description() string     { return "" }
func (e groupPickItem) FilterValue() string     { return e.label }

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

// columnWidths returns inner width (after indent) and three column visual widths.
func columnWidths(totalWidth int) (inner, lw, hw, uw int) {
	const indent = "  "
	inner = totalWidth - runewidth.StringWidth(indent)
	if inner < 28 {
		inner = 28
	}
	lw = inner * 30 / 100
	if lw < 10 {
		lw = 10
	}
	hw = inner * 44 / 100
	if hw < 14 {
		hw = 14
	}
	uw = inner - lw - hw - 2
	if uw < 8 {
		uw = 8
		hw = inner - lw - uw - 2
		if hw < 10 {
			hw = 10
		}
	}
	return inner, lw, hw, uw
}

// HostListColumnHeader aligns with formatHostLine columns (Host / HostName / User).
func HostListColumnHeader(totalWidth int) string {
	const indent = "  "
	_, lw, hw, uw := columnWidths(totalWidth)
	h1 := runewidth.Truncate("Host", lw, "…")
	h2 := runewidth.Truncate("HostName", hw, "…")
	h3 := runewidth.Truncate("User", uw, "…")
	return indent + padRightVisual(h1, lw) + " " + padRightVisual(h2, hw) + " " + padRightVisual(h3, uw)
}

func formatHostLine(alias, hostname, user string, totalWidth int) string {
	const indent = "  "
	_, lw, hw, uw := columnWidths(totalWidth)

	a := runewidth.Truncate(alias, lw, "…")
	hn := runewidth.Truncate(hostname, hw, "…")
	u := runewidth.Truncate(user, uw, "…")
	return indent + padRightVisual(a, lw) + " " + padRightVisual(hn, hw) + " " + padRightVisual(u, uw)
}

func padRightVisual(s string, w int) string {
	sw := runewidth.StringWidth(s)
	if sw >= w {
		return s
	}
	return s + strings.Repeat(" ", w-sw)
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
	if totalWidth < 48 {
		totalWidth = 80
	}
	var items []list.Item

	items = append(items, groupHeaderEntry{label: "(default)"})
	for i := range cfg.DefaultHosts {
		h := &cfg.DefaultHosts[i]
		hn := directiveValue(h, "HostName", "hostname")
		user := directiveValue(h, "User", "user")
		if hn == "" {
			hn = "—"
		}
		if user == "" {
			user = "—"
		}
		al := hostAlias(h)
		line := formatHostLine(al, hn, user, totalWidth)
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
			if hn == "" {
				hn = "—"
			}
			if user == "" {
				user = "—"
			}
			al := hostAlias(h)
			line := formatHostLine(al, hn, user, totalWidth)
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
	d.Styles.NormalTitle = d.Styles.NormalTitle.Copy().Padding(0, 0, 0, 0)
	d.Styles.SelectedTitle = d.Styles.SelectedTitle.Copy().Padding(0, 0, 0, 0)
	d.Styles.DimmedTitle = d.Styles.DimmedTitle.Copy().Padding(0, 0, 0, 0)
	return d
}
