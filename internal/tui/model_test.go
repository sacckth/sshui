package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	scfg "github.com/sacckth/sshui/internal/config"
)

func keyRunes(s string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func mustParse(t *testing.T, s string) *scfg.Config {
	t.Helper()
	cfg, err := scfg.Parse(strings.NewReader(s))
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func isQuit(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.QuitMsg)
	return ok
}

func TestImportWithDeletePreservesMainConfig(t *testing.T) {
	dir := t.TempDir()
	hostsPath := filepath.Join(dir, "ssh_hosts")
	mainPath := filepath.Join(dir, "config")
	if err := os.WriteFile(hostsPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	mainSrc := "#sshui-managed\nInclude " + hostsPath + "\n\n# my comment\nHost a\n    HostName a.example\n\nHost b\n    HostName b.example\nMatch host b\n    User x\n"
	if err := os.WriteFile(mainPath, []byte(mainSrc), 0o600); err != nil {
		t.Fatal(err)
	}
	mainCfg := scfg.StripBridgeIncludes(mustParse(t, mainSrc), hostsPath, dir)

	m := New(&scfg.Config{}, hostsPath, Options{SSHHostsPath: hostsPath, MainSSHConfigPath: mainPath, MainConfig: mainCfg})
	m.selRef = scfg.HostRef{FromMain: true, InDefault: true, HostIdx: 0}
	m.actionReturnMode = modeTree
	m.startImportFromMain()
	if m.mode != modeConfirmImport {
		t.Fatalf("mode %v status %q", m.mode, m.status)
	}
	m.Update(keyRunes("y"))

	gotMain, _ := os.ReadFile(mainPath)
	wantMain := "#sshui-managed\nInclude " + hostsPath + "\n\n# my comment\n\nHost b\n    HostName b.example\nMatch host b\n    User x\n"
	if string(gotMain) != wantMain {
		t.Fatalf("main config:\n%s\nwant:\n%s", gotMain, wantMain)
	}
	gotHosts, _ := os.ReadFile(hostsPath)
	if !strings.Contains(string(gotHosts), "Host a\n    HostName a.example") {
		t.Fatalf("ssh_hosts missing imported host:\n%s", gotHosts)
	}
	if bkp, _ := os.ReadFile(filepath.Join(dir, ".config.bkp")); string(bkp) != mainSrc {
		t.Fatalf("backup:\n%s", bkp)
	}
}

func TestHostMetaEditKeepsSeparateLines(t *testing.T) {
	cfg := mustParse(t, "#@host: line one\n#@host: line two\nHost a\n    HostName a\n")
	m := New(cfg, "/nonexistent/ssh_hosts", Options{})
	m.openDetail(scfg.HostRef{InDefault: true, HostIdx: 0})
	m.editMode = true
	m.Update(keyRunes("i"))
	if m.mode != modeInputHostMeta {
		t.Fatalf("mode %v", m.mode)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlJ})
	m.Update(keyRunes("three"))
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got := cfg.DefaultHosts[0].HostComments
	want := []string{"#@host: line one", "#@host: line two", "#@host: three"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("comments %q want %q", got, want)
	}
}

func TestQuitAsksWhenDirty(t *testing.T) {
	m := New(mustParse(t, "Host a\n"), "/nonexistent/ssh_hosts", Options{})
	if _, cmd := m.Update(keyRunes("q")); !isQuit(cmd) {
		t.Fatal("clean model should quit immediately")
	}

	m.dirty = true
	if _, cmd := m.Update(keyRunes("q")); isQuit(cmd) || m.mode != modeConfirmQuit {
		t.Fatalf("dirty quit should ask; mode=%v", m.mode)
	}
	m.Update(keyRunes("n"))
	if m.mode != modeTree {
		t.Fatalf("n should return to tree, mode=%v", m.mode)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if _, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC}); !isQuit(cmd) {
		t.Fatal("second ctrl+c should quit")
	}
}
