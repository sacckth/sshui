package tui

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	scfg "github.com/sacckth/sshui/internal/config"
)

func requireSSH(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("ssh"); err != nil {
		t.Skip("ssh not installed")
	}
}

func TestSSHCheckConfigReportsBadOption(t *testing.T) {
	requireSSH(t)
	err := sshCheckConfig("Host other\n    HostNme typo.example\n")
	if err == nil || !strings.Contains(err.Error(), "ssh_hosts") || !strings.Contains(strings.ToLower(err.Error()), "hostnme") {
		t.Fatalf("err %v", err)
	}
	if err := sshCheckConfig("Host ok\n    HostName ok.example\n"); err != nil {
		t.Fatalf("valid config rejected: %v", err)
	}
}

func TestSaveCheckedRefusesOnceThenSaves(t *testing.T) {
	requireSSH(t)
	path := filepath.Join(t.TempDir(), "ssh_hosts")
	m := New(mustParse(t, "Host a\n    Port abc\n"), path, Options{})
	m.dirty = true
	if err := m.saveChecked(); err == nil {
		t.Fatal("expected ssh -G rejection")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("file must not be written on first rejection")
	}
	if err := m.saveChecked(); err != nil {
		t.Fatalf("second w should save anyway: %v", err)
	}
	if data, _ := os.ReadFile(path); !strings.Contains(string(data), "Port abc") {
		t.Fatalf("saved %q", data)
	}
}

func TestOverviewFlagsOverriddenValue(t *testing.T) {
	requireSSH(t)
	t.Setenv("HOME", t.TempDir())
	src := "Host *\n    User first\n\nHost box\n    User second\n    HostName box.example\n"
	mainPath := filepath.Join(t.TempDir(), "config")
	if err := os.WriteFile(mainPath, []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	m := New(mustParse(t, src), mainPath, Options{MainSSHConfigPath: mainPath})
	m.openDetail(scfg.HostRef{InDefault: true, HostIdx: 1})
	m.Update(keyRunes("t")) // Overview is reached after Directives → Connectivity → Overview
	m.Update(keyRunes("t"))
	if m.detailTab != detailTabOverview {
		t.Fatalf("tab %v", m.detailTab)
	}
	panel := m.overviewPanel()
	if !strings.Contains(panel, "hostname box.example") || !strings.Contains(panel, `host sets "second"`) {
		t.Fatalf("overview:\n%s", panel)
	}
}
