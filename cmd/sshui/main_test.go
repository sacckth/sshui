package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func writeTestConfigs(t *testing.T) (string, string) {
	t.Helper()

	tmpDir := t.TempDir()
	homeDir := filepath.Join(tmpDir, "home")
	mainConfigPath := filepath.Join(homeDir, ".ssh", "config")

	t.Setenv("HOME", homeDir)
	configDir, err := os.UserConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	managedPath := filepath.Join(configDir, "sshui", "ssh_hosts")

	if err := os.MkdirAll(filepath.Dir(managedPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(mainConfigPath), 0o755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("SSH_CONFIG", mainConfigPath)

	if err := os.WriteFile(managedPath, []byte("Host managed\n    HostName managed.example.com\n    User managed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mainConfigPath, []byte("Host main\n    HostName main.example.com\n    User main\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	return managedPath, mainConfigPath
}

func captureStdout(t *testing.T, fn func() error) (string, error) {
	t.Helper()

	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	defer func() {
		os.Stdout = oldStdout
	}()

	runErr := fn()

	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(out), runErr
}

func TestRunListUsesManagedSSHHosts(t *testing.T) {
	_, _ = writeTestConfigs(t)
	output, runErr := captureStdout(t, func() error {
		return runList(&cobra.Command{}, nil)
	})

	if runErr != nil {
		t.Fatal(runErr)
	}
	if !strings.Contains(output, "managed\tmanaged.example.com\tmanaged") {
		t.Fatalf("expected managed host in output, got %q", output)
	}
	if strings.Contains(output, "main.example.com") {
		t.Fatalf("unexpected main config host in output, got %q", output)
	}
}

func TestRunDumpUsesManagedSSHHosts(t *testing.T) {
	_, _ = writeTestConfigs(t)
	output, runErr := captureStdout(t, func() error {
		return runDump(&cobra.Command{}, nil)
	})

	if runErr != nil {
		t.Fatal(runErr)
	}
	if !strings.Contains(output, "managed.example.com") {
		t.Fatalf("expected managed host in output, got %q", output)
	}
	if strings.Contains(output, "main.example.com") {
		t.Fatalf("unexpected main config host in output, got %q", output)
	}
}

func TestRunShowUsesManagedSSHHosts(t *testing.T) {
	_, _ = writeTestConfigs(t)
	output, runErr := captureStdout(t, func() error {
		return runShow(&cobra.Command{}, []string{"managed"})
	})

	if runErr != nil {
		t.Fatal(runErr)
	}
	if !strings.Contains(output, "managed.example.com") {
		t.Fatalf("expected managed host in output, got %q", output)
	}
	if strings.Contains(output, "main.example.com") {
		t.Fatalf("unexpected main config host in output, got %q", output)
	}
}
