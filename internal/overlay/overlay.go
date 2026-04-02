// Package overlay handles the password_hosts.toml file — a TOML-based overlay
// that maps SSH Host patterns to askpass scripts for password authentication.
package overlay

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
	"github.com/sacckth/sshui/internal/appcfg"
)

// PasswordHost is one entry in the password overlay file.
type PasswordHost struct {
	Group          string   `toml:"group,omitempty"`
	Patterns       []string `toml:"patterns"`
	Hostname       string   `toml:"hostname"`
	User           string   `toml:"user"`
	Port           int      `toml:"port"`
	Askpass        string   `toml:"askpass"`
	AskpassRequire string   `toml:"askpass_require,omitempty"`
	Display        string   `toml:"display,omitempty"`
}

// Title returns a display string for the host (first pattern or hostname).
func (ph *PasswordHost) Title() string {
	if len(ph.Patterns) > 0 {
		return ph.Patterns[0]
	}
	return ph.Hostname
}

// PatternsString returns the patterns joined by space.
func (ph *PasswordHost) PatternsString() string {
	return strings.Join(ph.Patterns, " ")
}

// EffectivePort returns Port if set, else 22.
func (ph *PasswordHost) EffectivePort() int {
	if ph.Port <= 0 {
		return 22
	}
	return ph.Port
}

// File is the top-level structure of password_hosts.toml.
type File struct {
	Version       int            `toml:"version"`
	Groups        []string       `toml:"groups,omitempty"`
	PasswordHosts []PasswordHost `toml:"password_host"`
}

// AddGroup adds a named group if it doesn't already exist.
func (f *File) AddGroup(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("group name required")
	}
	for _, g := range f.Groups {
		if strings.EqualFold(g, name) {
			return fmt.Errorf("group %q already exists", name)
		}
	}
	f.Groups = append(f.Groups, name)
	return nil
}

// GroupedHosts returns a map of group name → indices into PasswordHosts.
// Hosts with no group go under "".
func (f *File) GroupedHosts() map[string][]int {
	m := make(map[string][]int)
	for i := range f.PasswordHosts {
		g := f.PasswordHosts[i].Group
		m[g] = append(m[g], i)
	}
	return m
}

// DeletePasswordGroup removes name from the explicit groups list and clears Group on
// all password_host entries that referenced it (hosts become ungrouped).
func (f *File) DeletePasswordGroup(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("group name required")
	}
	found := false
	var keep []string
	for _, g := range f.Groups {
		if g == name {
			found = true
			continue
		}
		keep = append(keep, g)
	}
	f.Groups = keep
	for i := range f.PasswordHosts {
		if f.PasswordHosts[i].Group == name {
			f.PasswordHosts[i].Group = ""
			found = true
		}
	}
	if !found {
		return fmt.Errorf("group %q not found", name)
	}
	return nil
}

// OrderedGroups returns group names in declaration order, including any
// groups that have hosts but aren't in the explicit Groups list.
func (f *File) OrderedGroups() []string {
	seen := make(map[string]bool)
	var out []string
	for _, g := range f.Groups {
		if !seen[g] {
			seen[g] = true
			out = append(out, g)
		}
	}
	for i := range f.PasswordHosts {
		g := f.PasswordHosts[i].Group
		if g != "" && !seen[g] {
			seen[g] = true
			out = append(out, g)
		}
	}
	return out
}

// Load reads the overlay file at path. A missing file yields an empty File and nil error.
func Load(path string) (*File, error) {
	var f File
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			f.Version = 1
			return &f, nil
		}
		return nil, fmt.Errorf("read overlay %s: %w", path, err)
	}
	if len(strings.TrimSpace(string(b))) == 0 {
		f.Version = 1
		return &f, nil
	}
	if err := toml.Unmarshal(b, &f); err != nil {
		return nil, fmt.Errorf("parse overlay %s: %w", path, err)
	}
	if f.Version == 0 {
		f.Version = 1
	}
	return &f, nil
}

// Save writes the overlay file at path (0600), creating parent dirs.
func Save(path string, f *File) error {
	if f.Version == 0 {
		f.Version = 1
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(path), err)
	}
	b, err := toml.Marshal(f)
	if err != nil {
		return fmt.Errorf("marshal overlay: %w", err)
	}
	return os.WriteFile(path, b, 0o600)
}

// MatchHost finds the first overlay entry whose patterns match the given host alias.
// Uses simple OpenSSH-style matching: exact match or fnmatch-style wildcard.
func (f *File) MatchHost(alias string) *PasswordHost {
	alias = strings.TrimSpace(alias)
	if alias == "" {
		return nil
	}
	for i := range f.PasswordHosts {
		for _, pat := range f.PasswordHosts[i].Patterns {
			if matchPattern(pat, alias) {
				return &f.PasswordHosts[i]
			}
		}
	}
	return nil
}

// matchPattern performs simple OpenSSH-style Host pattern matching:
// exact string match (case-insensitive) or filepath.Match-style glob.
func matchPattern(pattern, alias string) bool {
	pattern = strings.TrimSpace(pattern)
	if strings.EqualFold(pattern, alias) {
		return true
	}
	matched, _ := filepath.Match(strings.ToLower(pattern), strings.ToLower(alias))
	return matched
}

// AskpassEnv returns the child-process environment variables to set for SSH_ASKPASS
// when connecting to a password host. Returns nil if no askpass is configured.
func (ph *PasswordHost) AskpassEnv() []string {
	askpass, err := appcfg.ExpandPath(ph.Askpass)
	if err != nil || askpass == "" {
		return nil
	}
	env := []string{"SSH_ASKPASS=" + askpass}
	if ph.AskpassRequire != "" {
		env = append(env, "SSH_ASKPASS_REQUIRE="+ph.AskpassRequire)
	}
	if ph.Display != "" {
		env = append(env, "DISPLAY="+ph.Display)
	}
	return env
}
