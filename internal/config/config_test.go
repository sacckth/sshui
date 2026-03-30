package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseWriteRoundTrip(t *testing.T) {
	raw := `#@group: work
#@desc: office
Host prod
    HostName prod.example.com
    User deploy

Host jump
    HostName jump.example.com
`
	cfg, err := Parse(strings.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Groups) != 1 || cfg.Groups[0].Name != "work" {
		t.Fatalf("group: %+v", cfg.Groups)
	}
	if len(cfg.Groups[0].Hosts) != 2 {
		t.Fatalf("hosts: %d", len(cfg.Groups[0].Hosts))
	}
	out, err := String(cfg)
	if err != nil {
		t.Fatal(err)
	}
	cfg2, err := Parse(strings.NewReader(out))
	if err != nil {
		t.Fatal(err)
	}
	s2, _ := String(cfg2)
	if s2 != out {
		t.Fatalf("roundtrip mismatch:\n%s\nvs\n%s", out, s2)
	}
}

func TestMoveHostDefaultToGroup(t *testing.T) {
	cfg := &Config{
		DefaultHosts: []HostBlock{
			{Patterns: []string{"h1"}, Directives: []Directive{{Key: "HostName", Value: "a.example.com"}}},
		},
		Groups: []Group{
			{Name: "work", Hosts: []HostBlock{}},
		},
	}
	ref := HostRef{InDefault: true, HostIdx: 0}
	if err := cfg.MoveHost(ref, false, 0); err != nil {
		t.Fatal(err)
	}
	if len(cfg.DefaultHosts) != 0 || len(cfg.Groups[0].Hosts) != 1 {
		t.Fatalf("default=%d group0=%d", len(cfg.DefaultHosts), len(cfg.Groups[0].Hosts))
	}
	if cfg.Groups[0].Hosts[0].Patterns[0] != "h1" {
		t.Fatalf("host: %+v", cfg.Groups[0].Hosts[0])
	}
}

func TestMoveHostGroupToDefault(t *testing.T) {
	cfg := &Config{
		Groups: []Group{
			{Name: "work", Hosts: []HostBlock{
				{Patterns: []string{"srv"}, Directives: nil},
			}},
		},
	}
	ref := HostRef{InDefault: false, GroupIdx: 0, HostIdx: 0}
	if err := cfg.MoveHost(ref, true, -1); err != nil {
		t.Fatal(err)
	}
	if len(cfg.DefaultHosts) != 1 || len(cfg.Groups[0].Hosts) != 0 {
		t.Fatalf("default=%d group0=%d", len(cfg.DefaultHosts), len(cfg.Groups[0].Hosts))
	}
}

func TestAddDeleteRenameGroup(t *testing.T) {
	cfg := &Config{}
	if err := cfg.AddGroup("work"); err != nil {
		t.Fatal(err)
	}
	if err := cfg.AddGroup("work"); err == nil {
		t.Fatal("expected duplicate error")
	}
	if len(cfg.Groups) != 1 {
		t.Fatalf("groups %d", len(cfg.Groups))
	}
	cfg.Groups[0].Hosts = []HostBlock{{Patterns: []string{"h"}}}
	if err := cfg.DeleteGroupByName("work"); err != nil {
		t.Fatal(err)
	}
	if len(cfg.Groups) != 0 || len(cfg.DefaultHosts) != 1 {
		t.Fatalf("g=%d d=%d", len(cfg.Groups), len(cfg.DefaultHosts))
	}
	if err := cfg.DeleteGroupByName("(default)"); err == nil {
		t.Fatal("expected error deleting default")
	}
	cfg.Groups = []Group{{Name: "a", Hosts: nil}}
	if err := cfg.RenameGroup(0, "b"); err != nil {
		t.Fatal(err)
	}
	if cfg.Groups[0].Name != "b" {
		t.Fatal(cfg.Groups[0].Name)
	}
	if err := cfg.SetGroupDescription(0, "note"); err != nil {
		t.Fatal(err)
	}
	if len(cfg.Groups[0].Descriptions) != 1 || !strings.HasPrefix(cfg.Groups[0].Descriptions[0], "#@desc:") {
		t.Fatalf("%+v", cfg.Groups[0].Descriptions)
	}
}

func TestHostCommentsRoundTrip(t *testing.T) {
	raw := `#@group: lab
#@host: jump via bastion
Host serv1
    HostName 10.0.0.1
    User u
`
	cfg, err := Parse(strings.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Groups) != 1 || len(cfg.Groups[0].Hosts) != 1 {
		t.Fatalf("structure: %+v", cfg.Groups)
	}
	h := cfg.Groups[0].Hosts[0]
	if len(h.HostComments) != 1 || !strings.Contains(h.HostComments[0], "bastion") {
		t.Fatalf("host comments: %+v", h.HostComments)
	}
	out, err := String(cfg)
	if err != nil {
		t.Fatal(err)
	}
	cfg2, err := Parse(strings.NewReader(out))
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg2.Groups[0].Hosts[0].HostComments) != 1 {
		t.Fatalf("roundtrip lost comments: %+v", cfg2.Groups[0].Hosts[0].HostComments)
	}
}

func TestMergeIncludes(t *testing.T) {
	dir := t.TempDir()
	mainPath := filepath.Join(dir, "config")
	incPath := filepath.Join(dir, "extra.conf")
	mainContent := "Include " + incPath + "\nHost main\n    HostName m.example\n"
	incContent := "Host inc\n    HostName i.internal\n"
	if err := os.WriteFile(mainPath, []byte(mainContent), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(incPath, []byte(incContent), 0o600); err != nil {
		t.Fatal(err)
	}
	mainAbs, err := filepath.Abs(mainPath)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := Parse(strings.NewReader(mainContent))
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.HasInclude {
		t.Fatal("expected HasInclude")
	}
	merged := MergeIncludes(mainAbs, cfg)
	var found bool
	for _, g := range merged.Groups {
		if strings.HasPrefix(g.Name, "include:") && len(g.Hosts) > 0 {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected include:* group, groups=%+v", merged.Groups)
	}
}

func TestIncludeFlag(t *testing.T) {
	raw := `Include config.d/*
Host x
    HostName y
`
	cfg, err := Parse(strings.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.HasInclude {
		t.Fatal("expected HasInclude")
	}
}
