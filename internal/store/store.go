package store

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/NelsonBrandao/work-ledger/internal/ledger"
	"github.com/NelsonBrandao/work-ledger/internal/lock"
	"github.com/NelsonBrandao/work-ledger/internal/task"
)

var ErrConflict = errors.New("task changed since it was read")

func Mutate(paths ledger.Paths, identifier, expectedETag string, apply func(*task.Document) error) (task.Task, error) {
	initial, err := findDocument(paths.Tasks, identifier)
	if err != nil {
		return task.Task{}, err
	}
	heldLock, err := lock.Acquire(paths.Locks, initial.ID, time.Now())
	if err != nil {
		return task.Task{}, err
	}
	defer heldLock.Release()

	document, err := findDocument(paths.Tasks, identifier)
	if err != nil {
		return task.Task{}, err
	}
	if expectedETag != "" && expectedETag != document.ETag {
		return task.Task{}, fmt.Errorf("%w: expected %s, current %s", ErrConflict, expectedETag, document.ETag)
	}
	if err := apply(&document); err != nil {
		return task.Task{}, err
	}
	if err := document.Save(time.Now()); err != nil {
		return task.Task{}, err
	}
	return document.Task, nil
}

func Archive(paths ledger.Paths) ([]task.Task, error) {
	tasks, err := task.LoadAll(paths.Tasks)
	if err != nil {
		return nil, err
	}
	var archived []task.Task
	for _, candidate := range tasks {
		if candidate.Status != "done" && candidate.Status != "cancelled" {
			continue
		}
		heldLock, err := lock.Acquire(paths.Locks, candidate.ID, time.Now())
		if err != nil {
			return archived, err
		}
		document, err := findDocument(paths.Tasks, candidate.ID)
		if err == nil && (document.Status == "done" || document.Status == "cancelled") {
			year := archiveYear(document.UpdatedAt)
			destinationDirectory := filepath.Join(paths.Archive, year)
			if err = os.MkdirAll(destinationDirectory, 0o700); err == nil {
				destination := filepath.Join(destinationDirectory, filepath.Base(document.Path))
				err = os.Rename(document.Path, destination)
				if err == nil {
					document.Path = destination
					archived = append(archived, document.Task)
				}
			}
		}
		releaseErr := heldLock.Release()
		if err != nil {
			return archived, fmt.Errorf("archive %s: %w", candidate.ID, err)
		}
		if releaseErr != nil {
			return archived, releaseErr
		}
	}
	return archived, nil
}

func Delete(paths ledger.Paths, identifier string) (task.Task, error) {
	initial, err := findDocument(paths.Tasks, identifier)
	if err != nil {
		return task.Task{}, err
	}
	heldLock, err := lock.Acquire(paths.Locks, initial.ID, time.Now())
	if err != nil {
		return task.Task{}, err
	}
	defer heldLock.Release()

	document, err := findDocument(paths.Tasks, identifier)
	if err != nil {
		return task.Task{}, err
	}
	if document.ID != identifier && !contains(document.LegacyIDs, identifier) {
		return task.Task{}, fmt.Errorf("delete needs the full task ID; %q matches %q", identifier, document.ID)
	}
	allTasks, err := task.LoadAll(paths.Tasks, paths.Archive)
	if err != nil {
		return task.Task{}, err
	}
	for _, candidate := range allTasks {
		if candidate.ID == document.ID {
			continue
		}
		if candidate.Parent == document.ID || contains(candidate.BlockedBy, document.ID) {
			return task.Task{}, fmt.Errorf("cannot delete %q because task %q references it", document.ID, candidate.ID)
		}
	}
	if err := os.Remove(document.Path); err != nil {
		return task.Task{}, fmt.Errorf("delete %s: %w", document.Path, err)
	}
	return document.Task, nil
}

func findDocument(directory, identifier string) (task.Document, error) {
	tasks, err := task.LoadAll(directory)
	if err != nil {
		return task.Document{}, fmt.Errorf("find task %q: %w", identifier, err)
	}
	found, err := task.Find(tasks, identifier)
	if err != nil {
		return task.Document{}, err
	}
	return task.LoadDocument(found.Path)
}

func archiveYear(updatedAt string) string {
	parsed, err := time.Parse(time.RFC3339, updatedAt)
	if err != nil {
		return time.Now().UTC().Format("2006")
	}
	return parsed.UTC().Format("2006")
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
