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

// AppendInclude backs up mainPath when it has content, ensures a top-of-file
// sshui-managed block in the form:
//
//	#sshui-managed
//	Include <targetAbs>
//
// OpenSSH accepts Include at file scope (preferred for IDE tooling); older
// sshui releases used a Host * wrapper at the end of the file — those blocks
// are stripped and replaced with this header when the file is updated.
//
// Returns an error if mainPath and targetAbs are the same file, or if
// targetAbs (or any file it Includes) already references mainPath, which would
// create a recursive Include chain once main lists targetAbs.
func AppendInclude(mainPath, targetAbs string) error {
	mainAbs, err := filepath.Abs(mainPath)
	if err != nil {
		return fmt.Errorf("abs main config path: %w", err)
	}
	targAbs, err := filepath.Abs(targetAbs)
	if err != nil {
		return fmt.Errorf("abs ssh_hosts path: %w", err)
	}
	if strings.EqualFold(mainAbs, targAbs) {
		return fmt.Errorf("main ssh_config and ssh_hosts are the same file")
	}
	if includeChainReaches(includeScanStart{path: targAbs, seek: mainAbs}, map[string]bool{}, 0) {
		return fmt.Errorf("include cycle: %s already includes %s (remove that Include before linking)", targAbs, mainAbs)
	}

	data, err := os.ReadFile(mainPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read %s: %w", mainPath, err)
	}

	body := string(data)
	if leadingManagedIncludeOK(body, targAbs) {
		return nil
	}

	if len(data) > 0 {
		bkp := hiddenBackupPath(mainPath)
		if err := os.WriteFile(bkp, data, 0o600); err != nil {
			return fmt.Errorf("backup %s: %w", bkp, err)
		}
	}

	stripped := stripManagedIncludeBlocks(body)
	stripped = strings.TrimLeft(stripped, " \t\r\n")
	header := sshuiManagedMarker + "\nInclude " + targAbs + "\n"
	var out string
	if stripped == "" {
		out = header + "\n"
	} else {
		out = header + "\n" + stripped
		if !strings.HasSuffix(out, "\n") {
			out += "\n"
		}
	}

	if err := os.MkdirAll(filepath.Dir(mainPath), 0o700); err != nil {
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(mainPath), err)
	}
	return os.WriteFile(mainPath, []byte(out), 0o600)
}

type includeScanStart struct {
	path string
	seek string
}

// includeChainReaches performs a depth-first audit of Include directives
// starting at path, returning true if any resolved include target is seek
// (case-insensitive, absolute paths) or eventually includes it. visited
// prevents infinite traversal on cycles within the include graph.
func includeChainReaches(s includeScanStart, visited map[string]bool, depth int) bool {
	if depth > maxIncludeDepth {
		return false
	}
	ap, err := filepath.Abs(s.path)
	if err != nil {
		return false
	}
	key := strings.ToLower(ap)
	seekLower := strings.ToLower(s.seek)
	if key == seekLower {
		return true
	}
	if visited[key] {
		return false
	}
	visited[key] = true

	data, err := os.ReadFile(ap)
	if err != nil {
		return false
	}
	cfg, err := Parse(strings.NewReader(string(data)))
	if err != nil {
		return false
	}
	baseDir := filepath.Dir(ap)
	for _, pat := range collectIncludePatterns(cfg) {
		for _, incAbs := range resolveIncludePattern(baseDir, pat) {
			if includeChainReaches(includeScanStart{path: incAbs, seek: s.seek}, visited, depth+1) {
				return true
			}
		}
	}
	return false
}

func leadingManagedIncludeOK(body string, targetAbs string) bool {
	s := strings.TrimPrefix(body, "\ufeff")
	s = strings.TrimLeft(s, " \t\r\n")
	if !strings.HasPrefix(s, sshuiManagedMarker) {
		return false
	}
	rest := strings.TrimLeft(s[len(sshuiManagedMarker):], " \t\r\n")
	line, _, _ := strings.Cut(rest, "\n")
	line = strings.TrimSpace(line)
	fields := strings.Fields(line)
	if len(fields) < 2 || !strings.EqualFold(fields[0], "Include") {
		return false
	}
	incPath := strings.Join(fields[1:], " ")
	return includePathsEquivalent(incPath, targetAbs)
}

func includePathsEquivalent(pathInFile, targetAbs string) bool {
	pathInFile = strings.TrimSpace(pathInFile)
	targetAbs, err := filepath.Abs(targetAbs)
	if err != nil {
		return false
	}
	base := filepath.Dir(targetAbs)
	for _, r := range resolveIncludePattern(base, pathInFile) {
		if ar, err := filepath.Abs(r); err == nil && strings.EqualFold(ar, targetAbs) {
			return true
		}
	}
	return false
}

func stripManagedIncludeBlocks(content string) string {
	lines := strings.Split(content, "\n")
	var out []string
	i := 0
	for i < len(lines) {
		if strings.TrimSpace(lines[i]) != sshuiManagedMarker {
			out = append(out, lines[i])
			i++
			continue
		}
		i++
		for i < len(lines) && strings.TrimSpace(lines[i]) == "" {
			i++
		}
		if i >= len(lines) {
			break
		}
		next := strings.TrimSpace(lines[i])
		lower := strings.ToLower(next)
		if strings.HasPrefix(lower, "include ") {
			i++
			continue
		}
		if strings.EqualFold(next, "Host *") {
			i++
			for i < len(lines) && strings.TrimSpace(lines[i]) == "" {
				i++
			}
			if i < len(lines) {
				dir := strings.TrimSpace(lines[i])
				if strings.HasPrefix(strings.ToLower(dir), "include ") {
					i++
				}
			}
			continue
		}
		out = append(out, sshuiManagedMarker)
	}
	return strings.Join(out, "\n")
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
