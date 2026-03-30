// Package appcfg loads optional sshui settings from ~/.config/sshui/config.toml (or OS config dir).
package appcfg

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"
)

// Config is optional UI and path configuration. Omitted fields use CLI/env defaults.
type Config struct {
	SSHConfig          string `toml:"ssh_config"`            // default SSH client config path
	Editor             string `toml:"editor"`                // shell fragment, e.g. vim or `code --wait` (passed to sh -c)
	Theme              string `toml:"theme"`                 // default | warm | muted
	SSHConfigGitMirror string `toml:"ssh_config_git_mirror"` // optional: copy saved config here (e.g. dotfiles repo)
}

// FilePath returns the path to config.toml (may not exist yet).
func FilePath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "sshui", "config.toml"), nil
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
