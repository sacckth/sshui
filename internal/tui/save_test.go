package tui

import (
	"path/filepath"
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
