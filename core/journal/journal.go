// Package journal records every write to the vault with the content before and after,
// so a file or a whole batch can be undone. The log is append-only (journal.jsonl).
package journal

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

const (
	logName  = "journal.jsonl"
	fileMode = 0o600
	idBytes  = 6
	maxLine  = 64 << 20
)

// ErrUnknown means the entry does not exist.
var ErrUnknown = errors.New("journal: unknown entry")

// Entry is one write.
type Entry struct {
	ID        string    `json:"id"`
	Batch     string    `json:"batch"`
	Title     string    `json:"title"`
	Path      string    `json:"path"`
	Existed   bool      `json:"existed"`
	Before    string    `json:"before"`
	After     string    `json:"after"`
	AfterHash string    `json:"afterHash"`
	Time      time.Time `json:"time"`
	Undone    bool      `json:"undone"`
	UndoOf    string    `json:"undoOf,omitempty"` // set on the record that marks an undo
}

// Journal is the append-only log.
type Journal struct {
	path    string
	mu      sync.Mutex
	entries []Entry
}

// Open loads the log from dataDir.
func Open(dataDir string) (*Journal, error) {
	j := &Journal{path: filepath.Join(dataDir, logName)}
	f, err := os.Open(j.path)
	if errors.Is(err, os.ErrNotExist) {
		return j, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()

	undone := map[string]bool{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 1<<20), maxLine)
	for sc.Scan() {
		var e Entry
		if json.Unmarshal(sc.Bytes(), &e) != nil {
			continue
		}
		if e.UndoOf != "" {
			undone[e.UndoOf] = true
			continue
		}
		j.entries = append(j.entries, e)
	}
	for i := range j.entries {
		j.entries[i].Undone = undone[j.entries[i].ID]
	}
	return j, sc.Err()
}

// Add records a write and returns the entry ID.
func (j *Journal) Add(e Entry) (string, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	e.ID = newID()
	if e.Time.IsZero() {
		e.Time = time.Now()
	}
	if err := j.append(e); err != nil {
		return "", err
	}
	j.entries = append(j.entries, e)
	return e.ID, nil
}

// MarkUndone records that an entry was reverted.
func (j *Journal) MarkUndone(id string) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	for i := range j.entries {
		if j.entries[i].ID != id {
			continue
		}
		if err := j.append(Entry{UndoOf: id, Time: time.Now()}); err != nil {
			return err
		}
		j.entries[i].Undone = true
		return nil
	}
	return ErrUnknown
}

// Get returns one entry.
func (j *Journal) Get(id string) (Entry, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	for _, e := range j.entries {
		if e.ID == id {
			return e, nil
		}
	}
	return Entry{}, ErrUnknown
}

// List returns entries newest first; limit 0 means all.
func (j *Journal) List(limit int) []Entry {
	j.mu.Lock()
	defer j.mu.Unlock()
	out := append([]Entry(nil), j.entries...)
	sort.Slice(out, func(a, b int) bool { return out[a].Time.After(out[b].Time) })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

func (j *Journal) append(e Entry) error {
	if err := os.MkdirAll(filepath.Dir(j.path), 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(j.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, fileMode)
	if err != nil {
		return err
	}
	defer f.Close()
	raw, err := json.Marshal(e)
	if err != nil {
		return err
	}
	_, err = f.Write(append(raw, '\n'))
	return err
}

func newID() string {
	buf := make([]byte, idBytes)
	_, _ = rand.Read(buf)
	return hex.EncodeToString(buf)
}
