package tui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/sacckth/sshui/internal/appcfg"
)

// muxusArgs returns the muxus launch flags for an ssh target. A literal Host
// alias opens the saved host (--host); a target composed from a wildcard
// pattern is dialed ad hoc (--connect), still through ssh_config.
func muxusArgs(target string, alias bool) ([]string, error) {
	t := strings.TrimSpace(target)
	if t == "" {
		return nil, fmt.Errorf("empty target")
	}
	if !muxusTargetOK(t) {
		return nil, fmt.Errorf("muxus only accepts letters, digits and . _ - @ + : %% [ ] in a target, not %q", t)
	}
	if alias {
		return []string{"--host", t}, nil
	}
	return []string{"--connect", t}, nil
}

// muxusTargetOK mirrors the character set muxus accepts for launch targets;
// muxus silently ignores anything else, so reject it here with a message.
func muxusTargetOK(t string) bool {
	if strings.HasPrefix(t, "-") {
		return false
	}
	for _, r := range t {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case strings.ContainsRune("._-@+:%[]", r):
		default:
			return false
		}
	}
	return true
}

// resolveMuxusBinary returns the configured muxus path, else muxus on PATH,
// else the macOS app bundle executable.
func resolveMuxusBinary(configured string) (string, error) {
	if c := strings.TrimSpace(configured); c != "" {
		return appcfg.ExpandPath(c)
	}
	if p, err := exec.LookPath("muxus"); err == nil {
		return p, nil
	}
	if runtime.GOOS == "darwin" {
		candidates := []string{"/Applications/Muxus.app/Contents/MacOS/muxus"}
		if home, err := os.UserHomeDir(); err == nil {
			candidates = append(candidates, filepath.Join(home, "Applications", "Muxus.app", "Contents", "MacOS", "muxus"))
		}
		for _, p := range candidates {
			if _, err := os.Stat(p); err == nil {
				return p, nil
			}
		}
	}
	return "", fmt.Errorf("muxus not found; set muxus = \"/path/to/muxus\" in sshui config.toml")
}

// launchMuxus starts muxus in its own session so it outlives sshui and the
// terminal. A running muxus receives the request and the new process exits.
func launchMuxus(bin string, args []string) error {
	c := exec.Command(bin, args...)
	detachProcess(c)
	if err := c.Start(); err != nil {
		return err
	}
	go func() { _ = c.Wait() }()
	return nil
}

// quickMuxus is the M key: the action menu's muxus entry without the menu.
func (m *Model) quickMuxus(returnMode viewMode) (*Model, tea.Cmd) {
	m.actionReturnMode = returnMode
	return m.beginInteractiveSSH(returnMode, postWildOpenMuxus)
}

// finishMuxusOpen launches muxus for target and returns to the screen the action menu came from.
func (m *Model) finishMuxusOpen(target string, alias bool) (*Model, tea.Cmd) {
	m.status = m.openInMuxus(target, alias)
	m.sshConnectPendingNote = ""
	m.mode = m.actionReturnMode
	if m.mode == modeDetail || m.mode == modeTree {
		m.layoutDetailPanes()
		m.refreshDetailList()
	}
	return m, nil
}

func (m *Model) openInMuxus(target string, alias bool) string {
	args, err := muxusArgs(target, alias)
	if err != nil {
		return errStyle.Render("muxus: " + err.Error())
	}
	bin, err := resolveMuxusBinary(m.muxusPath)
	if err != nil {
		return errStyle.Render(err.Error())
	}
	if err := launchMuxus(bin, args); err != nil {
		return errStyle.Render("muxus: " + err.Error())
	}
	msg := "Opened in muxus: " + strings.Join(args, " ") + "."
	if m.dirty {
		msg += " Unsaved changes are not visible to muxus until you save (w)."
	}
	if len(m.sshConfigArgs()) > 0 {
		msg += " Note: muxus reads ~/.ssh/config, not " + m.mainSSHConfigPath + "."
	}
	return msg
}
