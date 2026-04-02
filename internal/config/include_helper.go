package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const sshuiManagedMarker = "#sshui-managed"

// IsSSHUIManaged returns true if the file at mainPath contains the
// "#sshui-managed" metadata comment, indicating sshui has already set up
// Include for this file.
func IsSSHUIManaged(mainPath string) bool {
	f, err := os.Open(mainPath)
	if err != nil {
		return false
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		if strings.Contains(scanner.Text(), sshuiManagedMarker) {
			return true
		}
	}
	return false
}

// AppendInclude backs up mainPath, then appends an Include directive for
// targetAbs at the end of the file with the #sshui-managed marker.
// No-op if the marker is already present.
func AppendInclude(mainPath, targetAbs string) error {
	if IsSSHUIManaged(mainPath) {
		return nil
	}

	data, err := os.ReadFile(mainPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read %s: %w", mainPath, err)
	}

	if len(data) > 0 {
		bkp := hiddenBackupPath(mainPath)
		if err := os.WriteFile(bkp, data, 0o600); err != nil {
			return fmt.Errorf("backup %s: %w", bkp, err)
		}
	}

	footer := fmt.Sprintf("\n%s\nInclude %s\n", sshuiManagedMarker, targetAbs)

	var out []byte
	if len(data) == 0 {
		if err := os.MkdirAll(filepath.Dir(mainPath), 0o700); err != nil {
			return fmt.Errorf("mkdir %s: %w", filepath.Dir(mainPath), err)
		}
		out = []byte(fmt.Sprintf("%s\nInclude %s\n", sshuiManagedMarker, targetAbs))
	} else {
		s := string(data)
		if !strings.HasSuffix(s, "\n") {
			s += "\n"
		}
		out = []byte(s + footer)
	}
	return os.WriteFile(mainPath, out, 0o600)
}

// StripHostBlocks removes all Host blocks and sshclick metadata comments
// (#@group:, #@desc:, #@info:, #@host:) from the file at path.
// Backs up before writing. Collapses runs of blank lines.
func StripHostBlocks(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read %s: %w", path, err)
	}
	if len(data) == 0 {
		return nil
	}

	bkp := hiddenBackupPath(path)
	if err := os.WriteFile(bkp, data, 0o600); err != nil {
		return fmt.Errorf("backup %s: %w", bkp, err)
	}

	lines := strings.Split(string(data), "\n")
	var kept []string
	inHost := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		lower := strings.ToLower(trimmed)

		if strings.HasPrefix(lower, "host ") && !strings.HasPrefix(lower, "hostname") {
			inHost = true
			continue
		}

		if inHost {
			// Indented lines are directives belonging to the Host block.
			if len(line) > 0 && (line[0] == ' ' || line[0] == '\t') {
				continue
			}
			// Blank lines between/after directives are consumed.
			if trimmed == "" {
				continue
			}
			// Anything else (non-indented, non-empty) ends the block.
			inHost = false
		}

		if isSshclickMeta(trimmed) {
			continue
		}

		kept = append(kept, line)
	}

	// Collapse consecutive blank lines into at most one.
	var final []string
	prevBlank := false
	for _, line := range kept {
		blank := strings.TrimSpace(line) == ""
		if blank && prevBlank {
			continue
		}
		prevBlank = blank
		final = append(final, line)
	}

	result := strings.Join(final, "\n")
	result = strings.TrimRight(result, "\n") + "\n"
	return os.WriteFile(path, []byte(result), 0o600)
}

func isSshclickMeta(trimmed string) bool {
	for _, prefix := range []string{"#@group:", "#@desc:", "#@info:", "#@host:"} {
		if strings.HasPrefix(trimmed, prefix) {
			return true
		}
	}
	return false
}

// EnsureSSHHostsFile creates ssh_hosts (0600) if it doesn't exist.
func EnsureSSHHostsFile(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return os.WriteFile(path, nil, 0o600)
	}
	return nil
}

// ExportHostsTo copies all Host blocks from src Config into the file at dstPath
// (appending if it exists). Backs up dstPath first if it has content.
func ExportHostsTo(src *Config, dstPath string) error {
	if err := os.MkdirAll(filepath.Dir(dstPath), 0o755); err != nil {
		return err
	}
	existing, _ := os.ReadFile(dstPath)
	if len(existing) > 0 {
		bkp := hiddenBackupPath(dstPath)
		if err := os.WriteFile(bkp, existing, 0o600); err != nil {
			return fmt.Errorf("backup %s: %w", bkp, err)
		}
	}

	serialized, err := String(src)
	if err != nil {
		return fmt.Errorf("serialize hosts: %w", err)
	}

	var out string
	if len(existing) > 0 {
		out = string(existing)
		if !strings.HasSuffix(out, "\n") {
			out += "\n"
		}
		out += serialized
	} else {
		out = serialized
	}
	return os.WriteFile(dstPath, []byte(out), 0o600)
}

func hiddenBackupPath(path string) string {
	dir := filepath.Dir(path)
	base := filepath.Base(path)
	return filepath.Join(dir, "."+base+".bkp")
}
