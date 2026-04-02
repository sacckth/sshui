// Package lockfile provides vim-style .sshui.swp lock files to guard against
// concurrent sshui instances editing the same file.
package lockfile

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// Info is the content written into a lock file.
type Info struct {
	PID       int       `json:"pid"`
	StartTime time.Time `json:"start_time"`
	Path      string    `json:"path"`
}

// LockPath returns the .sshui.swp path adjacent to the given data file.
func LockPath(dataFile string) string {
	dir := filepath.Dir(dataFile)
	base := filepath.Base(dataFile)
	return filepath.Join(dir, base+".sshui.swp")
}

// Acquire creates a lock file for dataFile. Returns an error if the lock
// already exists and the owning process is still alive (ErrLocked).
// If the lock is stale (owning PID dead), it is removed and re-acquired.
func Acquire(dataFile string) error {
	lp := LockPath(dataFile)
	existing, err := read(lp)
	if err == nil {
		if processAlive(existing.PID) {
			return &LockedError{Lock: *existing}
		}
		_ = os.Remove(lp)
	}
	info := Info{
		PID:       os.Getpid(),
		StartTime: time.Now().UTC(),
		Path:      dataFile,
	}
	b, err := json.Marshal(info)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(lp), 0o755); err != nil {
		return err
	}
	return os.WriteFile(lp, b, 0o600)
}

// Release removes the lock file for dataFile, but only if it belongs to this process.
func Release(dataFile string) error {
	lp := LockPath(dataFile)
	existing, err := read(lp)
	if err != nil {
		return nil
	}
	if existing.PID == os.Getpid() {
		return os.Remove(lp)
	}
	return nil
}

// LockedError is returned when the lock is held by another live process.
type LockedError struct {
	Lock Info
}

func (e *LockedError) Error() string {
	return fmt.Sprintf("file locked by PID %d since %s", e.Lock.PID, e.Lock.StartTime.Format(time.RFC3339))
}

// IsLocked returns true if err is a LockedError.
func IsLocked(err error) bool {
	var le *LockedError
	return errors.As(err, &le)
}

func read(path string) (*Info, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var info Info
	if err := json.Unmarshal(b, &info); err != nil {
		return nil, err
	}
	return &info, nil
}

func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = p.Signal(syscall.Signal(0))
	return err == nil
}
