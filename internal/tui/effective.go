package tui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	scfg "github.com/sacckth/sshui/internal/config"
)

// sshProbeTimeout bounds ssh -G runs; Match exec in a config can run commands.
const sshProbeTimeout = 3 * time.Second

func runSSHG(args ...string) ([]byte, error) {
	path, err := exec.LookPath("ssh")
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), sshProbeTimeout)
	defer cancel()
	return exec.CommandContext(ctx, path, append([]string{"-G"}, args...)...).CombinedOutput()
}

// sshCheckConfig asks ssh to parse content as a config file and returns its
// complaints. It returns nil when ssh is unavailable or times out, so a
// missing or slow ssh never blocks saving.
func sshCheckConfig(content string) error {
	if _, err := exec.LookPath("ssh"); err != nil {
		return nil
	}
	f, err := os.CreateTemp("", "sshui-check-*.conf")
	if err != nil {
		return nil
	}
	defer os.Remove(f.Name())
	_, werr := f.WriteString(content)
	if cerr := f.Close(); werr != nil || cerr != nil {
		return nil
	}
	out, err := runSSHG("-F", f.Name(), "sshui-validate.invalid")
	var exitErr *exec.ExitError
	if err == nil || !errors.As(err, &exitErr) {
		return nil
	}
	var msgs []string
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(strings.ReplaceAll(line, f.Name(), "ssh_hosts"))
		if line == "" || strings.HasPrefix(line, "Pseudo-terminal") || strings.Contains(line, "terminating,") {
			continue
		}
		msgs = append(msgs, line)
	}
	if len(msgs) == 0 {
		return nil
	}
	return errors.New("ssh rejects config: " + strings.Join(msgs, "; "))
}

// effectiveSettings holds ssh -G output for one alias (keys lowercased, in order).
type effectiveSettings struct {
	kv  [][2]string
	err string
}

// loadEffective runs ssh -G for alias (cached until the files on disk change).
func (m *Model) loadEffective(alias string) {
	if _, ok := m.effective[alias]; ok {
		return
	}
	if m.effective == nil {
		m.effective = map[string]effectiveSettings{}
	}
	out, err := runSSHG(append(m.sshConfigArgs(), alias)...)
	var res effectiveSettings
	if err != nil {
		res.err = strings.TrimSpace(string(out))
		if res.err == "" {
			res.err = err.Error()
		}
	} else {
		for _, line := range strings.Split(string(out), "\n") {
			k, v, ok := strings.Cut(strings.TrimSpace(line), " ")
			if ok && !strings.HasPrefix(line, "Pseudo-terminal") {
				res.kv = append(res.kv, [2]string{k, v})
			}
		}
	}
	m.effective[alias] = res
}

// effectiveCompareKeys are single-valued options where a mismatch between the
// host's own directive and ssh -G means an earlier block takes precedence.
var effectiveCompareKeys = map[string]bool{"hostname": true, "user": true, "port": true, "proxyjump": true}

// effectiveLines renders the ssh -G view for h: core options plus every option
// h sets, flagging values that an earlier matching block overrides.
func (m Model) effectiveLines(h *scfg.HostBlock) []string {
	plan := sshConnectPlanFromHost(h)
	if plan.Invalid || plan.NeedWildcard {
		return []string{statusStyle.Render("Effective settings need a concrete host name (pattern " + fmt.Sprintf("%q", plan.FirstPattern) + ").")}
	}
	res, ok := m.effective[plan.DirectTarget]
	if !ok {
		return nil
	}
	if res.err != "" {
		return []string{errStyle.Render("ssh -G: " + res.err)}
	}
	own := map[string]string{}
	keys := []string{"hostname", "user", "port", "proxyjump"}
	for _, d := range h.Directives {
		k := strings.ToLower(d.Key)
		if _, seen := own[k]; !seen {
			own[k] = strings.TrimSpace(d.Value)
			keys = append(keys, k)
		}
	}
	var lines []string
	done := map[string]bool{}
	for _, k := range keys {
		if done[k] {
			continue
		}
		done[k] = true
		var vals []string
		for _, kv := range res.kv {
			if kv[0] == k {
				vals = append(vals, kv[1])
			}
		}
		if len(vals) == 0 {
			continue
		}
		line := k + " " + strings.Join(vals, ", ")
		if v, set := own[k]; set && effectiveCompareKeys[k] && !strings.ContainsAny(v, "%$") && !strings.EqualFold(v, vals[0]) {
			line += errStyle.Render(fmt.Sprintf("  ⚠ host sets %q (earlier block wins)", v))
		}
		lines = append(lines, line)
	}
	return lines
}
