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

// validateManagedLink checks that mainPath and targetAbs can be linked without
// same-file or Include-cycle issues. Returns absolute paths.
func validateManagedLink(mainPath, targetAbs string) (mainAbs, targAbs string, err error) {
	mainAbs, err = filepath.Abs(mainPath)
	if err != nil {
		return "", "", fmt.Errorf("abs main config path: %w", err)
	}
	targAbs, err = filepath.Abs(targetAbs)
	if err != nil {
		return "", "", fmt.Errorf("abs ssh_hosts path: %w", err)
	}
	if strings.EqualFold(mainAbs, targAbs) {
		return "", "", fmt.Errorf("main ssh_config and ssh_hosts are the same file")
	}
	if includeChainReaches(includeScanStart{path: targAbs, seek: mainAbs}, map[string]bool{}, 0) {
		return "", "", fmt.Errorf("include cycle: %s already includes %s (remove that Include before linking)", targAbs, mainAbs)
	}
	return mainAbs, targAbs, nil
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
	_, targAbs, err := validateManagedLink(mainPath, targetAbs)
	if err != nil {
		return err
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

// ReplaceMainSSHConfigWithManagedInclude backs up mainPath if it exists and is
// non-empty, then overwrites it with only the sshui-managed Include block for
// targetAbs. Used when the setup wizard moves all hosts into the managed file
// so the main config is a clean stub (no preserved comments or globals).
func ReplaceMainSSHConfigWithManagedInclude(mainPath, targetAbs string) error {
	_, targAbs, err := validateManagedLink(mainPath, targetAbs)
	if err != nil {
		return err
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
	out := sshuiManagedMarker + "\nInclude " + targAbs + "\n\n"
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

// ExportHostsTo serializes all Host blocks from src into dstPath (appending if
// dstPath already has content). Backs up dstPath first if it has content.
//
// includeResolveBaseDir should be the directory of the file src was parsed from
// (the main ssh_config), so relative Include patterns resolve correctly.
// If empty, filepath.Dir(dstPath) is used.
//
// Include-only stanzas that resolve to dstPath (the managed file) are omitted
// so the copy does not pull in the bridge stanza.
func ExportHostsTo(src *Config, dstPath, includeResolveBaseDir string) error {
	if err := os.MkdirAll(filepath.Dir(dstPath), 0o755); err != nil {
		return err
	}
	managedAbs, err := filepath.Abs(dstPath)
	if err != nil {
		return fmt.Errorf("abs managed path: %w", err)
	}
	if includeResolveBaseDir == "" {
		includeResolveBaseDir = filepath.Dir(managedAbs)
	}
	filtered := StripBridgeIncludes(src, managedAbs, includeResolveBaseDir)

	existing, _ := os.ReadFile(dstPath)
	if len(existing) > 0 {
		bkp := hiddenBackupPath(dstPath)
		if err := os.WriteFile(bkp, existing, 0o600); err != nil {
			return fmt.Errorf("backup %s: %w", bkp, err)
		}
	}

	serialized, err := String(filtered)
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
