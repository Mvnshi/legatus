// Package store keeps runs and their event journals as plain files, so a person can read, back up or
// delete them with ordinary tools. There is no database.
//
//	<dir>/runs/<id>/run.json      the run's current state (replaced atomically)
//	<dir>/runs/<id>/events.jsonl  an append-only journal
package store

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"sync"
	"time"

	"github.com/Mvnshi/legatus/internal/model"
)

// ErrNotFound is returned for a run that does not exist.
var ErrNotFound = errors.New("run not found")

var idPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,63}$`)

// Store is safe for use by many goroutines in one process. One process owns a store directory at a time.
type Store struct {
	dir string
	mu  sync.RWMutex // readers (Load, List, Events) share it; writers (Save, Append) hold it alone
}

// Open creates the directory layout if needed.
func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(filepath.Join(dir, "runs"), 0o700); err != nil {
		return nil, err
	}
	return &Store{dir: dir}, nil
}

// Dir is the directory the store lives in.
func (s *Store) Dir() string { return s.dir }

func (s *Store) runDir(id string) (string, error) {
	if !idPattern.MatchString(id) {
		return "", fmt.Errorf("invalid run id %q", id)
	}
	return filepath.Join(s.dir, "runs", id), nil
}

// Save replaces the run's state file atomically.
func (s *Store) Save(r *model.Run) error {
	dir, err := s.runDir(r.ID)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	r.UpdatedAt = time.Now().UTC()
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "run-*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(name)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return err
	}
	if err := RenameReplace(name, filepath.Join(dir, "run.json")); err != nil {
		os.Remove(name)
		return err
	}
	return nil
}

// RenameReplace renames over an existing file. On Windows that fails with "Access is denied" for a moment
// whenever another process (a second `legatus show`, an editor, a backup) has the old file open, so it is
// retried briefly before giving up.
func RenameReplace(from, to string) error {
	var err error
	for attempt := 0; attempt < 40; attempt++ {
		if err = os.Rename(from, to); err == nil {
			return nil
		}
		time.Sleep(time.Duration(attempt+1) * 5 * time.Millisecond)
	}
	return err
}

// readFile reads a whole file, retrying briefly when another process is replacing it.
func readFile(path string) ([]byte, error) {
	var data []byte
	var err error
	for attempt := 0; attempt < 20; attempt++ {
		if data, err = os.ReadFile(path); err == nil || errors.Is(err, os.ErrNotExist) {
			return data, err
		}
		time.Sleep(time.Duration(attempt+1) * 5 * time.Millisecond)
	}
	return data, err
}

// Load reads one run.
func (s *Store) Load(id string) (*model.Run, error) {
	dir, err := s.runDir(id)
	if err != nil {
		return nil, err
	}
	s.mu.RLock()
	data, err := readFile(filepath.Join(dir, "run.json"))
	s.mu.RUnlock()
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	var r model.Run
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("run %s: %w", id, err)
	}
	return &r, nil
}

// List returns every run, newest first. A run whose file cannot be read is skipped.
func (s *Store) List() ([]*model.Run, error) {
	entries, err := os.ReadDir(filepath.Join(s.dir, "runs"))
	if err != nil {
		return nil, err
	}
	var runs []*model.Run
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		r, err := s.Load(entry.Name())
		if err != nil {
			continue
		}
		runs = append(runs, r)
	}
	sort.Slice(runs, func(i, j int) bool { return runs[i].CreatedAt.After(runs[j].CreatedAt) })
	return runs, nil
}

// Append adds one event to the run's journal.
func (s *Store) Append(e model.Event) error {
	dir, err := s.runDir(e.Run)
	if err != nil {
		return err
	}
	if e.Time.IsZero() {
		e.Time = time.Now().UTC()
	}
	line, err := json.Marshal(e)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(dir, "events.jsonl"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(append(line, '\n'))
	return err
}

// Events returns the run's journal from the given line (0 is the start) and the next line to ask for.
func (s *Store) Events(id string, from int) ([]model.Event, int, error) {
	dir, err := s.runDir(id)
	if err != nil {
		return nil, from, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	f, err := os.Open(filepath.Join(dir, "events.jsonl"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, from, nil
	}
	if err != nil {
		return nil, from, err
	}
	defer f.Close()
	var events []model.Event
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	line := 0
	for scanner.Scan() {
		if line >= from {
			var e model.Event
			if json.Unmarshal(scanner.Bytes(), &e) == nil {
				events = append(events, e)
			}
		}
		line++
	}
	return events, line, scanner.Err()
}

// RunDir is where a run keeps its own files (journal, evidence).
func (s *Store) RunDir(id string) (string, error) { return s.runDir(id) }
