package lockfile

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLockPath(t *testing.T) {
	got := LockPath("/home/u/.config/sshui/ssh_hosts")
	want := "/home/u/.config/sshui/ssh_hosts.sshui.swp"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestAcquireRelease(t *testing.T) {
	dir := t.TempDir()
	data := filepath.Join(dir, "test_file")
	if err := os.WriteFile(data, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := Acquire(data); err != nil {
		t.Fatal("first acquire:", err)
	}
	defer Release(data)

	info, err := read(LockPath(data))
	if err != nil || info == nil {
		t.Fatal("expected lock info")
	}
	if info.PID != os.Getpid() {
		t.Fatalf("PID: got %d, want %d", info.PID, os.Getpid())
	}

	// Second acquire by same process should detect lock is held by us (alive PID).
	err = Acquire(data)
	if err == nil {
		t.Fatal("expected error on second acquire")
	}
	if !IsLocked(err) {
		t.Fatalf("expected LockedError, got %v", err)
	}

	if err := Release(data); err != nil {
		t.Fatal("release:", err)
	}

	if _, err := read(LockPath(data)); err == nil {
		t.Fatal("lock still exists after release")
	}
}

func TestStaleLock(t *testing.T) {
	dir := t.TempDir()
	data := filepath.Join(dir, "stale_file")
	if err := os.WriteFile(data, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Write a lock file with a dead PID.
	lp := LockPath(data)
	content := `{"pid":999999999,"start_time":"2020-01-01T00:00:00Z","path":"` + data + `"}`
	if err := os.WriteFile(lp, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	staleInfo, err := read(lp)
	if err != nil || processAlive(staleInfo.PID) {
		t.Fatal("expected stale lock (dead PID)")
	}

	// Acquire should succeed (removes stale lock).
	if err := Acquire(data); err != nil {
		t.Fatal("acquire after stale:", err)
	}

	info, err := read(LockPath(data))
	if err != nil || info == nil || info.PID != os.Getpid() {
		t.Fatal("lock not reacquired")
	}

	Release(data)
}

