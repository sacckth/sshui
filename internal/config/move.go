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

func cloneHostBlock(h HostBlock) HostBlock {
	out := HostBlock{
		Patterns:   append([]string(nil), h.Patterns...),
		Directives: make([]Directive, len(h.Directives)),
	}
	copy(out.Directives, h.Directives)
	return out
}
