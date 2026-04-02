package main

import (
	"bytes"
	"encoding/json"
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
	version   = "0.1.3"
	cfgPath   string
	dumpJSON  bool
	dumpCheck bool
	showJSON  bool
)

func main() {
	root := &cobra.Command{
		Use:   "sshui",
		Short: "Terminal UI for ~/.ssh/config (Bubble Tea)",
		Long:  "Edit OpenSSH client configuration with a keyboard-driven TUI. Back up your config before save.",
		RunE:  runTUI,
	}
	root.Flags().StringVar(&cfgPath, "config", "", "SSH config path (overrides $SSH_CONFIG and app config; default chain in README)")
	root.Version = version
	root.SetVersionTemplate("{{.Version}}\n")

	dumpCmd := &cobra.Command{
		Use:   "dump",
		Short: "Print canonical serialized config to stdout",
		RunE:  runDump,
	}
	dumpCmd.Flags().BoolVar(&dumpJSON, "json", false, "emit JSON list of hosts (patterns + directives)")
	dumpCmd.Flags().BoolVar(&dumpCheck, "check", false, "exit 1 if on-disk file differs from canonical serialization")

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List hosts (tab-separated: group, alias, hostname, user)",
		RunE:  runList,
	}

	showCmd := &cobra.Command{
		Use:   "show HOST",
		Short: "Show one host block by alias (first matching Host pattern)",
		Args:  cobra.ExactArgs(1),
		RunE:  runShow,
	}
	showCmd.Flags().BoolVar(&showJSON, "json", false, "JSON output")

	completionCmd := &cobra.Command{
		Use:   "completion [bash|zsh|fish]",
		Short: "Generate shell completion script",
		Args:  cobra.ExactArgs(1),
		RunE:  runCompletion,
	}

	root.AddCommand(dumpCmd, listCmd, showCmd, completionCmd)

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func resolveSSHConfigPath(flag string, ac *appcfg.Config) (string, error) {
	path := strings.TrimSpace(flag)
	if path == "" {
		path = os.Getenv("SSH_CONFIG")
	}
	if path == "" {
		path = ac.SSHConfig
	}
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		path = filepath.Join(home, ".ssh", "config")
	}
	return filepath.Abs(path)
}

func loadParsedConfig(path string) (*config.Config, error) {
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("read config: %w", err)
	}
	if len(data) == 0 {
		return &config.Config{}, nil
	}
	return config.Parse(strings.NewReader(string(data)))
}

func runTUI(cmd *cobra.Command, args []string) error {
	if len(args) > 0 {
		return fmt.Errorf("unexpected arguments: %v (use subcommands for non-TUI actions)", args)
	}
	ac, err := appcfg.Load()
	if err != nil {
		return err
	}
	path, err := resolveSSHConfigPath(cfgPath, &ac)
	if err != nil {
		return err
	}

	baseCfg, err := loadParsedConfig(path)
	if err != nil {
		return err
	}

	displayCfg := baseCfg
	readOnly := baseCfg.HasInclude
	if readOnly {
		displayCfg = config.MergeIncludes(path, baseCfg)
	}

	mirrorPath := ""
	if ac.SSHConfigGitMirror != "" {
		mirrorPath, err = appcfg.ExpandPath(ac.SSHConfigGitMirror)
		if err != nil {
			return fmt.Errorf("ssh_config_git_mirror: %w", err)
		}
	}

	appToml, err := appcfg.FilePath()
	if err != nil {
		return err
	}

	p := tui.InitProgram(displayCfg, path, tui.Options{
		Theme:         ac.Theme,
		Editor:        ac.Editor,
		ReadOnly:      readOnly,
		MirrorPath:    mirrorPath,
		AppConfigPath: appToml,
	})
	final, err := p.Run()
	if err != nil {
		return err
	}
	if final == nil {
		return nil
	}
	return nil
}

func runDump(cmd *cobra.Command, args []string) error {
	ac, err := appcfg.Load()
	if err != nil {
		return err
	}
	path, err := resolveSSHConfigPath(cfgPath, &ac)
	if err != nil {
		return err
	}
	cfg, err := loadParsedConfig(path)
	if err != nil {
		return err
	}
	if dumpJSON {
		type row struct {
			Group        string             `json:"group"`
			Patterns     []string           `json:"patterns"`
			HostComments []string           `json:"host_comments,omitempty"`
			Directives   []config.Directive `json:"directives"`
		}
		var rows []row
		for i := range cfg.DefaultHosts {
			h := &cfg.DefaultHosts[i]
			rows = append(rows, row{
				Group:        "(default)",
				Patterns:     append([]string(nil), h.Patterns...),
				HostComments: append([]string(nil), h.HostComments...),
				Directives:   append([]config.Directive(nil), h.Directives...),
			})
		}
		for gi := range cfg.Groups {
			g := cfg.Groups[gi].Name
			for hi := range cfg.Groups[gi].Hosts {
				h := &cfg.Groups[gi].Hosts[hi]
				rows = append(rows, row{
					Group:        g,
					Patterns:     append([]string(nil), h.Patterns...),
					HostComments: append([]string(nil), h.HostComments...),
					Directives:   append([]config.Directive(nil), h.Directives...),
				})
			}
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(rows)
	}
	out, err := config.String(cfg)
	if err != nil {
		return err
	}
	if dumpCheck {
		disk, err := os.ReadFile(path)
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		canonical := []byte(out)
		if !bytes.Equal(bytes.TrimSpace(disk), bytes.TrimSpace(canonical)) {
			fmt.Fprintln(os.Stderr, "sshui: config differs from canonical serialization (run dump without --check to see)")
			os.Exit(1)
		}
		return nil
	}
	fmt.Print(out)
	return nil
}

func runList(cmd *cobra.Command, args []string) error {
	ac, err := appcfg.Load()
	if err != nil {
		return err
	}
	path, err := resolveSSHConfigPath(cfgPath, &ac)
	if err != nil {
		return err
	}
	cfg, err := loadParsedConfig(path)
	if err != nil {
		return err
	}
	for _, r := range cfg.ListRows() {
		fmt.Printf("%s\t%s\t%s\t%s\n", r.Group, r.Alias, r.HostName, r.User)
	}
	return nil
}

func runShow(cmd *cobra.Command, args []string) error {
	ac, err := appcfg.Load()
	if err != nil {
		return err
	}
	path, err := resolveSSHConfigPath(cfgPath, &ac)
	if err != nil {
		return err
	}
	cfg, err := loadParsedConfig(path)
	if err != nil {
		return err
	}
	ref, h, ok := cfg.FindHostByAlias(args[0])
	if !ok {
		return fmt.Errorf("no host matching %q", args[0])
	}
	group := "(default)"
	if !ref.InDefault && ref.GroupIdx >= 0 && ref.GroupIdx < len(cfg.Groups) {
		group = cfg.Groups[ref.GroupIdx].Name
	}
	if showJSON {
		b, err := config.MarshalHostJSON(group, h)
		if err != nil {
			return err
		}
		fmt.Println(string(b))
		return nil
	}
	fmt.Printf("Group: %s\nPatterns: %s\n", group, strings.Join(h.Patterns, " "))
	for _, line := range h.HostComments {
		fmt.Println(line)
	}
	for _, d := range h.Directives {
		if d.Value != "" {
			fmt.Printf("    %s %s\n", d.Key, d.Value)
		} else {
			fmt.Printf("    %s\n", d.Key)
		}
	}
	return nil
}

func runCompletion(cmd *cobra.Command, args []string) error {
	switch args[0] {
	case "bash":
		return cmd.Root().GenBashCompletion(os.Stdout)
	case "zsh":
		return cmd.Root().GenZshCompletion(os.Stdout)
	case "fish":
		return cmd.Root().GenFishCompletion(os.Stdout, true)
	default:
		return fmt.Errorf("unsupported shell %q", args[0])
	}
}
