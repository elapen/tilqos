package config

// Where the learner (core.Learner) lives on disk: learned.json next to config.json. Kept out
// of config.json on purpose, as the Swift app keeps it out of Prefs: nothing in it is a
// setting the user types in, and it saves itself on its own schedule.

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"time"

	"tilqos/internal/core"
)

// LearnerSaveDelay coalesces saves: a burst of typing is one write, not one per word.
const LearnerSaveDelay = 2 * time.Second

// LearnerStore owns the learner and its file.
type LearnerStore struct {
	path    string
	learner *core.Learner

	mu      sync.Mutex
	timer   *time.Timer
	closed  bool
	lastErr error

	// saveMu serializes saves: a timer save and the final one must not overlap, or the older
	// snapshot could win the rename.
	saveMu sync.Mutex
}

// LearnerPath is learned.json in the directory of the config file.
func LearnerPath(configPath string) string {
	return filepath.Join(filepath.Dir(configPath), "learned.json")
}

// OpenLearner loads the learner from path. A missing file starts fresh; a malformed one
// starts fresh too, after being moved aside (and the error says so) — learning is a
// convenience, never a reason not to start. A file that exists but cannot be read right
// now (another program holds it) is left alone: this run learns in memory only, so the
// file is not overwritten with an empty store. A file written under older learning rules
// loses what those rules learned (core.Learner.Upgraded) and is rewritten at once.
func OpenLearner(path string) (*LearnerStore, error) {
	store := &LearnerStore{path: path}
	var loadErr error
	data, err := os.ReadFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		store.learner = core.NewLearner(nil)
	case err != nil:
		store.learner = core.NewLearner(nil)
		store.path = ""
		loadErr = fmt.Errorf("learned words unreadable, learning in memory only this time: %w", err)
	default:
		learner, err := core.DecodeLearner(data, nil)
		store.learner = learner
		if err != nil {
			backup := path + ".bad"
			_ = os.Rename(path, backup)
			loadErr = fmt.Errorf("learned words unreadable (moved to %s): %w", filepath.Base(backup), err)
		} else if _, upgraded := learner.Upgraded(); upgraded {
			// Saved now, so what the old rules learned stays dropped even if nothing is
			// learned this run.
			if err := store.Flush(); err != nil {
				loadErr = fmt.Errorf("learned words upgraded to the new rules but not saved: %w", err)
			}
		}
	}
	store.learner.OnChange(store.schedule)
	return store, loadErr
}

// NewMemoryLearnerStore is a store that never touches the disk (tests).
func NewMemoryLearnerStore() *LearnerStore {
	return &LearnerStore{learner: core.NewLearner(nil)}
}

func (s *LearnerStore) Learner() *core.Learner { return s.learner }

func (s *LearnerStore) schedule() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.path == "" || s.timer != nil {
		return
	}
	s.timer = time.AfterFunc(LearnerSaveDelay, func() {
		s.mu.Lock()
		s.timer = nil
		s.mu.Unlock()
		if err := s.Flush(); err != nil {
			s.mu.Lock()
			s.lastErr = err
			s.mu.Unlock()
		}
	})
}

// Flush writes now.
func (s *LearnerStore) Flush() error {
	if s.path == "" {
		return nil
	}
	s.saveMu.Lock()
	defer s.saveMu.Unlock()
	data, err := s.learner.Encode()
	if err != nil {
		return err
	}
	return WriteFileAtomic(s.path, data)
}

// ResetLearnedVocabulary deletes everything learned (entries and salt) and writes the empty
// store at once, instead of two seconds later.
func (s *LearnerStore) ResetLearnedVocabulary() error {
	s.learner.ResetLearnedVocabulary()
	return s.Flush()
}

// Close stops the schedule and writes what is pending.
func (s *LearnerStore) Close() error {
	s.mu.Lock()
	s.closed = true
	if s.timer != nil {
		s.timer.Stop()
		s.timer = nil
	}
	s.mu.Unlock()
	return s.Flush()
}

// LastSaveError is the last failed background save, if any.
func (s *LearnerStore) LastSaveError() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastErr
}
