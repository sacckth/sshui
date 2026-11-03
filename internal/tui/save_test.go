package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHiddenBackupPath(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	want := filepath.Join(dir, ".config.bkp")
	if g := hiddenBackupPath(cfg); g != want {
		t.Fatalf("hiddenBackupPath(%q) = %q, want %q", cfg, g, want)
	}
}

func TestSaveRefusesExternalEditUntilConfirmed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ssh_hosts")
	orig := "Host a\n    HostName a.example\n"
	if err := os.WriteFile(path, []byte(orig), 0o600); err != nil {
		t.Fatal(err)
	}
	m := New(mustParse(t, orig), path, Options{})
	m.cfg.DefaultHosts[0].Directives[0].Value = "mine.example"
	m.dirty = true

	external := "# kept by muxus\nHost a\n    HostName theirs.example\n"
	if err := os.WriteFile(path, []byte(external), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := m.save(); err == nil || !strings.Contains(err.Error(), "changed on disk") {
		t.Fatalf("first save: err = %v, want changed-on-disk", err)
	}
	if got, _ := os.ReadFile(path); string(got) != external {
		t.Fatalf("file overwritten after refused save:\n%s", got)
	}

	if err := m.save(); err != nil {
		t.Fatalf("confirmed save: %v", err)
	}
	if got, _ := os.ReadFile(path); !strings.Contains(string(got), "mine.example") {
		t.Fatalf("confirmed save did not write model:\n%s", got)
	}
	if bkp, _ := os.ReadFile(hiddenBackupPath(path)); string(bkp) != external {
		t.Fatalf("backup should hold the external version:\n%s", bkp)
	}

	if err := m.save(); err != nil {
		t.Fatalf("save after own write: %v", err)
	}
}

func TestSaveConfirmationDoesNotCoverLaterEdit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ssh_hosts")
	if err := os.WriteFile(path, []byte("Host a\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	m := New(mustParse(t, "Host a\n"), path, Options{})
	_ = os.WriteFile(path, []byte("Host b\n"), 0o600)
	if err := m.save(); err == nil {
		t.Fatal("want conflict")
	}
	_ = os.WriteFile(path, []byte("Host c\n"), 0o600)
	if err := m.save(); err == nil {
		t.Fatal("a second, different external edit must be refused again")
	}
}

func TestReloadClearsSaveConflict(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ssh_hosts")
	if err := os.WriteFile(path, []byte("Host a\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	m := New(mustParse(t, "Host a\n"), path, Options{})
	_ = os.WriteFile(path, []byte("Host b\n"), 0o600)
	m.Update(keyRunes("r"))
	if err := m.save(); err != nil {
		t.Fatalf("save after reload: %v", err)
	}
}

func TestSaveCreatesMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ssh_hosts")
	m := New(mustParse(t, "Host a\n"), path, Options{})
	if err := m.save(); err != nil {
		t.Fatalf("save new file: %v", err)
	}
}
