package config

import (
	"strings"
	"testing"
)

func TestRemoveHostStanzaKeepsRestOfFile(t *testing.T) {
	src := `#sshui-managed
Include /home/u/.config/sshui/ssh_hosts

# personal boxes
#@host: the a box
Host a
    # old address
    HostName a.example
    User u
# keep me: about b

Host b
    HostName b.example
Match host b
    User x
`
	cfg, err := Parse(strings.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}
	var a *HostBlock
	for i := range cfg.DefaultHosts {
		if cfg.DefaultHosts[i].Patterns[0] == "a" {
			a = &cfg.DefaultHosts[i]
		}
	}
	if a == nil || a.SrcLine != 6 {
		t.Fatalf("host a SrcLine: %+v", a)
	}
	got, err := RemoveHostStanza(src, a.SrcLine, a.Patterns)
	if err != nil {
		t.Fatal(err)
	}
	want := `#sshui-managed
Include /home/u/.config/sshui/ssh_hosts

# personal boxes
# keep me: about b

Host b
    HostName b.example
Match host b
    User x
`
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestRemoveHostStanzaLastBlockStopsAtMatch(t *testing.T) {
	src := "Host a\n    User u\n\nHost b\n    User v\nMatch host b\n    User x\n"
	got, err := RemoveHostStanza(src, 4, []string{"b"})
	if err != nil {
		t.Fatal(err)
	}
	want := "Host a\n    User u\n\nMatch host b\n    User x\n"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestRemoveHostStanzaRejectsChangedFile(t *testing.T) {
	src := "Host a\n    User u\nHost b\n    User v\n"
	if _, err := RemoveHostStanza(src, 3, []string{"a"}); err == nil {
		t.Fatal("expected error when line no longer matches patterns")
	}
	if _, err := RemoveHostStanza(src, 99, []string{"a"}); err == nil {
		t.Fatal("expected out-of-range error")
	}
}
