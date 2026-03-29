package deploy

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

var lockPath = "/var/lib/freedb/deploy.lock"

const lockTimeout = 30 * time.Second

type Lock struct {
	file *os.File
}

// AcquireLock attempts to acquire the deploy lock.
// Waits up to 30 seconds for an existing lock to release.
// Returns the lock (must call Release when done) or an error.
func AcquireLock() (*Lock, error) {
	if err := os.MkdirAll(filepath.Dir(lockPath), 0755); err != nil {
		return nil, fmt.Errorf("creating lock directory: %w", err)
	}

	deadline := time.Now().Add(lockTimeout)
	for {
		// Try to clean up stale locks
		cleanStaleLock()

		f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
		if err == nil {
			// Write PID and timestamp for debugging
			content := fmt.Sprintf("%d\n%s\n", os.Getpid(), time.Now().Format(time.RFC3339))
			f.WriteString(content)
			return &Lock{file: f}, nil
		}

		if time.Now().After(deadline) {
			// Read lock info for the error message
			info := readLockInfo()
			return nil, fmt.Errorf("deploy lock timeout after %s — another deploy is in progress%s", lockTimeout, info)
		}

		time.Sleep(1 * time.Second)
	}
}

// Release releases the deploy lock.
func (l *Lock) Release() {
	if l.file != nil {
		l.file.Close()
	}
	os.Remove(lockPath)
}

// cleanStaleLock removes the lock file if the owning process is no longer running.
func cleanStaleLock() {
	data, err := os.ReadFile(lockPath)
	if err != nil {
		return // no lock file
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) < 1 {
		return
	}

	pid, err := strconv.Atoi(lines[0])
	if err != nil {
		// Corrupted lock file, remove it
		os.Remove(lockPath)
		return
	}

	// Check if the process is still running
	process, err := os.FindProcess(pid)
	if err != nil {
		os.Remove(lockPath)
		return
	}

	// On Unix, FindProcess always succeeds. Send signal 0 to check if process exists.
	if err := process.Signal(syscall.Signal(0)); err != nil {
		// Process is gone, remove stale lock
		os.Remove(lockPath)
	}
}

func readLockInfo() string {
	data, err := os.ReadFile(lockPath)
	if err != nil {
		return ""
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) >= 2 {
		return fmt.Sprintf(" (pid %s, started %s)", lines[0], lines[1])
	}
	return ""
}
