package overlay

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMissing(t *testing.T) {
	f, err := Load(filepath.Join(t.TempDir(), "nonexistent.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if f.Version != 1 {
		t.Fatalf("expected version 1, got %d", f.Version)
	}
	if len(f.PasswordHosts) != 0 {
		t.Fatalf("expected no hosts, got %d", len(f.PasswordHosts))
	}
}

func TestLoadEmpty(t *testing.T) {
	p := filepath.Join(t.TempDir(), "empty.toml")
	if err := os.WriteFile(p, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if f.Version != 1 || len(f.PasswordHosts) != 0 {
		t.Fatalf("unexpected: %+v", f)
	}
}

const testOverlay = `version = 1

[[password_host]]
patterns = ["legacy-box", "*.pw.example.com"]
hostname = "192.168.50.10"
user = "admin"
port = 22
askpass = "~/.local/bin/ssh-askpass-pass"
askpass_require = "force"

[[password_host]]
patterns = ["router-lan"]
hostname = "192.168.0.1"
user = "root"
port = 22
askpass = "~/.local/bin/ssh-askpass-op"
`

func TestLoadValid(t *testing.T) {
	p := filepath.Join(t.TempDir(), "overlay.toml")
	if err := os.WriteFile(p, []byte(testOverlay), 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if f.Version != 1 {
		t.Fatalf("version: got %d", f.Version)
	}
	if len(f.PasswordHosts) != 2 {
		t.Fatalf("hosts: got %d", len(f.PasswordHosts))
	}
	ph := f.PasswordHosts[0]
	if ph.Hostname != "192.168.50.10" || ph.User != "admin" || ph.Port != 22 {
		t.Fatalf("host 0: %+v", ph)
	}
	if len(ph.Patterns) != 2 || ph.Patterns[0] != "legacy-box" {
		t.Fatalf("patterns: %v", ph.Patterns)
	}
	if ph.AskpassRequire != "force" {
		t.Fatalf("askpass_require: %q", ph.AskpassRequire)
	}
}

func TestSaveRoundTrip(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "save.toml")
	f := &File{
		Version: 1,
		PasswordHosts: []PasswordHost{
			{Patterns: []string{"test"}, Hostname: "1.2.3.4", User: "u", Port: 2222, Askpass: "/bin/ap"},
		},
	}
	if err := Save(p, f); err != nil {
		t.Fatal(err)
	}
	f2, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(f2.PasswordHosts) != 1 {
		t.Fatalf("got %d hosts", len(f2.PasswordHosts))
	}
	ph := f2.PasswordHosts[0]
	if ph.Hostname != "1.2.3.4" || ph.User != "u" || ph.Port != 2222 {
		t.Fatalf("round trip: %+v", ph)
	}
}

func TestMatchHost(t *testing.T) {
	p := filepath.Join(t.TempDir(), "match.toml")
	if err := os.WriteFile(p, []byte(testOverlay), 0o644); err != nil {
		t.Fatal(err)
	}
	f, _ := Load(p)

	tests := []struct {
		alias string
		want  string // expected hostname or "" for no match
	}{
		{"legacy-box", "192.168.50.10"},
		{"LEGACY-BOX", "192.168.50.10"},
		{"foo.pw.example.com", "192.168.50.10"},
		{"router-lan", "192.168.0.1"},
		{"unknown", ""},
	}
	for _, tc := range tests {
		got := f.MatchHost(tc.alias)
		if tc.want == "" {
			if got != nil {
				t.Errorf("MatchHost(%q) = %+v, want nil", tc.alias, got)
			}
		} else {
			if got == nil || got.Hostname != tc.want {
				t.Errorf("MatchHost(%q) hostname = %v, want %q", tc.alias, got, tc.want)
			}
		}
	}
}

func TestEffectivePort(t *testing.T) {
	ph := PasswordHost{Port: 0}
	if ph.EffectivePort() != 22 {
		t.Fatalf("got %d", ph.EffectivePort())
	}
	ph.Port = 2222
	if ph.EffectivePort() != 2222 {
		t.Fatalf("got %d", ph.EffectivePort())
	}
}

func TestAskpassEnv(t *testing.T) {
	ph := PasswordHost{Askpass: "/bin/ap", AskpassRequire: "force"}
	t.Setenv("HOME", t.TempDir())
	env := ph.AskpassEnv()
	if len(env) != 2 {
		t.Fatalf("expected 2 env vars, got %d: %v", len(env), env)
	}
	if env[0] != "SSH_ASKPASS=/bin/ap" {
		t.Fatalf("env[0] = %q", env[0])
	}
	if env[1] != "SSH_ASKPASS_REQUIRE=force" {
		t.Fatalf("env[1] = %q", env[1])
	}
}

func TestAddGroup(t *testing.T) {
	f := &File{Version: 1}
	if err := f.AddGroup("servers"); err != nil {
		t.Fatal(err)
	}
	if len(f.Groups) != 1 || f.Groups[0] != "servers" {
		t.Fatalf("Groups = %v", f.Groups)
	}
	if err := f.AddGroup("servers"); err == nil {
		t.Fatal("expected duplicate error")
	}
	if err := f.AddGroup(""); err == nil {
		t.Fatal("expected empty name error")
	}
}

func TestGroupedHosts(t *testing.T) {
	f := &File{
		Version: 1,
		PasswordHosts: []PasswordHost{
			{Group: "", Patterns: []string{"a"}, Hostname: "h1"},
			{Group: "dev", Patterns: []string{"b"}, Hostname: "h2"},
			{Group: "dev", Patterns: []string{"c"}, Hostname: "h3"},
			{Group: "prod", Patterns: []string{"d"}, Hostname: "h4"},
		},
	}
	grouped := f.GroupedHosts()
	if len(grouped[""]) != 1 {
		t.Fatalf("ungrouped: %v", grouped[""])
	}
	if len(grouped["dev"]) != 2 {
		t.Fatalf("dev: %v", grouped["dev"])
	}
	if len(grouped["prod"]) != 1 {
		t.Fatalf("prod: %v", grouped["prod"])
	}
}

func TestOrderedGroups(t *testing.T) {
	f := &File{
		Version: 1,
		Groups:  []string{"prod", "dev"},
		PasswordHosts: []PasswordHost{
			{Group: "dev", Patterns: []string{"a"}, Hostname: "h1"},
			{Group: "staging", Patterns: []string{"b"}, Hostname: "h2"},
		},
	}
	ordered := f.OrderedGroups()
	// Should be: prod, dev (from explicit Groups), then staging (from hosts).
	if len(ordered) != 3 {
		t.Fatalf("expected 3, got %v", ordered)
	}
	if ordered[0] != "prod" || ordered[1] != "dev" || ordered[2] != "staging" {
		t.Fatalf("order: %v", ordered)
	}
}

func TestGroupRoundTrip(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "groups.toml")
	f := &File{
		Version: 1,
		Groups:  []string{"servers"},
		PasswordHosts: []PasswordHost{
			{Group: "servers", Patterns: []string{"web"}, Hostname: "10.0.0.1", Askpass: "/bin/ap"},
		},
	}
	if err := Save(p, f); err != nil {
		t.Fatal(err)
	}
	f2, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(f2.Groups) != 1 || f2.Groups[0] != "servers" {
		t.Fatalf("groups: %v", f2.Groups)
	}
	if f2.PasswordHosts[0].Group != "servers" {
		t.Fatalf("host group: %q", f2.PasswordHosts[0].Group)
	}
}

func TestDeletePasswordGroup(t *testing.T) {
	f := &File{
		Version: 1,
		Groups:  []string{"Hello1", "Other"},
		PasswordHosts: []PasswordHost{
			{Group: "Hello1", Patterns: []string{"a"}, Hostname: "h1"},
			{Group: "Hello1", Patterns: []string{"b"}, Hostname: "h2"},
			{Group: "Other", Patterns: []string{"c"}, Hostname: "h3"},
		},
	}
	if err := f.DeletePasswordGroup("Hello1"); err != nil {
		t.Fatal(err)
	}
	if len(f.Groups) != 1 || f.Groups[0] != "Other" {
		t.Fatalf("Groups = %v", f.Groups)
	}
	if f.PasswordHosts[0].Group != "" || f.PasswordHosts[1].Group != "" {
		t.Fatalf("expected ungrouped, got %q %q", f.PasswordHosts[0].Group, f.PasswordHosts[1].Group)
	}
	if f.PasswordHosts[2].Group != "Other" {
		t.Fatalf("other group: %q", f.PasswordHosts[2].Group)
	}
	if err := f.DeletePasswordGroup("nope"); err == nil {
		t.Fatal("expected error")
	}
}
