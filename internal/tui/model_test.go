package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	scfg "github.com/sacckth/sshui/internal/config"
	"github.com/sacckth/sshui/internal/overlay"
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

func TestFoldMainGroupWithPrefixName(t *testing.T) {
	main := mustParse(t, "#@group: prod\nHost a\n#@group: prod-eu\nHost b\n")
	m := New(&scfg.Config{}, "/nonexistent/ssh_hosts", Options{MainConfig: main})
	for _, it := range m.hostList.Items() {
		if gh, ok := it.(groupHeaderEntry); ok && gh.label == "prod-eu 🔒" {
			m.toggleFoldAt(gh)
		}
	}
	if main.Groups[0].CollapsedByDefault || !main.Groups[1].CollapsedByDefault {
		t.Fatalf("prod=%v prod-eu=%v", main.Groups[0].CollapsedByDefault, main.Groups[1].CollapsedByDefault)
	}
}

func TestReloadRefreshesPreview(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ssh_hosts")
	if err := os.WriteFile(path, []byte("Host a\n    HostName old.example\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	m := New(mustParse(t, "Host a\n    HostName old.example\n"), path, Options{})
	m.refreshDetailList()
	if err := os.WriteFile(path, []byte("Host a\n    HostName new.example\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	m.Update(keyRunes("r"))
	it, ok := m.detailList.Items()[0].(dirEntry)
	if !ok || it.d.Value != "new.example" {
		t.Fatalf("preview not refreshed: %+v", m.detailList.Items())
	}
}

func TestRawEditorErrorOrNoChangeKeepsModel(t *testing.T) {
	cfg := mustParse(t, "Host a\n")
	m := New(cfg, "/nonexistent/ssh_hosts", Options{})
	orig, _ := scfg.String(cfg)
	tmp := filepath.Join(t.TempDir(), "buf.conf")

	_ = os.WriteFile(tmp, []byte("Host changed\n"), 0o600)
	m.handleRawEditorFinished(rawEditorFinishedMsg{path: tmp, orig: orig, err: os.ErrInvalid})
	if m.dirty || m.cfg != cfg {
		t.Fatal("editor error must discard the buffer")
	}

	_ = os.WriteFile(tmp, []byte(orig), 0o600)
	m.handleRawEditorFinished(rawEditorFinishedMsg{path: tmp, orig: orig})
	if m.dirty || m.cfg != cfg {
		t.Fatal("unchanged buffer must not mark dirty")
	}

	_ = os.WriteFile(tmp, []byte("Host changed\n"), 0o600)
	m.handleRawEditorFinished(rawEditorFinishedMsg{path: tmp, orig: orig})
	if !m.dirty || m.cfg.DefaultHosts[0].Patterns[0] != "changed" {
		t.Fatal("edited buffer should replace the model")
	}
}

func TestOpenSSHCommandHonorsSFTPWithOverlayMatch(t *testing.T) {
	ov := &overlay.File{PasswordHosts: []overlay.PasswordHost{{Patterns: []string{"*"}, Hostname: "other", Askpass: "/bin/askpass"}}}
	m := New(&scfg.Config{}, "/nonexistent/ssh_hosts", Options{Overlay: ov})
	m.sshConnectPendingSFTP = true
	c := m.openSSHCommand("box")
	args := strings.Join(c.Args, " ")
	if !strings.Contains(args, "sftp box") || strings.Contains(args, "other") {
		t.Fatalf("args %q", args)
	}
	if !strings.Contains(strings.Join(c.Env, "\n"), "SSH_ASKPASS=/bin/askpass") {
		t.Fatal("matching overlay should still provide SSH_ASKPASS")
	}
}

func TestSSHConfigArgsForNonDefaultMainConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	m := New(&scfg.Config{}, "/x/ssh_hosts", Options{MainSSHConfigPath: filepath.Join(home, ".ssh", "config")})
	if args := m.sshConfigArgs(); args != nil {
		t.Fatalf("default config should not add -F: %v", args)
	}
	m.mainSSHConfigPath = "/work/my config"
	c := m.openSSHCommand("box")
	if !strings.HasSuffix(strings.Join(c.Args, " "), "ssh -F /work/my config box") {
		t.Fatalf("args %q", c.Args)
	}
	if got := m.sshCopyCommand("box"); got != "ssh -F '/work/my config' box" {
		t.Fatalf("copy command %q", got)
	}
}

func typeAndEnter(m *Model, s string) {
	m.valueInput.SetValue(s)
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
}

func TestRenamePatternsAndEnterEditsDirective(t *testing.T) {
	cfg := mustParse(t, "Host a\n    HostName a.example\n\nHost b\n")
	m := New(cfg, "/nonexistent/ssh_hosts", Options{})
	m.openDetail(scfg.HostRef{InDefault: true, HostIdx: 0})
	m.editMode = true

	m.Update(keyRunes("p"))
	if m.mode != modeInputHostPatterns || m.valueInput.Value() != "a" {
		t.Fatalf("mode %v value %q", m.mode, m.valueInput.Value())
	}
	typeAndEnter(m, "web b")
	if got := strings.Join(cfg.DefaultHosts[0].Patterns, " "); got != "web b" || !m.dirty {
		t.Fatalf("patterns %q dirty %v", got, m.dirty)
	}
	if !strings.Contains(m.status, "b also defined") {
		t.Fatalf("expected duplicate note, status %q", m.status)
	}

	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.mode != modeInputDirectiveValue || m.valueInput.Value() != "a.example" {
		t.Fatalf("enter should edit directive; mode %v value %q", m.mode, m.valueInput.Value())
	}
	typeAndEnter(m, "web.example")
	if cfg.DefaultHosts[0].Directives[0].Value != "web.example" {
		t.Fatalf("directives %+v", cfg.DefaultHosts[0].Directives)
	}
}

func TestNewHostGoesToSelectedGroupWithQuickFields(t *testing.T) {
	cfg := mustParse(t, "Host d\n#@group: work\nHost w1\n")
	m := New(cfg, "/nonexistent/ssh_hosts", Options{BrowseMode: "openssh"})
	for i, it := range m.hostList.Items() {
		if gh, ok := it.(groupHeaderEntry); ok && gh.groupIdx == 0 {
			m.hostList.Select(i)
		}
	}
	m.Update(keyRunes("n"))
	if m.mode != modeInputNewHost {
		t.Fatalf("mode %v", m.mode)
	}
	typeAndEnter(m, "w2")
	if m.mode != modeInputDirectiveValue || m.pendingDirectiveKey != "HostName" {
		t.Fatalf("expected HostName prompt, mode %v key %q", m.mode, m.pendingDirectiveKey)
	}
	typeAndEnter(m, "w2.example") // HostName
	typeAndEnter(m, "")           // User skipped
	typeAndEnter(m, "2222")       // Port
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.mode != modeDetail {
		t.Fatalf("esc should stop prompts, mode %v", m.mode)
	}
	g := cfg.Groups[0]
	if len(g.Hosts) != 2 || g.Hosts[1].Patterns[0] != "w2" {
		t.Fatalf("group hosts %+v", g.Hosts)
	}
	want := []scfg.Directive{{Key: "HostName", Value: "w2.example"}, {Key: "Port", Value: "2222"}}
	if len(g.Hosts[1].Directives) != 2 || g.Hosts[1].Directives[0] != want[0] || g.Hosts[1].Directives[1] != want[1] {
		t.Fatalf("directives %+v", g.Hosts[1].Directives)
	}
}

func TestReorderHostsAndDirectives(t *testing.T) {
	cfg := mustParse(t, "Host a\n    User u\n    Port 1\n\nHost b\n")
	m := New(cfg, "/nonexistent/ssh_hosts", Options{BrowseMode: "openssh"})
	for i, it := range m.hostList.Items() {
		if row, ok := it.(hostRowEntry); ok && row.ref.HostIdx == 1 {
			m.hostList.Select(i)
		}
	}
	m.editMode = true
	m.Update(keyRunes("K"))
	if cfg.DefaultHosts[0].Patterns[0] != "b" || m.selRef.HostIdx != 0 {
		t.Fatalf("hosts %+v selRef %+v", cfg.DefaultHosts, m.selRef)
	}

	m.openDetail(scfg.HostRef{InDefault: true, HostIdx: 1})
	m.detailList.Select(1)
	m.Update(keyRunes("K"))
	if cfg.DefaultHosts[1].Directives[0].Key != "Port" || m.detailList.Index() != 0 {
		t.Fatalf("directives %+v index %d", cfg.DefaultHosts[1].Directives, m.detailList.Index())
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
