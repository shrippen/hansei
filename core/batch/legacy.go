package batch

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"git.arianw.de/shrippen/hansei/core/diff"
	"git.arianw.de/shrippen/hansei/core/vault"
)

// Suffixes of the old review-queue/ layout: review-queue/{batch}/{path}.proposed (+ .review.json).
const (
	legacyProposed = ".proposed"
	legacyReview   = ".review.json"
)

// legacyReviewFile is the status file of the old review tool.
type legacyReviewFile struct {
	RelPath  string `json:"rel_path"`
	Status   string `json:"status"`
	Comments []struct {
		Author    string    `json:"author"`
		Text      string    `json:"text"`
		Timestamp time.Time `json:"timestamp"`
	} `json:"comments"`
}

// ReadFunc returns the current content of a vault note and whether it exists.
type ReadFunc func(rel string) (string, bool, error)

// LegacyBatches reads review-queue/ and returns one batch per sub folder.
// Folders already imported (by ImportDir) are skipped; files the reader rejects are skipped too.
func LegacyBatches(queue string, read ReadFunc, known map[string]bool) ([]*Batch, error) {
	entries, err := os.ReadDir(queue)
	if err != nil {
		return nil, err
	}
	var out []*Batch
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(queue, e.Name())
		if known[dir] {
			continue
		}
		b, err := legacyBatch(dir, e.Name(), read)
		if err != nil {
			return nil, err
		}
		if len(b.Files) > 0 {
			out = append(out, b)
		}
	}
	return out, nil
}

func legacyBatch(dir, name string, read ReadFunc) (*Batch, error) {
	now := time.Now()
	b := &Batch{
		ID: NewID(), Title: legacyTitle(name), Topic: legacyTopic(name), Source: SourceImport,
		Status: StatusReview, Created: now, Updated: now, ImportDir: dir,
		Instruction: name,
	}

	var proposed []string
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(p, legacyProposed) {
			proposed = append(proposed, p)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(proposed)

	for _, p := range proposed {
		rel, _ := filepath.Rel(dir, strings.TrimSuffix(p, legacyProposed))
		rel = filepath.ToSlash(rel)
		base, exists, err := read(rel)
		if err != nil {
			continue
		}
		content, err := os.ReadFile(p)
		if err != nil {
			return nil, err
		}
		if !diff.Compare(base, string(content)).Changed() {
			continue // already in the vault
		}
		f := &File{Path: rel, Base: base, Exists: exists, Status: FileOpen, Decisions: map[string]Decision{}}
		f.BaseHash = vault.Hash(base)
		f.AddVersion(Version{Content: string(content), Author: AuthorImport, Note: "review-queue"})
		b.Files = append(b.Files, f)
		legacyComments(b, strings.TrimSuffix(p, legacyProposed)+legacyReview, rel)
	}
	return b, nil
}

// legacyComments moves old comments into the thread.
func legacyComments(b *Batch, path, rel string) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var r legacyReviewFile
	if json.Unmarshal(raw, &r) != nil {
		return
	}
	for _, c := range r.Comments {
		role := AuthorUser
		if c.Author != "user" {
			role = AuthorAI
		}
		b.AddMessage(Message{Role: role, Text: c.Text, Scope: ScopeFile, Path: rel, Created: c.Timestamp})
	}
}

// legacyTitle turns "batch-a-secrets" into "Secrets" (the letter only numbered the batches).
func legacyTitle(name string) string {
	parts := strings.Split(strings.TrimPrefix(name, "batch-"), "-")
	if len(parts) > 1 && len(parts[0]) == 1 {
		parts = parts[1:]
	}
	t := strings.Join(parts, " ")
	if t == "" {
		return name
	}
	return strings.ToUpper(t[:1]) + t[1:]
}

func legacyTopic(name string) string {
	parts := strings.Split(legacyTitle(name), " ")
	return parts[len(parts)-1]
}
