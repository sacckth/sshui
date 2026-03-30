package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/sacckth/sshui/internal/appcfg"
	"github.com/sacckth/sshui/internal/config"
	"github.com/sacckth/sshui/internal/tui"
)

var (
	version = "0.1.0"
	cfgPath string
)

func main() {
	root := &cobra.Command{
		Use:   "sshui",
		Short: "Terminal UI for ~/.ssh/config (Bubble Tea)",
		Long:  "Edit OpenSSH client configuration with a keyboard-driven TUI. Backs up are recommended before save.",
		RunE:  run,
	}
	root.Flags().StringVar(&cfgPath, "config", "", "SSH config path (overrides $SSH_CONFIG and app config; default chain in README)")
	root.Version = version
	root.SetVersionTemplate("{{.Version}}\n")

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(cmd *cobra.Command, args []string) error {
	if len(args) > 0 {
		return fmt.Errorf("unexpected arguments: %v", args)
	}
	ac, err := appcfg.Load()
	if err != nil {
		return err
	}
	path := cfgPath
	if path == "" {
		path = os.Getenv("SSH_CONFIG")
	}
	if path == "" {
		path = ac.SSHConfig
	}
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		path = filepath.Join(home, ".ssh", "config")
	}
	path, err = filepath.Abs(path)
	if err != nil {
		return err
	}

	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read config: %w", err)
	}
	var cfg *config.Config
	if len(data) == 0 {
		cfg = &config.Config{}
	} else {
		cfg, err = config.Parse(strings.NewReader(string(data)))
		if err != nil {
			return fmt.Errorf("parse config: %w", err)
		}
	}

	p := tui.InitProgram(cfg, path, tui.Options{Theme: ac.Theme, Editor: ac.Editor})
	final, err := p.Run()
	if err != nil {
		return err
	}
	if final == nil {
		return nil
	}
	return nil
}
