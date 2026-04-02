// Package appcfg loads optional sshui settings from ~/.config/sshui/config.toml (or OS config dir).
package appcfg

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"
)

// HostsConfig holds [hosts] settings for the sshui host file, password overlay,
// Include management, and browse mode.
type HostsConfig struct {
	SSHHostsPath    string `toml:"ssh_hosts_path"`        // OpenSSH file sshui edits (default ~/.config/sshui/ssh_hosts)
	PasswordOverlay string `toml:"password_overlay_path"` // password overlay TOML (default ~/.config/sshui/password_hosts.toml)
	EnsureInclude   *bool  `toml:"ensure_include"`        // append Include to main ssh_config on startup (default true)
	BrowseMode      string `toml:"browse_mode"`           // merged | openssh | password (default merged)
}

// EnsureIncludeEnabled returns the effective value of EnsureInclude (default true).
func (h *HostsConfig) EnsureIncludeEnabled() bool {
	if h.EnsureInclude == nil {
		return true
	}
	return *h.EnsureInclude
}

// Config is optional UI and path configuration. Omitted fields use CLI/env defaults.
type Config struct {
	SSHConfig          string      `toml:"ssh_config"`            // default SSH client config path
	Editor             string      `toml:"editor"`                // shell fragment, e.g. vim or `code --wait` (passed to sh -c)
	Theme              string      `toml:"theme"`                 // default | warm | muted
	SSHConfigGitMirror string      `toml:"ssh_config_git_mirror"` // optional: copy saved config here (e.g. dotfiles repo)
	Hosts              HostsConfig `toml:"hosts"`
}

// BrowseMode constants.
const (
	BrowseModeMerged   = "merged"
	BrowseModeOpenSSH  = "openssh"
	BrowseModePassword = "password"
)

// EffectiveBrowseMode returns the browse mode with a default of "merged".
func (c *Config) EffectiveBrowseMode() string {
	switch c.Hosts.BrowseMode {
	case BrowseModeMerged, BrowseModeOpenSSH, BrowseModePassword:
		return c.Hosts.BrowseMode
	default:
		return BrowseModeMerged
	}
}

// ResolveSSHHostsPath returns the absolute ssh_hosts path, defaulting to ~/.config/sshui/ssh_hosts.
func (c *Config) ResolveSSHHostsPath() (string, error) {
	p := c.Hosts.SSHHostsPath
	if p == "" {
		dir, err := os.UserConfigDir()
		if err != nil {
			return "", err
		}
		p = filepath.Join(dir, "sshui", "ssh_hosts")
	}
	return ExpandPath(p)
}

// ResolvePasswordOverlayPath returns the absolute overlay path, defaulting to ~/.config/sshui/password_hosts.toml.
func (c *Config) ResolvePasswordOverlayPath() (string, error) {
	p := c.Hosts.PasswordOverlay
	if p == "" {
		dir, err := os.UserConfigDir()
		if err != nil {
			return "", err
		}
		p = filepath.Join(dir, "sshui", "password_hosts.toml")
	}
	return ExpandPath(p)
}

// FilePath returns the path to config.toml (may not exist yet).
func FilePath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "sshui", "config.toml"), nil
}

// ConfigExists returns the path to config.toml and whether it exists on disk.
func ConfigExists() (string, bool, error) {
	p, err := FilePath()
	if err != nil {
		return "", false, err
	}
	_, serr := os.Stat(p)
	if serr == nil {
		return p, true, nil
	}
	if os.IsNotExist(serr) {
		return p, false, nil
	}
	return p, false, serr
}

// Load reads config.toml if present. A missing file yields a zero Config and nil error.
func Load() (Config, error) {
	var c Config
	p, err := FilePath()
	if err != nil {
		return c, err
	}
	b, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return c, nil
		}
		return c, fmt.Errorf("read %s: %w", p, err)
	}
	if err := toml.Unmarshal(b, &c); err != nil {
		return c, fmt.Errorf("parse %s: %w", p, err)
	}
	return c, nil
}

// Save writes the config back to config.toml (0600), creating parent dirs.
func Save(c *Config) error {
	p, err := FilePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(p), err)
	}
	b, err := toml.Marshal(c)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	return os.WriteFile(p, b, 0o600)
}
