package lock

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const DefaultDuration = time.Minute

var ErrHeld = errors.New("task is being updated by another process")

type Metadata struct {
	PID       int       `json:"pid"`
	Hostname  string    `json:"hostname"`
	ExpiresAt time.Time `json:"expires_at"`
}

type Lock struct {
	path string
}

func Acquire(directory, taskID string, now time.Time) (Lock, error) {
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return Lock{}, fmt.Errorf("create lock directory: %w", err)
	}
	path := filepath.Join(directory, lockName(taskID))
	metadata := Metadata{PID: os.Getpid(), ExpiresAt: now.Add(DefaultDuration)}
	metadata.Hostname, _ = os.Hostname()
	contents, err := json.Marshal(metadata)
	if err != nil {
		return Lock{}, fmt.Errorf("encode lock: %w", err)
	}

	for attempt := 0; attempt < 2; attempt++ {
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err == nil {
			if _, err := file.Write(contents); err != nil {
				file.Close()
				os.Remove(path)
				return Lock{}, fmt.Errorf("write lock: %w", err)
			}
			if err := file.Close(); err != nil {
				os.Remove(path)
				return Lock{}, fmt.Errorf("close lock: %w", err)
			}
			return Lock{path: path}, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return Lock{}, fmt.Errorf("acquire lock: %w", err)
		}

		lockedBy, readErr := readMetadata(path)
		if readErr == nil && !lockedBy.ExpiresAt.After(now) {
			if removeErr := os.Remove(path); removeErr == nil || errors.Is(removeErr, os.ErrNotExist) {
				continue
			}
		}
		return Lock{}, fmt.Errorf("%w: %s until %s", ErrHeld, lockedBy.Hostname, lockedBy.ExpiresAt.Format(time.RFC3339))
	}
	return Lock{}, ErrHeld
}

func (lock Lock) Release() error {
	if lock.path == "" {
		return nil
	}
	if err := os.Remove(lock.path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("release lock: %w", err)
	}
	return nil
}

func readMetadata(path string) (Metadata, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return Metadata{}, err
	}
	var metadata Metadata
	if err := json.Unmarshal(contents, &metadata); err != nil {
		return Metadata{}, err
	}
	return metadata, nil
}

func lockName(taskID string) string {
	sum := sha256.Sum256([]byte(taskID))
	return hex.EncodeToString(sum[:]) + ".lock"
}
