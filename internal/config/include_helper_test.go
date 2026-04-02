package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIsSSHUIManaged(t *testing.T) {
	dir := t.TempDir()
	mainPath := filepath.Join(dir, "config")

	// File with marker.
	content := "#sshui-managed\nHost *\n    Include /tmp/ssh_hosts\n\nHost foo\n  HostName bar\n"
	if err := os.WriteFile(mainPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if !IsSSHUIManaged(mainPath) {
		t.Fatal("expected managed")
	}

	// File without marker.
	noMarker := filepath.Join(dir, "config2")
	if err := os.WriteFile(noMarker, []byte("Host foo\n  HostName bar\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if IsSSHUIManaged(noMarker) {
		t.Fatal("expected not managed")
	}

	// Missing file.
	if IsSSHUIManaged(filepath.Join(dir, "nope")) {
		t.Fatal("missing file should not be managed")
	}
}

func TestAppendInclude(t *testing.T) {
	dir := t.TempDir()
	mainPath := filepath.Join(dir, "config")
	target := filepath.Join(dir, "ssh_hosts")

	original := "Host foo\n  HostName bar\n"
	if err := os.WriteFile(mainPath, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := AppendInclude(mainPath, target); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(mainPath)
	content := string(data)
	if !strings.Contains(content, sshuiManagedMarker) {
		t.Fatalf("marker not found: %q", content)
	}
	if !strings.Contains(content, "Include "+target) {
		t.Fatalf("Include not found: %q", content)
	}
	if !strings.Contains(content, "Host *") {
		t.Fatalf("Host * wrapper not found: %q", content)
	}
	// Include should be at the end, after original content.
	markerIdx := strings.Index(content, sshuiManagedMarker)
	hostIdx := strings.Index(content, "Host foo")
	if markerIdx < hostIdx {
		t.Fatalf("marker should be after original content: %q", content)
	}

	// Backup should exist.
	bkp := hiddenBackupPath(mainPath)
	bkpData, err := os.ReadFile(bkp)
	if err != nil {
		t.Fatal("backup missing:", err)
	}
	if string(bkpData) != original {
		t.Fatalf("backup content: %q", string(bkpData))
	}
}

func TestAppendIncludeIdempotent(t *testing.T) {
	dir := t.TempDir()
	mainPath := filepath.Join(dir, "config")
	target := filepath.Join(dir, "ssh_hosts")

	// First call creates the marker.
	if err := AppendInclude(mainPath, target); err != nil {
		t.Fatal(err)
	}
	first, _ := os.ReadFile(mainPath)

	// Second call is a no-op.
	if err := AppendInclude(mainPath, target); err != nil {
		t.Fatal(err)
	}
	second, _ := os.ReadFile(mainPath)

	if string(first) != string(second) {
		t.Fatalf("second call changed file:\nfirst: %q\nsecond: %q", first, second)
	}
}

func TestAppendIncludeNewFile(t *testing.T) {
	dir := t.TempDir()
	mainPath := filepath.Join(dir, "config")
	target := filepath.Join(dir, "ssh_hosts")

	if err := AppendInclude(mainPath, target); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(mainPath)
	content := string(data)
	if !strings.Contains(content, sshuiManagedMarker) {
		t.Fatalf("marker missing: %q", content)
	}
	if !strings.Contains(content, "Include "+target) {
		t.Fatalf("content: %q", content)
	}
}

func TestEnsureSSHHostsFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "subdir", "ssh_hosts")

	if err := EnsureSSHHostsFile(p); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() != 0 {
		t.Fatalf("expected empty file, got %d bytes", info.Size())
	}
}

func TestExportHostsTo(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "ssh_hosts")

	src := &Config{
		DefaultHosts: []HostBlock{
			{Patterns: []string{"web1"}, Directives: []Directive{{Key: "HostName", Value: "10.0.0.1"}}},
		},
	}

	if err := ExportHostsTo(src, dst); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(dst)
	content := string(data)
	if !strings.Contains(content, "Host web1") {
		t.Fatalf("exported content missing Host: %q", content)
	}
	if !strings.Contains(content, "HostName 10.0.0.1") {
		t.Fatalf("exported content missing HostName: %q", content)
	}
}

func TestStripHostBlocks(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config")

	input := `# Global comment
ServerAliveInterval 60

#@group: servers
#@desc: production
#@host: the web server
Host web1
    HostName 10.0.0.1
    User admin

Host web2
    HostName 10.0.0.2

# Keep this comment
Include /etc/ssh/ssh_config.d/*
`
	if err := os.WriteFile(p, []byte(input), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := StripHostBlocks(p); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(p)
	content := string(data)

	if strings.Contains(content, "Host web1") {
		t.Fatalf("Host web1 should be stripped: %q", content)
	}
	if strings.Contains(content, "Host web2") {
		t.Fatalf("Host web2 should be stripped: %q", content)
	}
	if strings.Contains(content, "HostName 10.0.0.1") {
		t.Fatalf("HostName directive should be stripped: %q", content)
	}
	if strings.Contains(content, "#@group:") {
		t.Fatalf("sshclick metadata should be stripped: %q", content)
	}
	if strings.Contains(content, "#@host:") {
		t.Fatalf("sshclick host comment should be stripped: %q", content)
	}
	if !strings.Contains(content, "# Global comment") {
		t.Fatalf("global comment should remain: %q", content)
	}
	if !strings.Contains(content, "ServerAliveInterval 60") {
		t.Fatalf("global directive should remain: %q", content)
	}
	if !strings.Contains(content, "Include /etc/ssh/ssh_config.d/*") {
		t.Fatalf("Include should remain: %q", content)
	}
	if !strings.Contains(content, "# Keep this comment") {
		t.Fatalf("non-host comment should remain: %q", content)
	}

	// Backup should exist.
	bkp := hiddenBackupPath(p)
	bkpData, err := os.ReadFile(bkp)
	if err != nil {
		t.Fatal("backup missing:", err)
	}
	if string(bkpData) != input {
		t.Fatalf("backup should match original")
	}
}

func TestStripBridgeIncludes(t *testing.T) {
	dir := t.TempDir()
	sshHosts := filepath.Join(dir, "ssh_hosts")
	os.WriteFile(sshHosts, nil, 0o600)

	cfg := &Config{
		DefaultHosts: []HostBlock{
			{Patterns: []string{"*"}, Directives: []Directive{{Key: "Include", Value: sshHosts}}},
			{Patterns: []string{"prod"}, Directives: []Directive{{Key: "HostName", Value: "prod.example.com"}}},
		},
		Groups: []Group{
			{Name: "work", Hosts: []HostBlock{
				{Patterns: []string{"jump"}, Directives: []Directive{{Key: "HostName", Value: "jump.internal"}}},
			}},
		},
	}
	out := StripBridgeIncludes(cfg, sshHosts)
	if len(out.DefaultHosts) != 1 {
		t.Fatalf("expected 1 default host after strip, got %d", len(out.DefaultHosts))
	}
	if out.DefaultHosts[0].Patterns[0] != "prod" {
		t.Fatalf("wrong host kept: %v", out.DefaultHosts[0].Patterns)
	}
	if len(out.Groups) != 1 || len(out.Groups[0].Hosts) != 1 {
		t.Fatal("groups should be unchanged")
	}
}

func TestStripBridgeIncludesKeepsNonBridge(t *testing.T) {
	cfg := &Config{
		DefaultHosts: []HostBlock{
			{Patterns: []string{"*"}, Directives: []Directive{
				{Key: "Include", Value: "/etc/ssh/ssh_config.d/*"},
				{Key: "ServerAliveInterval", Value: "60"},
			}},
		},
	}
	out := StripBridgeIncludes(cfg, "/tmp/ssh_hosts")
	if len(out.DefaultHosts) != 1 {
		t.Fatal("host with mixed directives should be kept")
	}
}

func TestStripBridgeIncludesNil(t *testing.T) {
	if StripBridgeIncludes(nil, "/tmp/ssh_hosts") != nil {
		t.Fatal("nil cfg should return nil")
	}
}

func TestStripHostBlocksMissingFile(t *testing.T) {
	err := StripHostBlocks(filepath.Join(t.TempDir(), "nope"))
	if err != nil {
		t.Fatalf("missing file should be a no-op, got: %v", err)
	}
}

func TestStripHostBlocksEmpty(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config")
	os.WriteFile(p, []byte(""), 0o644)
	if err := StripHostBlocks(p); err != nil {
		t.Fatal(err)
	}
}
