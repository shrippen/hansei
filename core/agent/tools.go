package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"strings"

	"git.arianw.de/shrippen/hansei/core/batch"
	"git.arianw.de/shrippen/hansei/core/config"
	"git.arianw.de/shrippen/hansei/core/diff"
	"git.arianw.de/shrippen/hansei/core/llm"
	"git.arianw.de/shrippen/hansei/core/redact"
	"git.arianw.de/shrippen/hansei/core/vault"
)

const (
	listLimit   = 400
	searchLimit = 60
	// maxNote stays below the tool result cap, so the model always sees a note in full;
	// an edit based on a cut note would drop its tail.
	maxNote = 150_000
)

// run is the state of one agent run; tools are called one after another.
type run struct {
	find   bool // find-only: report notes instead of proposing edits
	env    Env
	scope  []string
	batch  *batch.Batch
	red    *redact.Redactor
	read   map[string]string // note path → content as the model read it (the base)
	exists map[string]bool
	props  map[string]*Proposal
	order  []string
	out    Outcome
}

func newRun(env Env, scope []string, b *batch.Batch) *run {
	return &run{env: env, scope: scope, batch: b, red: redact.New(), read: map[string]string{}, exists: map[string]bool{}, props: map[string]*Proposal{}}
}

// str is a JSON Schema string property.
func str(desc string) map[string]any { return map[string]any{"type": "string", "description": desc} }

func (r *run) tools() []llm.Tool {
	changeItem := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"before": str("the old text of the edit, a short exact snippet (empty for pure additions)"),
			"after":  str("the new text of the edit, a short exact snippet (empty for pure deletions)"),
			"reason": str("one sentence: why this edit"),
			"rule":   str("the rulebook section this follows, e.g. 'Design.md › Secrets' (optional)"),
		},
		"required": []string{"reason"},
	}
	tools := []llm.Tool{
		{Name: "list_notes", Description: "List the note paths in a folder of the vault (only folders in scope).",
			Properties: map[string]any{"folder": str("folder path, e.g. 'IT/Dienste'; empty for all folders in scope")},
			Run:        r.listNotes},
		{Name: "search", Description: "Find lines containing all given words (case-insensitive). Returns path:line: text.",
			Properties: map[string]any{"query": str("words to search for"), "folder": str("optional folder to limit the search")},
			Required:   []string{"query"}, Run: r.search},
		{Name: "read_note", Description: "Read the current content of a note from the vault.",
			Properties: map[string]any{"path": str("note path, e.g. 'IT/Geräte/Regis.md'")},
			Required:   []string{"path"}, Run: r.readNote},
		{Name: "backlinks", Description: "List notes that link to a note.",
			Properties: map[string]any{"path": str("note path")},
			Required:   []string{"path"}, Run: r.backlinks},
		{Name: "propose_edit", Description: "Propose the complete new content of one note. A human reviews it as a diff. Call again for the same path to replace your earlier proposal.",
			Properties: map[string]any{
				"path":    str("note path; new notes are allowed inside the scope"),
				"content": str("the complete new content of the note"),
				"summary": str("one sentence: what changed in this note"),
				"changes": map[string]any{"type": "array", "items": changeItem, "description": "one entry per edit"},
			},
			Required: []string{"path", "content", "summary", "changes"}, Run: r.proposeEdit},
		{Name: "ask_user", Description: "Ask the reviewer a short question when a fact is unclear. Do not guess instead.",
			Properties: map[string]any{"question": str("the question")},
			Required:   []string{"question"}, Run: r.askUser},
		{Name: "finish", Description: "Finish the run. Call exactly once at the end.",
			Properties: map[string]any{
				"title":   str("short title of the batch, at most 6 words"),
				"topic":   str("one-word topic, e.g. Secrets, Netzwerk, Backup"),
				"summary": str("at most two sentences"),
				"reply":   str("answer to the reviewer's feedback (revisions only)"),
			},
			Required: []string{"title", "topic", "summary"}, Run: r.finish},
	}
	if r.find {
		for i, t := range tools {
			if t.Name != "propose_edit" {
				continue
			}
			tools[i] = llm.Tool{Name: "report_note", Description: "Report one note the task concerns, with the reason. Changes nothing.",
				Properties: map[string]any{"path": str("note path"), "reason": str("one sentence: why this note needs work")},
				Required:   []string{"path", "reason"}, Run: r.reportNote}
		}
	}
	if r.batch != nil {
		tools = append(tools, llm.Tool{Name: "read_proposal", Description: "Read your current proposal for a note of this batch.",
			Properties: map[string]any{"path": str("note path")}, Required: []string{"path"}, Run: r.readProposal})
	}
	return tools
}

// inScope reports whether a note lies in the run's folders (all allowed folders if none).
func (r *run) inScope(p string) bool {
	if !r.env.Vault.Allowed(p) {
		return false
	}
	if len(r.scope) == 0 {
		return true
	}
	for _, f := range r.scope {
		if config.Within(p, f) {
			return true
		}
	}
	if r.batch != nil {
		_, ok := r.batch.File(p)
		return ok
	}
	return false
}

// external reports whether the note must not reach this (non-local) provider.
func (r *run) external(p string) bool {
	return r.env.Vault.LocalOnly(p) && !r.env.Provider.Local()
}

var errLocalOnly = errors.New("this note may only be sent to a local model; leave it unchanged")

func (r *run) listNotes(_ context.Context, raw json.RawMessage) (string, error) {
	var in struct {
		Folder string `json:"folder"`
	}
	if err := json.Unmarshal(raw, &in); err != nil {
		return "", err
	}
	folder := strings.Trim(in.Folder, "/")
	var b strings.Builder
	n := 0
	for _, note := range r.env.Vault.Notes() {
		if !r.inScope(note.Path) || !config.Within(note.Path, folder) {
			continue
		}
		if n++; n > listLimit {
			b.WriteString("… more notes, narrow the folder\n")
			break
		}
		fmt.Fprintf(&b, "%s (%d bytes)\n", note.Path, note.Size)
	}
	if n == 0 {
		return "no notes in scope for this folder", nil
	}
	return b.String(), nil
}

func (r *run) search(_ context.Context, raw json.RawMessage) (string, error) {
	var in struct {
		Query  string `json:"query"`
		Folder string `json:"folder"`
	}
	if err := json.Unmarshal(raw, &in); err != nil {
		return "", err
	}
	within := r.scope
	if f := strings.Trim(in.Folder, "/"); f != "" {
		within = []string{f}
	}
	var b strings.Builder
	n := 0
	for _, h := range r.env.Vault.Search(in.Query, within, 0) {
		if !r.inScope(h.Path) || r.external(h.Path) {
			continue
		}
		if n++; n > searchLimit {
			b.WriteString("… more hits, refine the query\n")
			break
		}
		fmt.Fprintf(&b, "%s:%d: %s\n", h.Path, h.Line, r.red.Redact(h.Text))
	}
	if n == 0 {
		return "no hits", nil
	}
	return b.String(), nil
}

func (r *run) readNote(_ context.Context, raw json.RawMessage) (string, error) {
	p, err := pathArg(raw)
	if err != nil {
		return "", err
	}
	if !r.inScope(p) {
		return "", fmt.Errorf("%s is outside the folders in scope", p)
	}
	if r.external(p) {
		return "", errLocalOnly
	}
	content, exists, err := r.env.Vault.Read(p)
	if err != nil {
		return "", err
	}
	if !exists {
		return "", fmt.Errorf("%s does not exist", p)
	}
	if len(content) > maxNote {
		return "", fmt.Errorf("%s is too large to edit safely (%d bytes); leave it unchanged", p, len(content))
	}
	if _, ok := r.read[p]; !ok || r.batch == nil {
		r.read[p], r.exists[p] = content, true
	}
	return r.red.Redact(r.read[p]), nil
}

func (r *run) readProposal(_ context.Context, raw json.RawMessage) (string, error) {
	p, err := pathArg(raw)
	if err != nil {
		return "", err
	}
	if prop, ok := r.props[p]; ok {
		return r.red.Redact(prop.Content), nil
	}
	f, ok := r.batch.File(p)
	if !ok {
		return "", fmt.Errorf("%s is not part of this batch", p)
	}
	return r.red.Redact(f.Latest().Content), nil
}

func (r *run) backlinks(_ context.Context, raw json.RawMessage) (string, error) {
	p, err := pathArg(raw)
	if err != nil {
		return "", err
	}
	var out []string
	for _, b := range r.env.Vault.Backlinks(p) {
		if r.inScope(b) {
			out = append(out, b)
		}
	}
	if len(out) == 0 {
		return "no backlinks in scope", nil
	}
	return strings.Join(out, "\n"), nil
}

func (r *run) proposeEdit(_ context.Context, raw json.RawMessage) (string, error) {
	var in struct {
		Path    string         `json:"path"`
		Content string         `json:"content"`
		Summary string         `json:"summary"`
		Changes []batch.Change `json:"changes"`
	}
	if err := json.Unmarshal(raw, &in); err != nil {
		return "", err
	}
	p := strings.TrimSpace(in.Path)
	if err := vault.CheckPath(p); err != nil {
		return "", fmt.Errorf("%s: not a valid note path (relative, ends in .md)", p)
	}
	if !r.inScope(p) {
		return "", fmt.Errorf("%s is outside the folders in scope", p)
	}
	if r.external(p) {
		return "", errLocalOnly
	}

	// Existing notes must have been read first, so the proposal is based on their content.
	base, read := r.read[p]
	if !read {
		current, exists, err := r.env.Vault.Read(p)
		if err != nil {
			return "", err
		}
		if exists {
			return "", fmt.Errorf("read %s with read_note before proposing a change", p)
		}
		base, r.read[p], r.exists[p] = current, current, false
	}

	if r.red.Placeholder(in.Content) {
		return "", errors.New("the content contains a secret placeholder that does not exist; keep placeholders exactly as read")
	}
	content := r.red.Restore(in.Content)
	if !diff.Compare(base, content).Changed() {
		return "", errors.New("the content has no changes compared to the note; nothing to propose")
	}
	if _, ok := r.props[p]; !ok && r.newFiles() >= r.env.Config.MaxBatchFiles {
		return "", fmt.Errorf("the batch already has %d notes, the limit; finish instead", r.env.Config.MaxBatchFiles)
	}

	for i := range in.Changes {
		in.Changes[i].Before = r.red.Restore(in.Changes[i].Before)
		in.Changes[i].After = r.red.Restore(in.Changes[i].After)
	}
	if _, ok := r.props[p]; !ok {
		r.order = append(r.order, p)
	}
	r.props[p] = &Proposal{Path: p, Content: content, Summary: in.Summary, Changes: in.Changes, Base: base, Exists: r.exists[p]}
	return fmt.Sprintf("proposal for %s stored (%s)", p, path.Base(p)), nil
}

// newFiles counts notes in the batch after this run (existing batch files plus new proposals).
func (r *run) newFiles() int {
	n := len(r.props)
	if r.batch == nil {
		return n
	}
	for _, f := range r.batch.Files {
		if _, ok := r.props[f.Path]; !ok {
			n++
		}
	}
	return n
}

func (r *run) reportNote(_ context.Context, raw json.RawMessage) (string, error) {
	var in struct {
		Path   string `json:"path"`
		Reason string `json:"reason"`
	}
	if err := json.Unmarshal(raw, &in); err != nil {
		return "", err
	}
	p := strings.TrimSpace(in.Path)
	if !r.inScope(p) {
		return "", fmt.Errorf("%s is outside the folders in scope", p)
	}
	for _, f := range r.out.Found {
		if f.Path == p {
			return "already reported", nil
		}
	}
	r.out.Found = append(r.out.Found, Found{Path: p, Reason: r.red.Restore(strings.TrimSpace(in.Reason))})
	return "reported", nil
}

func (r *run) askUser(_ context.Context, raw json.RawMessage) (string, error) {
	var in struct {
		Question string `json:"question"`
	}
	if err := json.Unmarshal(raw, &in); err != nil {
		return "", err
	}
	q := strings.TrimSpace(in.Question)
	if q == "" {
		return "", errors.New("empty question")
	}
	r.out.Questions = append(r.out.Questions, r.red.Restore(q))
	return "question stored; the reviewer answers later. Continue with what is clear, then finish.", nil
}

func (r *run) finish(_ context.Context, raw json.RawMessage) (string, error) {
	var in struct {
		Title   string `json:"title"`
		Topic   string `json:"topic"`
		Summary string `json:"summary"`
		Reply   string `json:"reply"`
	}
	if err := json.Unmarshal(raw, &in); err != nil {
		return "", err
	}
	r.out.Title = strings.TrimSpace(in.Title)
	r.out.Topic = strings.TrimSpace(in.Topic)
	r.out.Summary = r.red.Restore(strings.TrimSpace(in.Summary))
	r.out.Reply = r.red.Restore(strings.TrimSpace(in.Reply))
	return "done", nil
}

func pathArg(raw json.RawMessage) (string, error) {
	var in struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(raw, &in); err != nil {
		return "", err
	}
	return strings.TrimSpace(in.Path), nil
}
