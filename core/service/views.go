package service

import (
	"time"

	"git.arianw.de/shrippen/hansei/core/batch"
	"git.arianw.de/shrippen/hansei/core/config"
	"git.arianw.de/shrippen/hansei/core/diff"
	"git.arianw.de/shrippen/hansei/core/rules"
	"git.arianw.de/shrippen/hansei/core/stats"
)

// Views are the JSON shapes the UIs (TUI, KDE app, scripts) receive. They carry display
// data only, with secrets masked unless a view is explicitly revealed.

// ProviderView describes one AI provider.
type ProviderView struct {
	Name     string  `json:"name"`
	Kind     string  `json:"kind"`
	Model    string  `json:"model"`
	BaseURL  string  `json:"baseUrl,omitempty"`
	Fallback string  `json:"fallback,omitempty"`
	Local    bool    `json:"local"`
	HasKey   bool    `json:"hasKey"`
	Default  bool    `json:"default"`
	PriceIn  float64 `json:"priceIn,omitempty"`
	PriceOut float64 `json:"priceOut,omitempty"`
	Currency string  `json:"currency,omitempty"`
}

// StatusView is the header information of every screen.
type StatusView struct {
	Version   string         `json:"version"`
	Vault     string         `json:"vault"`
	VaultName string         `json:"vaultName"` // for obsidian://open links
	Allowed   []string       `json:"allowed"`
	Blocked   []string       `json:"blocked"`
	Scopes    []string       `json:"scopes"` // allowed folders and their sub folders (two levels), for scope pickers
	Notes     int            `json:"notes"`
	Scanned   time.Time      `json:"scanned"`
	Provider  string         `json:"provider"`
	Providers []ProviderView `json:"providers"`
	Style     string         `json:"style"`
	Lang      string         `json:"lang"`
	Demo      bool           `json:"demo"`
	Running   int            `json:"running"`
}

// FileSummary is one file in a batch list.
type FileSummary struct {
	Path     string `json:"path"`
	Status   string `json:"status"` // open, applied, skipped, undone, stale
	Open     int    `json:"open"`
	Hunks    int    `json:"hunks"`
	Accepted int    `json:"accepted"`
	Versions int    `json:"versions"`
	Feedback bool   `json:"feedback"` // last version came from a revision
	New      bool   `json:"new"`
}

// BatchSummary is one batch in lists and on the board.
type BatchSummary struct {
	ID        string        `json:"id"`
	Title     string        `json:"title"`
	Topic     string        `json:"topic"`
	Status    string        `json:"status"`
	Column    string        `json:"column"`
	Source    string        `json:"source"`
	Provider  string        `json:"provider,omitempty"`
	Progress  string        `json:"progress,omitempty"`
	Error     string        `json:"error,omitempty"`
	Counts    batch.Counts  `json:"counts"`
	Questions int           `json:"questions"`
	Found     int           `json:"found"` // notes a find-only task reported
	Revising  bool          `json:"revising"`
	Running   bool          `json:"running"`
	Created   time.Time     `json:"created"`
	Updated   time.Time     `json:"updated"`
	Usage     batch.Usage   `json:"usage"`
	Rule      string        `json:"rule,omitempty"`
	Files     []FileSummary `json:"files"`
}

// BatchView is a batch with its thread.
type BatchView struct {
	BatchSummary
	Instruction string             `json:"instruction"`
	Summary     string             `json:"summary"`
	Scope       []string           `json:"scope"`
	Thread      []batch.Message    `json:"thread"`
	Suggestions []batch.Suggestion `json:"suggestions"`
	Found       []batch.Found      `json:"found"`
}

// HunkView is one hunk with its state and explanation.
type HunkView struct {
	ID       string `json:"id"`
	Index    int    `json:"index"`
	Heading  string `json:"heading"`
	State    string `json:"state"`
	Reason   string `json:"reason,omitempty"`
	Rule     string `json:"rule,omitempty"`
	Rejected string `json:"rejected,omitempty"` // your reason for rejecting
	OldStart int    `json:"oldStart"`
	OldCount int    `json:"oldCount"`
	NewStart int    `json:"newStart"`
	NewCount int    `json:"newCount"`
	Feedback int    `json:"feedback"` // thread messages about this hunk
}

// VersionView is one version of a file.
type VersionView struct {
	N       int       `json:"n"`
	Author  string    `json:"author"`
	Note    string    `json:"note,omitempty"`
	Created time.Time `json:"created"`
}

// FileView is everything the review screen needs for one file.
type FileView struct {
	Batch    string        `json:"batch"`
	Path     string        `json:"path"`
	Status   string        `json:"status"`
	Stale    bool          `json:"stale"`
	Exists   bool          `json:"exists"`
	Summary  string        `json:"summary,omitempty"`
	Mode     string        `json:"mode"`
	Rows     []diff.Row    `json:"rows"`
	Hunks    []HunkView    `json:"hunks"`
	Versions []VersionView `json:"versions"`
	Current  int           `json:"current"` // version number shown
	Content  string        `json:"content"` // proposed content (masked unless revealed)
	Base     string        `json:"base"`    // vault content the proposal is based on (masked unless revealed)
	Revealed bool          `json:"revealed"`
}

// CompareView shows the change between two versions of a file.
type CompareView struct {
	From int        `json:"from"`
	To   int        `json:"to"`
	Rows []diff.Row `json:"rows"`
}

// DecideResult reports what a decision caused.
type DecideResult struct {
	File    FileSummary  `json:"file"`
	Batch   BatchSummary `json:"batch"`
	Written bool         `json:"written"`
	Skipped bool         `json:"skipped"`
	Done    bool         `json:"done"` // the batch is finished
	Message string       `json:"message,omitempty"`
}

// FindingsView is the rule report with display titles.
type FindingsView struct {
	rules.Report
	Titles map[string]string `json:"titles"`
	Open   map[string]string `json:"open"` // rule → ID of an open batch built from it
}

// HomeView is the start page.
type HomeView struct {
	Stats        stats.Summary     `json:"stats"`
	Waiting      int               `json:"waiting"`
	WaitingHunks int               `json:"waitingHunks"`
	Next         *BatchSummary     `json:"next,omitempty"`
	Findings     []rules.Summary   `json:"findings"`
	Titles       map[string]string `json:"titles"`
	Notes        int               `json:"notes"`
	Open         map[string]string `json:"open"` // rule → ID of an open batch built from it
	Checked      time.Time         `json:"checked"`
}

// Estimate is the expected size of a task.
type Estimate struct {
	Notes    int      `json:"notes"`
	Tokens   int      `json:"tokens"`
	Cost     float64  `json:"cost"`
	HasPrice bool     `json:"hasPrice"`
	Currency string   `json:"currency,omitempty"`
	Local    bool     `json:"local"`
	Blocked  []string `json:"blocked,omitempty"` // local-only notes this provider may not see
}

// JournalView is one write without its contents.
type JournalView struct {
	ID      string    `json:"id"`
	Batch   string    `json:"batch"`
	Title   string    `json:"title"`
	Path    string    `json:"path"`
	Time    time.Time `json:"time"`
	Undone  bool      `json:"undone"`
	New     bool      `json:"new"`
	Added   int       `json:"added"`
	Removed int       `json:"removed"`
}

// FolderView is one folder in the settings tree (up to three levels).
type FolderView struct {
	Name      string   `json:"name"`
	Path      string   `json:"path"`
	Depth     int      `json:"depth"`
	Allowed   bool     `json:"allowed"`
	Blocked   bool     `json:"blocked"`
	LocalOnly bool     `json:"localOnly"`
	Rulebook  []string `json:"rulebook"`
}

// SettingsView is the editable part of the config.
type SettingsView struct {
	Vault         string              `json:"vault"`
	Folders       []FolderView        `json:"folders"`
	Allow         []string            `json:"allow"`
	Block         []string            `json:"block"`
	LocalOnly     []string            `json:"localOnly"`
	Provider      string              `json:"provider"`
	Providers     []ProviderView      `json:"providers"`
	Style         string              `json:"style"`
	MaxBatchFiles int                 `json:"maxBatchFiles"`
	ReviewDays    int                 `json:"reviewDays"`
	ReviewField   string              `json:"reviewField"`
	Codenames     map[string]string   `json:"codenames"`
	Required      map[string][]string `json:"required"`
	Rulebooks     []config.Rulebook   `json:"rulebooks"`
	DefaultRules  []string            `json:"defaultRules"`
	Path          string              `json:"path"`
}

// SettingsPatch changes settings; nil fields stay as they are.
type SettingsPatch struct {
	Allow      *[]string            `json:"allow,omitempty"`
	Block      *[]string            `json:"block,omitempty"`
	LocalOnly  *[]string            `json:"localOnly,omitempty"`
	Provider   *string              `json:"provider,omitempty"`
	Providers  *[]ProviderView      `json:"providers,omitempty"`
	Style      *string              `json:"style,omitempty"`
	Codenames  *map[string]string   `json:"codenames,omitempty"`
	Required   *map[string][]string `json:"required,omitempty"`
	ReviewDays *int                 `json:"reviewDays,omitempty"`
	Rulebooks  *[]config.Rulebook   `json:"rulebooks,omitempty"`
}

// ProviderCheck is the result of testing a provider.
type ProviderCheck struct {
	OK      bool     `json:"ok"`
	Message string   `json:"message"`
	Models  []string `json:"models"`
}

// RuleSectionView is one section of a rulebook note, e.g. "Design.md › Secrets".
type RuleSectionView struct {
	File    string `json:"file"`
	Heading string `json:"heading"`
	Text    string `json:"text"`
}

// TaskInput starts an AI task.
type TaskInput struct {
	Instruction string   `json:"instruction"`
	Scope       []string `json:"scope"`
	Provider    string   `json:"provider,omitempty"`
	FindOnly    bool     `json:"findOnly,omitempty"` // only list affected notes, propose nothing
}

// FeedbackInput sends feedback to the AI.
type FeedbackInput struct {
	Batch    string `json:"batch"`
	Scope    string `json:"scope"` // batch, file, hunk
	Path     string `json:"path,omitempty"`
	Hunk     string `json:"hunk,omitempty"`
	Text     string `json:"text"`
	Quick    string `json:"quick,omitempty"`
	Remember bool   `json:"remember,omitempty"`
}

// Event is pushed to subscribers when something changes.
type Event struct {
	Kind  string `json:"kind"` // batch, progress, ai, toast, vault, settings
	Batch string `json:"batch,omitempty"`
	Text  string `json:"text,omitempty"`
	Path  string `json:"path,omitempty"` // toast about a written file: offers undo
}

// Event kinds.
const (
	EventBatch    = "batch"
	EventProgress = "progress"
	EventAI       = "ai"
	EventToast    = "toast"
	EventVault    = "vault"
	EventSettings = "settings"
	EventUsage    = "usage" // live token use of a running batch, JSON batch.Usage in Text
)
