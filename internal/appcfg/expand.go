package appcfg

import (
	"os"
	"path/filepath"
	"strings"
)

// ExpandPath turns ~/ into the home directory and applies filepath.Abs.
// Empty input returns ("", nil).
func ExpandPath(p string) (string, error) {
	p = strings.TrimSpace(p)
	if p == "" {
		return "", nil
	}
	if strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		p = filepath.Join(home, p[2:])
	} else if p == "~" {
		return os.UserHomeDir()
	}
	return filepath.Abs(p)
}
