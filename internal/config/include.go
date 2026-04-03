package config

import (
	"os"
	"path/filepath"
	"strings"
)

const maxIncludeDepth = 8

// MergeIncludes returns a copy of root with additional synthetic groups
// (name prefix "include:") for hosts loaded from Include targets. mainAbs must
// be an absolute path to the file root was parsed from. visited prevents cycles.
// Missing or unreadable include files are skipped.
func MergeIncludes(mainAbs string, root *Config) *Config {
	out := Clone(root)
	if out == nil {
		return &Config{}
	}
	visited := map[string]bool{}
	if ap, err := filepath.Abs(mainAbs); err == nil {
		visited[strings.ToLower(ap)] = true
	}
	appendIncludes(filepath.Dir(mainAbs), root, out, visited, 0)
	return out
}

func appendIncludes(baseDir string, cfg *Config, out *Config, visited map[string]bool, depth int) {
	if depth > maxIncludeDepth {
		return
	}
	for _, pat := range collectIncludePatterns(cfg) {
		for _, abs := range resolveIncludePattern(baseDir, pat) {
			key := strings.ToLower(abs)
			if visited[key] {
				continue // already loaded this file — breaks Include cycles
			}
			visited[key] = true
			data, err := os.ReadFile(abs)
			if err != nil {
				continue
			}
			sub, err := Parse(strings.NewReader(string(data)))
			if err != nil {
				continue
			}
			flat := flattenHostBlocks(sub)
			if len(flat) == 0 {
				continue
			}
			out.Groups = append(out.Groups, Group{
				Name:  "include:" + filepath.Base(abs),
				Hosts: flat,
			})
			appendIncludes(filepath.Dir(abs), sub, out, visited, depth+1)
		}
	}
}

func collectIncludePatterns(cfg *Config) []string {
	var out []string
	walk := func(hb *HostBlock) {
		for _, d := range hb.Directives {
			if strings.EqualFold(d.Key, "Include") {
				for _, f := range strings.Fields(d.Value) {
					if f != "" {
						out = append(out, f)
					}
				}
			}
		}
	}
	for i := range cfg.DefaultHosts {
		walk(&cfg.DefaultHosts[i])
	}
	for gi := range cfg.Groups {
		for hi := range cfg.Groups[gi].Hosts {
			walk(&cfg.Groups[gi].Hosts[hi])
		}
	}
	return out
}

func resolveIncludePattern(baseDir, pattern string) []string {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return nil
	}
	pattern = expandTilde(pattern)
	var candidates []string
	if filepath.IsAbs(pattern) {
		candidates = append(candidates, pattern)
	} else {
		candidates = append(candidates, filepath.Join(baseDir, pattern))
	}
	var matches []string
	seen := map[string]bool{}
	for _, c := range candidates {
		list, err := filepath.Glob(c)
		if err != nil || len(list) == 0 {
			// try as single file
			if st, err := os.Stat(c); err == nil && !st.IsDir() {
				if ap, err := filepath.Abs(c); err == nil && !seen[ap] {
					seen[ap] = true
					matches = append(matches, ap)
				}
			}
			continue
		}
		for _, m := range list {
			if st, err := os.Stat(m); err != nil || st.IsDir() {
				continue
			}
			ap, err := filepath.Abs(m)
			if err != nil {
				continue
			}
			if !seen[ap] {
				seen[ap] = true
				matches = append(matches, ap)
			}
		}
	}
	return matches
}

func expandTilde(s string) string {
	if !strings.HasPrefix(s, "~") {
		return s
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return s
	}
	if s == "~" {
		return home
	}
	if strings.HasPrefix(s, "~/") {
		return filepath.Join(home, s[2:])
	}
	return s
}

// StripBridgeIncludes returns a clone of cfg with HostBlocks removed whose
// only directives are Include(s) that resolve to targetAbs (typically the
// managed ssh_hosts path). resolveBaseDir is the directory used to resolve
// relative Include patterns in cfg (the directory of the file cfg was parsed
// from, usually filepath.Dir(main_ssh_config)). If resolveBaseDir is empty,
// filepath.Dir(targetAbs) is used as a fallback.
//
// This prevents the sshui-managed Include bridge (file-scope Include or legacy
// Host * wrapper) from appearing as a visible host in the tree, and is also
// used when exporting hosts so Include-only stanzas that pull in the managed
// file or the main config are not copied (recursion).
func StripBridgeIncludes(cfg *Config, targetAbs string, resolveBaseDir string) *Config {
	if cfg == nil {
		return nil
	}
	if resolveBaseDir == "" {
		resolveBaseDir = filepath.Dir(targetAbs)
	}
	absTarget, err := filepath.Abs(targetAbs)
	if err != nil {
		absTarget = targetAbs
	}
	out := Clone(cfg)
	target := strings.ToLower(absTarget)
	out.DefaultHosts = filterBridgeBlocks(out.DefaultHosts, resolveBaseDir, target)
	for gi := range out.Groups {
		out.Groups[gi].Hosts = filterBridgeBlocks(out.Groups[gi].Hosts, resolveBaseDir, target)
	}
	return out
}

func filterBridgeBlocks(hosts []HostBlock, baseDir, targetLower string) []HostBlock {
	var kept []HostBlock
	for _, hb := range hosts {
		if isBridgeBlock(hb, baseDir, targetLower) {
			continue
		}
		kept = append(kept, hb)
	}
	return kept
}

func isBridgeBlock(hb HostBlock, baseDir, targetLower string) bool {
	if len(hb.Directives) == 0 {
		return false
	}
	for _, d := range hb.Directives {
		if !strings.EqualFold(d.Key, "Include") {
			return false
		}
		for _, f := range strings.Fields(d.Value) {
			resolved := resolveIncludePattern(baseDir, f)
			match := false
			for _, r := range resolved {
				if strings.ToLower(r) == targetLower {
					match = true
					break
				}
			}
			if !match {
				abs := expandTilde(f)
				if !filepath.IsAbs(abs) {
					abs = filepath.Join(baseDir, abs)
				}
				if ap, err := filepath.Abs(abs); err == nil && strings.ToLower(ap) == targetLower {
					match = true
				}
			}
			if !match {
				return false
			}
		}
	}
	return true
}

func flattenHostBlocks(cfg *Config) []HostBlock {
	var out []HostBlock
	for i := range cfg.DefaultHosts {
		out = append(out, cloneHostBlock(cfg.DefaultHosts[i]))
	}
	for gi := range cfg.Groups {
		for hi := range cfg.Groups[gi].Hosts {
			out = append(out, cloneHostBlock(cfg.Groups[gi].Hosts[hi]))
		}
	}
	return out
}
