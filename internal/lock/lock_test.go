package lock

import (
	"errors"
	"testing"
	"time"
)

func TestAcquirePreventsConcurrentHoldersAndReclaimsExpiredLocks(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.July, 15, 14, 0, 0, 0, time.UTC)
	directory := t.TempDir()
	first, err := Acquire(directory, "wl-01", now)
	if err != nil {
		t.Fatalf("first Acquire returned an error: %v", err)
	}
	defer first.Release()

	if _, err := Acquire(directory, "wl-01", now); !errors.Is(err, ErrHeld) {
		t.Fatalf("second Acquire error = %v, want ErrHeld", err)
	}

	second, err := Acquire(directory, "wl-01", now.Add(DefaultDuration))
	if err != nil {
		t.Fatalf("Acquire after expiry returned an error: %v", err)
	}
	defer second.Release()
}
