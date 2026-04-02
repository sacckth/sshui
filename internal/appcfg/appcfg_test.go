package appcfg

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func sshuiConfigDirUnderHome(home string) string {
	if runtime.GOOS == "darwin" {
		return filepath.Join(home, "Library", "Application Support", "sshui")
	}
	return filepath.Join(home, ".config", "sshui")
}

func TestLoadMissingIsEmpty(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if runtime.GOOS != "windows" {
		// Linux/BSD respect XDG when set; darwin uses Application Support only.
		if runtime.GOOS != "darwin" {
			t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
		}
	}
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.SSHConfig != "" || c.Editor != "" || c.Theme != "" || c.SSHConfigGitMirror != "" {
		t.Fatalf("expected empty, got %+v", c)
	}
}

func TestLoadValid(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if runtime.GOOS != "windows" {
		if runtime.GOOS != "darwin" {
			t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
		}
	}
	dir := sshuiConfigDirUnderHome(home)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "config.toml")
	content := `ssh_config = "/tmp/ssh.conf"
editor = "vim"
theme = "warm"
ssh_config_git_mirror = "~/dotfiles/ssh/config"
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.SSHConfig != "/tmp/ssh.conf" || c.Editor != "vim" || c.Theme != "warm" ||
		c.SSHConfigGitMirror != "~/dotfiles/ssh/config" {
		t.Fatalf("got %+v", c)
	}
}

func TestExpandPathTilde(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	got, err := ExpandPath("~/foo/bar")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, "foo", "bar")
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestLoadValidWithHosts(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if runtime.GOOS != "windows" && runtime.GOOS != "darwin" {
		t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	}
	dir := sshuiConfigDirUnderHome(home)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "config.toml")
	content := `ssh_config = "/tmp/ssh.conf"

[hosts]
ssh_hosts_path = "~/.config/sshui/ssh_hosts"
password_overlay_path = "~/.config/sshui/pw.toml"
browse_mode = "openssh"
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.Hosts.SSHHostsPath != "~/.config/sshui/ssh_hosts" {
		t.Fatalf("ssh_hosts_path: %q", c.Hosts.SSHHostsPath)
	}
	if c.Hosts.PasswordOverlay != "~/.config/sshui/pw.toml" {
		t.Fatalf("password_overlay_path: %q", c.Hosts.PasswordOverlay)
	}
	if c.Hosts.BrowseMode != "openssh" {
		t.Fatalf("browse_mode: %q", c.Hosts.BrowseMode)
	}
}

func TestEffectiveBrowseMode(t *testing.T) {
	c := Config{}
	if c.EffectiveBrowseMode() != BrowseModeMerged {
		t.Fatalf("default: %q", c.EffectiveBrowseMode())
	}
	c.Hosts.BrowseMode = BrowseModePassword
	if c.EffectiveBrowseMode() != BrowseModePassword {
		t.Fatalf("password: %q", c.EffectiveBrowseMode())
	}
	c.Hosts.BrowseMode = "invalid"
	if c.EffectiveBrowseMode() != BrowseModeMerged {
		t.Fatalf("invalid fallback: %q", c.EffectiveBrowseMode())
	}
}

func TestEnsureIncludeEnabled(t *testing.T) {
	h := HostsConfig{}
	if !h.EnsureIncludeEnabled() {
		t.Fatal("nil pointer should default to true")
	}
	f := false
	h.EnsureInclude = &f
	if h.EnsureIncludeEnabled() {
		t.Fatal("should be false")
	}
	tr := true
	h.EnsureInclude = &tr
	if !h.EnsureIncludeEnabled() {
		t.Fatal("should be true")
	}
}

func TestConfigExists(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if runtime.GOOS != "windows" && runtime.GOOS != "darwin" {
		t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	}

	p, exists, err := ConfigExists()
	if err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Fatal("should not exist yet")
	}
	if p == "" {
		t.Fatal("path should not be empty")
	}

	// Create it via Save.
	c := &Config{SSHConfig: "/test"}
	if err := Save(c); err != nil {
		t.Fatal(err)
	}

	p2, exists2, err := ConfigExists()
	if err != nil {
		t.Fatal(err)
	}
	if !exists2 {
		t.Fatal("should exist after Save")
	}
	if p2 != p {
		t.Fatalf("paths differ: %q vs %q", p, p2)
	}
}

func TestSave(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if runtime.GOOS != "windows" && runtime.GOOS != "darwin" {
		t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	}

	c := &Config{
		SSHConfig: "/test",
		Hosts: HostsConfig{
			BrowseMode: BrowseModeOpenSSH,
		},
	}
	if err := Save(c); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.SSHConfig != "/test" {
		t.Fatalf("ssh_config: %q", loaded.SSHConfig)
	}
	if loaded.Hosts.BrowseMode != BrowseModeOpenSSH {
		t.Fatalf("browse_mode: %q", loaded.Hosts.BrowseMode)
	}
}

func TestResolveSSHHostsPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	c := Config{}
	p, err := c.ResolveSSHHostsPath()
	if err != nil {
		t.Fatal(err)
	}
	if p == "" {
		t.Fatal("empty path")
	}

	c.Hosts.SSHHostsPath = "~/custom/ssh_hosts"
	p, err = c.ResolveSSHHostsPath()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, "custom", "ssh_hosts")
	if p != want {
		t.Fatalf("got %q, want %q", p, want)
	}
}
