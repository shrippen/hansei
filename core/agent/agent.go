// Package agent turns a task or a piece of feedback into proposals, using an llm.Provider
// with tools that read the vault. It never writes to the vault and never touches batches;
// the service applies the Outcome.
//
//	task/feedback ─▶ prompt + rulebook ─▶ model ⇄ tools (list, search, read, backlinks)
//	                                          └─▶ propose_edit / ask_user / finish ─▶ Outcome
package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"git.arianw.de/shrippen/hansei/core/batch"
	"git.arianw.de/shrippen/hansei/core/config"
	"git.arianw.de/shrippen/hansei/core/llm"
	"git.arianw.de/shrippen/hansei/core/redact"
	"git.arianw.de/shrippen/hansei/core/rules"
	"git.arianw.de/shrippen/hansei/core/vault"
)

// Env is what one run may use.
type Env struct {
	Vault    *vault.Vault
	Config   *config.Config
	Provider llm.Provider
	Lang     string // language for titles and replies, e.g. "de"
	Now      time.Time
	OnEvent  func(llm.Event)
}

// Proposal is the new full content of one note.
type Proposal struct {
	Path    string
	Content string
	Summary string
	Changes []batch.Change
	Base    string // vault content the model read
	Exists  bool
}

// Outcome is everything a run produced.
type Outcome struct {
	Title     string
	Topic     string
	Summary   string
	Reply     string
	Proposals []Proposal
	Questions []string
	Found     []Found
	Usage     llm.Usage
}

// Found is a note a find-only run reports.
type Found struct {
	Path   string
	Reason string
}

var (
	// ErrNothing means the model finished without proposals or questions.
	ErrNothing = errors.New("agent: no proposals")
	// ErrNoProvider means the environment has no provider.
	ErrNoProvider = errors.New("agent: no AI provider configured")
)

// Create runs a new task: find the affected notes and propose edits.
func Create(ctx context.Context, env Env, instruction string, scope []string) (Outcome, error) {
	r := newRun(env, scope, nil)
	prompt := fmt.Sprintf(createPrompt, env.Now.Format(time.DateOnly), strings.Join(scopeNames(scope, env.Config), ", "),
		env.Config.MaxBatchFiles, langName(env.Lang), r.red.Redact(instruction))
	return r.run(ctx, prompt)
}

// Find lists the notes a task concerns without proposing any change ("Nur Befunde suchen").
func Find(ctx context.Context, env Env, instruction string, scope []string) (Outcome, error) {
	r := newRun(env, scope, nil)
	r.find = true
	prompt := fmt.Sprintf(findPrompt, env.Now.Format(time.DateOnly), strings.Join(scopeNames(scope, env.Config), ", "),
		langName(env.Lang), r.red.Redact(instruction))
	return r.run(ctx, prompt)
}

// Revise applies feedback to an existing batch.
func Revise(ctx context.Context, env Env, b *batch.Batch, feedback batch.Message) (Outcome, error) {
	r := newRun(env, b.Scope, b)
	var state strings.Builder
	for _, f := range b.Files {
		r.read[f.Path] = f.Base
		r.exists[f.Path] = f.Exists
		fmt.Fprintf(&state, "<file path=%q status=%q>\n<proposed_content>\n%s\n</proposed_content>\n<decisions>\n%s</decisions>\n</file>\n",
			f.Path, f.Status, r.red.Redact(f.Latest().Content), decisions(f))
	}
	var thread strings.Builder
	for _, m := range b.Thread {
		fmt.Fprintf(&thread, "[%s%s] %s\n", m.Role, where(m), r.red.Redact(m.Text))
	}
	prompt := fmt.Sprintf(revisePrompt, env.Now.Format(time.DateOnly), langName(env.Lang), r.red.Redact(b.Instruction),
		state.String(), thread.String(), where(feedback), quick(feedback), r.red.Redact(feedback.Text))
	return r.run(ctx, prompt)
}

// run executes the model with the tools and collects the outcome.
func (r *run) run(ctx context.Context, prompt string) (Outcome, error) {
	if r.env.Provider == nil {
		return Outcome{}, ErrNoProvider
	}
	book := rules.Rulebook(r.env.Vault, r.env.Config, r.scope)
	system := systemPrompt + "\n\n" + book.Text

	res, err := r.env.Provider.Run(ctx, llm.Request{
		System:   system,
		Prompt:   prompt,
		Tools:    r.tools(),
		MaxTurns: r.env.Config.MaxTurns,
		OnEvent:  r.env.OnEvent,
	})
	r.out.Usage = res.Usage
	if err != nil {
		return r.out, err
	}
	if r.out.Reply == "" {
		r.out.Reply = strings.TrimSpace(res.Text)
	}
	for _, p := range r.order {
		r.out.Proposals = append(r.out.Proposals, *r.props[p])
	}
	if len(r.out.Proposals) == 0 && len(r.out.Questions) == 0 && len(r.out.Found) == 0 && r.batch == nil {
		return r.out, ErrNothing
	}
	return r.out, nil
}

// decisions lists your decisions on the current version for the model.
func decisions(f *batch.File) string {
	var b strings.Builder
	for _, h := range f.Diff().Hunks {
		d := f.Decision(h.ID)
		if d == batch.Pending {
			continue
		}
		fmt.Fprintf(&b, "- %s: %q → %q", d, strings.Join(h.OldLines, "\n"), strings.Join(h.NewLines, "\n"))
		if reason := f.Reasons[h.ID]; reason != "" {
			fmt.Fprintf(&b, " (reason: %s)", reason)
		}
		b.WriteString("\n")
	}
	return redact.New().Redact(b.String())
}

func where(m batch.Message) string {
	switch m.Scope {
	case batch.ScopeFile:
		return " on file " + m.Path
	case batch.ScopeHunk:
		return fmt.Sprintf(" on a change in %s (hunk %s)", m.Path, m.Hunk)
	}
	return ""
}

func quick(m batch.Message) string {
	if m.Quick == "" {
		return ""
	}
	return "Quick reason: " + m.Quick + "\n"
}

func scopeNames(scope []string, c *config.Config) []string {
	if len(scope) == 0 {
		return c.Allow
	}
	return scope
}

func langName(lang string) string {
	if strings.HasPrefix(lang, "en") {
		return "English"
	}
	return "German"
}
