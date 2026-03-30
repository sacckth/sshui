package config

// Clone returns a deep copy of cfg (slice-backed data duplicated).
func Clone(cfg *Config) *Config {
	if cfg == nil {
		return nil
	}
	out := &Config{HasInclude: cfg.HasInclude}
	out.DefaultHosts = make([]HostBlock, len(cfg.DefaultHosts))
	for i := range cfg.DefaultHosts {
		out.DefaultHosts[i] = cloneHostBlock(cfg.DefaultHosts[i])
	}
	out.Groups = make([]Group, len(cfg.Groups))
	for i := range cfg.Groups {
		g := cfg.Groups[i]
		ng := Group{
			Name:         g.Name,
			Descriptions: append([]string(nil), g.Descriptions...),
			Hosts:        make([]HostBlock, len(g.Hosts)),
		}
		for j := range g.Hosts {
			ng.Hosts[j] = cloneHostBlock(g.Hosts[j])
		}
		out.Groups[i] = ng
	}
	return out
}
