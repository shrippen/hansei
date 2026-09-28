// Package batch holds proposals grouped by topic: files, versions, hunk decisions and the
// feedback thread with the AI.
//
//	Batch ── File ── Version v1, v2, v3 …   (content proposed by AI, rules, import or you)
//	   │        └─ Decisions (hunk ID → accepted/rejected, survive revisions)
//	   └─ Thread (feedback and AI replies, scoped to batch, file or hunk)
package batch

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"git.arianw.de/shrippen/hansei/core/diff"
)

// Status is the life cycle state of a batch.
type Status string

const (
	StatusWorking   Status = "working"   // AI builds the first proposal
	StatusReview    Status = "review"    // waiting for your review
	StatusDone      Status = "done"      // every file decided
	StatusFailed    Status = "failed"    // AI run failed, see Error
	StatusDiscarded Status = "discarded" // thrown away
)

// Column is where a batch sits on the board (Werkbank).
type Column string

const (
	ColumnWorking  Column = "working"
	ColumnReview   Column = "review"
	ColumnFeedback Column = "feedback" // AI question open or revision running
	ColumnDone     Column = "done"
	ColumnFailed   Column = "failed"
)

// Source tells where a batch came from.
type Source string

const (
	SourceAI     Source = "ai"
	SourceRules  Source = "rules"  // built from rule findings without AI
	SourceImport Source = "import" // old review-queue/ folder
	SourceRule   Source = "rule"   // rulebook change suggested from repeated feedback
)

// Decision on one hunk.
type Decision string

const (
	Accepted Decision = "accepted"
	Rejected Decision = "rejected"
	Pending  Decision = "pending"
)

// FileStatus is the state of one file in a batch.
type FileStatus string

const (
	FileOpen    FileStatus = "open"
	FileApplied FileStatus = "applied" // written to the vault
	FileSkipped FileStatus = "skipped" // every hunk rejected, nothing written
	FileUndone  FileStatus = "undone"  // write was undone
)

// Author of a version or message.
type Author string

const (
	AuthorAI     Author = "ai"
	AuthorUser   Author = "user"
	AuthorRules  Author = "rules"
	AuthorImport Author = "import"
	AuthorSystem Author = "system"
)

// Scope of a feedback message.
type Scope string

const (
	ScopeBatch Scope = "batch"
	ScopeFile  Scope = "file"
	ScopeHunk  Scope = "hunk"
)

// Change is the AI's explanation of one edit, matched to hunks by text.
type Change struct {
	Before string `json:"before,omitempty"`
	After  string `json:"after,omitempty"`
	Reason string `json:"reason"`
	Rule   string `json:"rule,omitempty"`
}

// Version is one proposed content of a file.
type Version struct {
	N       int       `json:"n"`
	Content string    `json:"content"`
	Author  Author    `json:"author"`
	Note    string    `json:"note,omitempty"`
	Changes []Change  `json:"changes,omitempty"`
	Created time.Time `json:"created"`
}

// File is one note in a batch.
type File struct {
	Path      string              `json:"path"`
	Base      string              `json:"base"`
	BaseHash  string              `json:"baseHash"`
	Exists    bool                `json:"exists"`
	Summary   string              `json:"summary,omitempty"`
	Versions  []Version           `json:"versions"`
	Current   int                 `json:"current"` // index into Versions
	Decisions map[string]Decision `json:"decisions"`
	Reasons   map[string]string   `json:"reasons,omitempty"` // hunk ID → your reason for rejecting
	Status    FileStatus          `json:"status"`
	Applied   time.Time           `json:"applied,omitempty"`
	Journal   string              `json:"journal,omitempty"` // journal entry of the write

	diffKey string       // cache: base hash + version the diff below belongs to
	diff    *diff.Result // cache, read-only once built
}

// Message is one entry of the feedback thread.
type Message struct {
	ID       string         `json:"id"`
	Role     Author         `json:"role"`
	Text     string         `json:"text"`
	Scope    Scope          `json:"scope,omitempty"`
	Path     string         `json:"path,omitempty"`
	Hunk     string         `json:"hunk,omitempty"`
	Quick    string         `json:"quick,omitempty"`    // quick reason chip, e.g. "wrong-fact"
	Remember bool           `json:"remember,omitempty"` // "Als Regel merken"
	Question bool           `json:"question,omitempty"` // AI asks you
	Answered bool           `json:"answered,omitempty"`
	Versions map[string]int `json:"versions,omitempty"` // file → version this round created
	Created  time.Time      `json:"created"`
}

// Found is a note a find-only task reports, with the reason.
type Found struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

// Usage counts tokens of all AI runs of a batch.
type Usage struct {
	In   int     `json:"in"`
	Out  int     `json:"out"`
	Cost float64 `json:"cost,omitempty"`
}

// Suggestion is a rulebook change proposed from repeated feedback.
type Suggestion struct {
	ID     string `json:"id"`
	Text   string `json:"text"`
	Count  int    `json:"count"`
	Status string `json:"status"` // open, accepted, dismissed
	Batch  string `json:"batch,omitempty"`
	Target string `json:"target,omitempty"` // rulebook note the rule would go to
}

// Suggestion states.
const (
	SuggestionOpen      = "open"
	SuggestionAccepted  = "accepted"
	SuggestionDismissed = "dismissed"
)

// Batch is a group of proposals on one topic.
type Batch struct {
	ID          string       `json:"id"`
	Title       string       `json:"title"`
	Topic       string       `json:"topic"`
	Instruction string       `json:"instruction"`
	Scope       []string     `json:"scope,omitempty"`
	Source      Source       `json:"source"`
	Provider    string       `json:"provider,omitempty"`
	Status      Status       `json:"status"`
	Revising    bool         `json:"revising,omitempty"`
	Progress    string       `json:"progress,omitempty"`
	Error       string       `json:"error,omitempty"`
	Summary     string       `json:"summary,omitempty"`
	Created     time.Time    `json:"created"`
	Updated     time.Time    `json:"updated"`
	Files       []*File      `json:"files"`
	Thread      []Message    `json:"thread"`
	Usage       Usage        `json:"usage"`
	Suggestions []Suggestion `json:"suggestions,omitempty"`
	ImportDir   string       `json:"importDir,omitempty"`
	Found       []Found      `json:"found,omitempty"`
	Rule        string       `json:"rule,omitempty"` // rule ID for batches built from findings
}

// NewID returns a sortable unique ID, e.g. "20260927-1412-3fa9".
func NewID() string {
	buf := make([]byte, 2)
	_, _ = rand.Read(buf)
	return time.Now().Format("20060102-150405") + "-" + hex.EncodeToString(buf)
}

// File finds a file by path.
func (b *Batch) File(p string) (*File, bool) {
	for _, f := range b.Files {
		if f.Path == p {
			return f, true
		}
	}
	return nil, false
}

// Column places the batch on the board.
func (b *Batch) Column() Column {
	switch {
	case b.Status == StatusFailed:
		return ColumnFailed
	case b.Status == StatusDone:
		return ColumnDone
	case b.Status == StatusWorking && !b.Revising:
		return ColumnWorking
	case b.Revising || b.OpenQuestions() > 0:
		return ColumnFeedback
	}
	return ColumnReview
}

// OpenQuestions counts AI questions you have not answered yet.
func (b *Batch) OpenQuestions() int {
	n := 0
	for _, m := range b.Thread {
		if m.Question && !m.Answered {
			n++
		}
	}
	return n
}

// AddMessage appends to the thread and returns the message.
func (b *Batch) AddMessage(m Message) Message {
	if m.ID == "" {
		m.ID = NewID()
	}
	if m.Created.IsZero() {
		m.Created = time.Now()
	}
	// A reply from you answers all open AI questions of the same batch.
	if m.Role == AuthorUser {
		for i := range b.Thread {
			if b.Thread[i].Question {
				b.Thread[i].Answered = true
			}
		}
	}
	b.Thread = append(b.Thread, m)
	b.Updated = m.Created
	return m
}

// Counts summarises the review progress.
type Counts struct {
	Files    int `json:"files"`
	Done     int `json:"done"` // files applied or skipped
	Hunks    int `json:"hunks"`
	Accepted int `json:"accepted"`
	Rejected int `json:"rejected"`
}

// Counts returns the progress of the whole batch.
func (b *Batch) Counts() Counts {
	var c Counts
	for _, f := range b.Files {
		c.Files++
		if f.Status != FileOpen {
			c.Done++
		}
		for _, h := range f.Diff().Hunks {
			c.Hunks++
			switch f.Decision(h.ID) {
			case Accepted:
				c.Accepted++
			case Rejected:
				c.Rejected++
			}
		}
	}
	return c
}

// Latest returns the current version.
func (f *File) Latest() Version {
	if len(f.Versions) == 0 {
		return Version{Content: f.Base}
	}
	return f.Versions[f.Current]
}

// AddVersion appends a version and makes it current.
func (f *File) AddVersion(v Version) Version {
	v.N = len(f.Versions) + 1
	if v.Created.IsZero() {
		v.Created = time.Now()
	}
	f.Versions = append(f.Versions, v)
	f.Current = len(f.Versions) - 1
	return v
}

// Diff compares the base with the current version (cached until base or version change).
func (f *File) Diff() *diff.Result {
	latest := f.Latest()
	key := fmt.Sprintf("%s/%d/%d/%d", f.BaseHash, len(f.Base), latest.N, len(latest.Content))
	if f.diff != nil && f.diffKey == key {
		return f.diff
	}
	f.diff, f.diffKey = diff.Compare(f.Base, latest.Content), key
	return f.diff
}

// Decision returns the state of a hunk.
func (f *File) Decision(id string) Decision {
	if d, ok := f.Decisions[id]; ok {
		return d
	}
	return Pending
}

// Decide stores a decision (Pending removes it).
func (f *File) Decide(id string, d Decision) {
	if f.Decisions == nil {
		f.Decisions = map[string]Decision{}
	}
	if d == Pending {
		delete(f.Decisions, id)
		return
	}
	f.Decisions[id] = d
}

// Open counts undecided hunks of the current version.
func (f *File) Open() int {
	n := 0
	for _, h := range f.Diff().Hunks {
		if f.Decision(h.ID) == Pending {
			n++
		}
	}
	return n
}

// Reason finds the AI's explanation for a hunk by matching the changed text.
func (f *File) Reason(h diff.Hunk) (string, string) {
	v := f.Latest()
	oldText, newText := join(h.OldLines), join(h.NewLines)
	for _, c := range v.Changes {
		if c.After != "" && contains(newText, c.After) {
			return c.Reason, c.Rule
		}
		if c.After == "" && c.Before != "" && contains(oldText, c.Before) {
			return c.Reason, c.Rule
		}
	}
	for _, c := range v.Changes {
		if c.Before != "" && contains(oldText, c.Before) {
			return c.Reason, c.Rule
		}
	}
	return "", ""
}
