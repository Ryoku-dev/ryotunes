package state

import (
	"errors"
	"testing"
)

func TestAcquireClientLockExcludesSecondOwner(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())

	first, err := AcquireClientLock()
	if err != nil {
		t.Fatalf("acquire first client lock: %v", err)
	}
	defer first.Close()

	second, err := AcquireClientLock()
	if second != nil {
		_ = second.Close()
		t.Fatal("second acquisition returned a lock")
	}
	if !errors.Is(err, ErrClientLockHeld) {
		t.Fatalf("second acquisition error = %v, want ErrClientLockHeld", err)
	}

	if err := first.Close(); err != nil {
		t.Fatalf("release first client lock: %v", err)
	}
	third, err := AcquireClientLock()
	if err != nil {
		t.Fatalf("acquire after release: %v", err)
	}
	if err := third.Close(); err != nil {
		t.Fatalf("release third client lock: %v", err)
	}
}
