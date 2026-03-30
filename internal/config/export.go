package config

import (
	"encoding/json"
	"strings"
)

// ListRow is one host line for CLI list output.
type ListRow struct {
	Group    string
	Alias    string
	HostName string
	User     string
}

func directiveLookup(h *HostBlock, keys ...string) string {
	for _, want := range keys {
		for i := range h.Directives {
			if strings.EqualFold(h.Directives[i].Key, want) {
				return strings.TrimSpace(h.Directives[i].Value)
			}
		}
	}
	return ""
}

// ListRows enumerates hosts in tree order (default, then each group).
func (cfg *Config) ListRows() []ListRow {
	var rows []ListRow
	for i := range cfg.DefaultHosts {
		h := &cfg.DefaultHosts[i]
		rows = append(rows, ListRow{
			Group:    "(default)",
			Alias:    hostAliasForExport(h),
			HostName: firstOrDash(directiveLookup(h, "HostName", "hostname")),
			User:     firstOrDash(directiveLookup(h, "User", "user")),
		})
	}
	for gi := range cfg.Groups {
		g := cfg.Groups[gi].Name
		for hi := range cfg.Groups[gi].Hosts {
			h := &cfg.Groups[gi].Hosts[hi]
			rows = append(rows, ListRow{
				Group:    g,
				Alias:    hostAliasForExport(h),
				HostName: firstOrDash(directiveLookup(h, "HostName", "hostname")),
				User:     firstOrDash(directiveLookup(h, "User", "user")),
			})
		}
	}
	return rows
}

func hostAliasForExport(h *HostBlock) string {
	if len(h.Patterns) == 0 {
		return "(empty)"
	}
	return strings.Join(h.Patterns, " ")
}

func firstOrDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

// HostShowJSON is the JSON shape for `sshui show`.
type HostShowJSON struct {
	Group        string      `json:"group,omitempty"`
	Patterns     []string    `json:"patterns"`
	HostComments []string    `json:"host_comments,omitempty"`
	Directives   []Directive `json:"directives"`
}

// FindHostByAlias returns the first host whose patterns contain an exact match to name.
func (cfg *Config) FindHostByAlias(name string) (HostRef, *HostBlock, bool) {
	name = strings.TrimSpace(name)
	if name == "" {
		return HostRef{}, nil, false
	}
	for i := range cfg.DefaultHosts {
		if hostPatternsMatch(&cfg.DefaultHosts[i], name) {
			ref := HostRef{InDefault: true, HostIdx: i}
			return ref, &cfg.DefaultHosts[i], true
		}
	}
	for gi := range cfg.Groups {
		for hi := range cfg.Groups[gi].Hosts {
			h := &cfg.Groups[gi].Hosts[hi]
			if hostPatternsMatch(h, name) {
				ref := HostRef{InDefault: false, GroupIdx: gi, HostIdx: hi}
				return ref, h, true
			}
		}
	}
	return HostRef{}, nil, false
}

func hostPatternsMatch(h *HostBlock, name string) bool {
	for _, p := range h.Patterns {
		if strings.EqualFold(strings.TrimSpace(p), name) {
			return true
		}
	}
	return false
}

// MarshalHostJSON encodes a single host for show --json.
func MarshalHostJSON(group string, h *HostBlock) ([]byte, error) {
	v := HostShowJSON{
		Group:        group,
		Patterns:     append([]string(nil), h.Patterns...),
		HostComments: append([]string(nil), h.HostComments...),
		Directives:   append([]Directive(nil), h.Directives...),
	}
	return json.MarshalIndent(v, "", "  ")
}
