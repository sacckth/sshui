package config

import "fmt"

// MoveHost removes the host at ref and appends it to the default section or to
// Groups[toGroupIdx]. Host block data is copied; ref is invalid after success.
func (cfg *Config) MoveHost(ref HostRef, toDefault bool, toGroupIdx int) error {
	if err := cfg.ValidateRef(ref); err != nil {
		return err
	}
	hb := cloneHostBlock(*cfg.HostAt(ref))
	cfg.DeleteHost(ref)

	if toDefault {
		cfg.DefaultHosts = append(cfg.DefaultHosts, hb)
		return nil
	}
	if toGroupIdx < 0 || toGroupIdx >= len(cfg.Groups) {
		return fmt.Errorf("invalid group index %d", toGroupIdx)
	}
	cfg.Groups[toGroupIdx].Hosts = append(cfg.Groups[toGroupIdx].Hosts, hb)
	return nil
}

// AppendHost adds hb at the end of the default section or Groups[groupIdx]
// and returns its ref.
func (cfg *Config) AppendHost(toDefault bool, groupIdx int, hb HostBlock) (HostRef, error) {
	if toDefault {
		cfg.DefaultHosts = append(cfg.DefaultHosts, hb)
		return HostRef{InDefault: true, HostIdx: len(cfg.DefaultHosts) - 1}, nil
	}
	if groupIdx < 0 || groupIdx >= len(cfg.Groups) {
		return HostRef{}, fmt.Errorf("invalid group index %d", groupIdx)
	}
	g := &cfg.Groups[groupIdx]
	g.Hosts = append(g.Hosts, hb)
	return HostRef{GroupIdx: groupIdx, HostIdx: len(g.Hosts) - 1}, nil
}

// ShiftHost swaps the host at ref with its neighbour delta (-1 up, +1 down)
// in the same section and returns the host's new ref. Order matters to
// OpenSSH: for each option the first matching Host block wins.
func (cfg *Config) ShiftHost(ref HostRef, delta int) (HostRef, error) {
	if err := cfg.ValidateRef(ref); err != nil {
		return ref, err
	}
	hosts := cfg.DefaultHosts
	if !ref.InDefault {
		hosts = cfg.Groups[ref.GroupIdx].Hosts
	}
	j := ref.HostIdx + delta
	if j < 0 || j >= len(hosts) {
		return ref, fmt.Errorf("already at the edge of its section")
	}
	hosts[ref.HostIdx], hosts[j] = hosts[j], hosts[ref.HostIdx]
	ref.HostIdx = j
	return ref, nil
}

// ShiftDirective swaps directive i with its neighbour delta (-1 up, +1 down)
// and returns the new index.
func (h *HostBlock) ShiftDirective(i, delta int) (int, error) {
	j := i + delta
	if i < 0 || i >= len(h.Directives) || j < 0 || j >= len(h.Directives) {
		return i, fmt.Errorf("cannot move directive further")
	}
	h.Directives[i], h.Directives[j] = h.Directives[j], h.Directives[i]
	return j, nil
}

func cloneHostBlock(h HostBlock) HostBlock {
	out := HostBlock{
		HostComments: append([]string(nil), h.HostComments...),
		Patterns:     append([]string(nil), h.Patterns...),
		Directives:   make([]Directive, len(h.Directives)),
		SrcLine:      h.SrcLine,
	}
	copy(out.Directives, h.Directives)
	return out
}
