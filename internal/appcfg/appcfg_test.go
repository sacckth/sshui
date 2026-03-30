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
