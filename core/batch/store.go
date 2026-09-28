package batch

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

const (
	batchDir = "batches"
	fileMode = 0o600
	dirMode  = 0o700
)

// ErrNotFound means there is no batch with that ID.
var ErrNotFound = errors.New("batch: not found")

// Store keeps batches as one JSON file each in the data folder.
type Store struct {
	dir     string
	mu      sync.Mutex
	batches map[string]*Batch
}

// OpenStore loads all batches from dataDir/batches.
func OpenStore(dataDir string) (*Store, error) {
	dir := filepath.Join(dataDir, batchDir)
	if err := os.MkdirAll(dir, dirMode); err != nil {
		return nil, err
	}
	s := &Store{dir: dir, batches: map[string]*Batch{}}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		b := &Batch{}
		if err := json.Unmarshal(raw, b); err != nil {
			return nil, fmt.Errorf("batch %s: %w", e.Name(), err)
		}
		s.batches[b.ID] = b
	}
	return s, nil
}

// Lock serialises changes; callers hold it while they modify a batch and save it.
func (s *Store) Lock()   { s.mu.Lock() }
func (s *Store) Unlock() { s.mu.Unlock() }

// Get returns a batch; the caller must hold the lock when modifying it.
func (s *Store) Get(id string) (*Batch, error) {
	b, ok := s.batches[id]
	if !ok {
		return nil, ErrNotFound
	}
	return b, nil
}

// All returns batches, newest change first, without discarded ones.
func (s *Store) All() []*Batch {
	out := make([]*Batch, 0, len(s.batches))
	for _, b := range s.batches {
		if b.Status == StatusDiscarded {
			continue
		}
		out = append(out, b)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Updated.After(out[j].Updated) })
	return out
}

// Put adds or replaces a batch and saves it.
func (s *Store) Put(b *Batch) error {
	s.batches[b.ID] = b
	return s.save(b)
}

// Save writes a batch atomically.
func (s *Store) save(b *Batch) error {
	raw, err := json.MarshalIndent(b, "", " ")
	if err != nil {
		return err
	}
	path := filepath.Join(s.dir, safeName(b.ID)+".json")
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, fileMode); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Save persists a batch the caller modified under the lock.
func (s *Store) Save(b *Batch) error { return s.save(b) }

func safeName(id string) string {
	return strings.Map(func(r rune) rune {
		if r == '/' || r == '\\' || r == '.' {
			return '_'
		}
		return r
	}, id)
}
