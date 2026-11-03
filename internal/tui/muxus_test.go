package tui

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func TestMuxusArgs(t *testing.T) {
	cases := []struct {
		target string
		alias  bool
		want   string
		err    bool
	}{
		{"edge-router", true, "--host edge-router", false},
		{"  web01.example.com ", true, "--host web01.example.com", false},
		{"admin@10.0.0.1:2222", false, "--connect admin@10.0.0.1:2222", false},
		{"[2001:db8::1]:830", false, "--connect [2001:db8::1]:830", false},
		{"fe80::1%eth0", false, "--connect fe80::1%eth0", false},
		{"", true, "", true},
		{"-oProxyCommand=x", false, "", true},
		{"a b", true, "", true},
		{"host;rm", false, "", true},
		{"win*", true, "", true},
	}
	for _, c := range cases {
		got, err := muxusArgs(c.target, c.alias)
		if c.err {
			if err == nil {
				t.Errorf("muxusArgs(%q) = %q, want error", c.target, got)
			}
			continue
		}
		if err != nil || strings.Join(got, " ") != c.want {
			t.Errorf("muxusArgs(%q, %v) = %q, %v; want %q", c.target, c.alias, got, err, c.want)
		}
	}
}

func TestActionMenuOpensMuxus(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a shell script as the fake muxus")
	}
	dir := t.TempDir()
	out := filepath.Join(dir, "args")
	fake := filepath.Join(dir, "muxus")
	script := "#!/bin/sh\nprintf '%s ' \"$@\" > " + out + ".tmp && mv " + out + ".tmp " + out + "\n"
	if err := os.WriteFile(fake, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	m := New(mustParse(t, "Host edge-router\n    HostName 10.0.0.1\n"), "/nonexistent/ssh_hosts", Options{Muxus: fake})
	m.openActionMenu(modeTree)
	for i, it := range m.actionMenuList.Items() {
		if it.(actionItem).id == "muxus" {
			m.actionMenuList.Select(i)
		}
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.mode != modeTree || !strings.Contains(m.status, "Opened in muxus") {
		t.Fatalf("mode %v status %q", m.mode, m.status)
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		if got, err := os.ReadFile(out); err == nil {
			if string(got) != "--host edge-router " {
				t.Fatalf("muxus args %q", got)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("fake muxus was not run")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestQuickKeyOpensMuxus(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a shell script as the fake muxus")
	}
	dir := t.TempDir()
	out := filepath.Join(dir, "args")
	fake := filepath.Join(dir, "muxus")
	script := "#!/bin/sh\nprintf '%s ' \"$@\" > " + out + ".tmp && mv " + out + ".tmp " + out + "\n"
	if err := os.WriteFile(fake, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	m := New(mustParse(t, "Host edge-router\n    HostName 10.0.0.1\n"), "/nonexistent/ssh_hosts", Options{Muxus: fake})
	for i, it := range m.hostList.Items() {
		if _, ok := it.(hostRowEntry); ok {
			m.hostList.Select(i)
		}
	}
	m.actionReturnMode = modeDetail
	m.Update(keyRunes("M"))
	if m.mode != modeTree || !strings.Contains(m.status, "Opened in muxus") {
		t.Fatalf("mode %v status %q", m.mode, m.status)
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		if got, err := os.ReadFile(out); err == nil {
			if string(got) != "--host edge-router " {
				t.Fatalf("muxus args %q", got)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("fake muxus was not run")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestResolveMuxusBinaryConfigured(t *testing.T) {
	got, err := resolveMuxusBinary("/opt/Muxus/muxus")
	if err != nil || got != "/opt/Muxus/muxus" {
		t.Fatalf("got %q, %v", got, err)
	}
}
