package deploy

import (
	"os"
	"path/filepath"
	"testing"
)

func withTempLock(t *testing.T) func() {
	origPath := lockPath
	dir := t.TempDir()
	lockPath = filepath.Join(dir, "deploy.lock")
	return func() { lockPath = origPath }
}

func TestAcquireAndReleaseLock(t *testing.T) {
	defer withTempLock(t)()

	lock, err := AcquireLock()
	if err != nil {
		t.Fatalf("AcquireLock failed: %v", err)
	}

	// Lock file should exist
	if _, err := os.Stat(lockPath); os.IsNotExist(err) {
		t.Fatal("lock file not created")
	}

	lock.Release()

	// Lock file should be gone
	if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
		t.Fatal("lock file not removed after release")
	}
}

func TestStaleLockCleanup(t *testing.T) {
	defer withTempLock(t)()

	// Write a lock with a non-existent PID
	os.WriteFile(lockPath, []byte("999999999\n2026-01-01T00:00:00Z\n"), 0644)

	// Should clean up stale lock and acquire
	lock, err := AcquireLock()
	if err != nil {
		t.Fatalf("AcquireLock should clean stale lock: %v", err)
	}
	lock.Release()
}
