package config

import (
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
