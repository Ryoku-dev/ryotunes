package state

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"syscall"

	"github.com/ryoku-dev/ryotunes/internal/protocol"
)

// ErrClientLockHeld means another Ryotunes window process owns the client lock.
var ErrClientLockHeld = errors.New("Ryotunes client is already running")

// ClientLock holds the process-wide client singleton lock until Close.
type ClientLock struct {
	file *os.File
	once sync.Once
	err  error
}

// ClientLockPath returns the per-user client singleton lock path.
func ClientLockPath() string {
	return filepath.Join(protocol.RuntimeDir(), "client.lock")
}

// AcquireClientLock takes the client singleton lock without waiting.
func AcquireClientLock() (*ClientLock, error) {
	directory := protocol.RuntimeDir()
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return nil, fmt.Errorf("create client runtime directory: %w", err)
	}
	if err := os.Chmod(directory, 0o700); err != nil {
		return nil, fmt.Errorf("secure client runtime directory: %w", err)
	}
	file, err := os.OpenFile(ClientLockPath(), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open client lock: %w", err)
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = file.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
			return nil, ErrClientLockHeld
		}
		return nil, fmt.Errorf("lock client: %w", err)
	}
	return &ClientLock{file: file}, nil
}

// Close releases the singleton lock. It is safe to call more than once.
func (lock *ClientLock) Close() error {
	if lock == nil {
		return nil
	}
	lock.once.Do(func() {
		if err := syscall.Flock(int(lock.file.Fd()), syscall.LOCK_UN); err != nil {
			lock.err = err
		}
		if err := lock.file.Close(); lock.err == nil {
			lock.err = err
		}
	})
	return lock.err
}
