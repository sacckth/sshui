package config

import (
	"fmt"
	"io"
	"strings"
)

// Write serializes cfg to w with stable formatting (reformats the file).
func Write(w io.Writer, cfg *Config) error {
	var b strings.Builder
	writeBlock := func(hb HostBlock) {
		for _, c := range hb.HostComments {
			b.WriteString(c)
			b.WriteByte('\n')
		}
		b.WriteString("Host ")
		b.WriteString(strings.Join(hb.Patterns, " "))
		b.WriteByte('\n')
		for _, d := range hb.Directives {
			line := "    " + d.Key
			if d.Value != "" {
				line += " " + d.Value
			}
			b.WriteString(line)
			b.WriteByte('\n')
		}
	}

	for _, hb := range cfg.DefaultHosts {
		writeBlock(hb)
		b.WriteByte('\n')
	}

	for _, g := range cfg.Groups {
		b.WriteString("#@group: ")
		b.WriteString(g.Name)
		b.WriteByte('\n')
		for _, desc := range g.Descriptions {
			b.WriteString(desc)
			b.WriteByte('\n')
		}
		for _, hb := range g.Hosts {
			writeBlock(hb)
			b.WriteByte('\n')
		}
	}

	_, err := io.WriteString(w, strings.TrimRight(b.String(), "\n"))
	if err != nil {
		return err
	}
	_, err = w.Write([]byte("\n"))
	return err
}

// String is Write to a string (for tests).
func String(cfg *Config) (string, error) {
	var b strings.Builder
	if err := Write(&b, cfg); err != nil {
		return "", err
	}
	return b.String(), nil
}

// Validate returns an error if a HostRef is out of range.
func (cfg *Config) ValidateRef(ref HostRef) error {
	if ref.InDefault {
		if ref.HostIdx < 0 || ref.HostIdx >= len(cfg.DefaultHosts) {
			return fmt.Errorf("invalid default host index %d", ref.HostIdx)
		}
		return nil
	}
	if ref.GroupIdx < 0 || ref.GroupIdx >= len(cfg.Groups) {
		return fmt.Errorf("invalid group index %d", ref.GroupIdx)
	}
	g := cfg.Groups[ref.GroupIdx]
	if ref.HostIdx < 0 || ref.HostIdx >= len(g.Hosts) {
		return fmt.Errorf("invalid host index %d in group %q", ref.HostIdx, g.Name)
	}
	return nil
}

// HostAt returns a pointer to the HostBlock for ref. Caller must not retain
// across mutations that reallocate slices.
func (cfg *Config) HostAt(ref HostRef) *HostBlock {
	if ref.InDefault {
		return &cfg.DefaultHosts[ref.HostIdx]
	}
	return &cfg.Groups[ref.GroupIdx].Hosts[ref.HostIdx]
}

// DeleteHost removes the host at ref.
func (cfg *Config) DeleteHost(ref HostRef) {
	if ref.InDefault {
		cfg.DefaultHosts = append(cfg.DefaultHosts[:ref.HostIdx], cfg.DefaultHosts[ref.HostIdx+1:]...)
		return
	}
	h := cfg.Groups[ref.GroupIdx].Hosts
	cfg.Groups[ref.GroupIdx].Hosts = append(h[:ref.HostIdx], h[ref.HostIdx+1:]...)
}

// DuplicateHost copies directives to a new Host block after ref with new patterns.
func (cfg *Config) DuplicateHost(ref HostRef, newPatterns []string) error {
	if err := cfg.ValidateRef(ref); err != nil {
		return err
	}
	src := cfg.HostAt(ref)
	dup := HostBlock{
		HostComments: append([]string(nil), src.HostComments...),
		Patterns:     append([]string(nil), newPatterns...),
		Directives:   make([]Directive, len(src.Directives)),
	}
	copy(dup.Directives, src.Directives)
	if ref.InDefault {
		i := ref.HostIdx + 1
		cfg.DefaultHosts = append(cfg.DefaultHosts[:i], append([]HostBlock{dup}, cfg.DefaultHosts[i:]...)...)
		return nil
	}
	g := &cfg.Groups[ref.GroupIdx]
	i := ref.HostIdx + 1
	g.Hosts = append(g.Hosts[:i], append([]HostBlock{dup}, g.Hosts[i:]...)...)
	return nil
}
