package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	scfg "github.com/sacckth/sshui/internal/config"
	"github.com/sacckth/sshui/internal/sshkeywords"
)

// newHostQuickFields are prompted, in order, right after creating an OpenSSH host.
var newHostQuickFields = []string{"HostName", "User", "Port", "IdentityFile"}

// newHostTarget picks the managed section for a new host from the tree cursor:
// the group of a selected managed host or group header, else (default).
func (m *Model) newHostTarget() (toDefault bool, groupIdx int) {
	switch it := m.hostList.SelectedItem().(type) {
	case hostRowEntry:
		if !it.ref.FromMain && !it.ref.InDefault && it.ref.GroupIdx < len(m.cfg.Groups) {
			return false, it.ref.GroupIdx
		}
	case groupHeaderEntry:
		if !it.unmanaged && !it.defaultSec && it.groupIdx >= 0 && it.groupIdx < len(m.cfg.Groups) {
			return false, it.groupIdx
		}
	}
	return true, -1
}

// promptNextNewHostField asks for the next quick field of a new host, or ends
// the sequence in host detail.
func (m *Model) promptNextNewHostField() tea.Cmd {
	if len(m.newHostFields) == 0 {
		m.newHostPrompting = false
		m.mode = modeDetail
		m.refreshDetailList()
		return nil
	}
	k := m.newHostFields[0]
	m.newHostFields = m.newHostFields[1:]
	m.newHostPrompting = true
	m.returnAfterInput = modeDetail
	m.editDirectiveIndex = -1
	m.pendingDirectiveKey = k
	m.mode = modeInputDirectiveValue
	m.valueInput.SetValue("")
	m.valueInput.Placeholder = "Enter to skip"
	for _, e := range sshkeywords.Catalog {
		if e.Name == k && e.Hint != "" {
			m.valueInput.Placeholder = e.Hint + " (Enter to skip)"
		}
	}
	m.valueInput.Focus()
	return textinput.Blink
}

// duplicatePatternNote returns a status suffix naming literal patterns in pats
// that another host (managed or main config) also lists, or "".
func (m *Model) duplicatePatternNote(pats []string, self scfg.HostRef) string {
	var dups []string
	seen := map[string]bool{}
	check := func(c *scfg.Config, fromMain bool) {
		if c == nil {
			return
		}
		visit := func(ref scfg.HostRef, h *scfg.HostBlock) {
			if ref == self {
				return
			}
			for _, p := range h.Patterns {
				for _, want := range pats {
					if strings.ContainsAny(want, "*?!") || seen[want] || !strings.EqualFold(p, want) {
						continue
					}
					seen[want] = true
					dups = append(dups, want)
				}
			}
		}
		for i := range c.DefaultHosts {
			visit(scfg.HostRef{InDefault: true, HostIdx: i, FromMain: fromMain}, &c.DefaultHosts[i])
		}
		for gi := range c.Groups {
			for hi := range c.Groups[gi].Hosts {
				visit(scfg.HostRef{GroupIdx: gi, HostIdx: hi, FromMain: fromMain}, &c.Groups[gi].Hosts[hi])
			}
		}
	}
	check(m.cfg, false)
	check(m.mainCfg, true)
	if len(dups) == 0 {
		return ""
	}
	return fmt.Sprintf(" Note: %s also defined by another host; ssh uses the first match for each option.", strings.Join(dups, ", "))
}
