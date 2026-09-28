package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"sort"
	"strings"
	"unicode"

	"git.arianw.de/shrippen/hansei/core/agent"
	"git.arianw.de/shrippen/hansei/core/batch"
	"git.arianw.de/shrippen/hansei/core/config"
	"git.arianw.de/shrippen/hansei/core/diff"
	"git.arianw.de/shrippen/hansei/core/i18n"
	"git.arianw.de/shrippen/hansei/core/llm"
	"git.arianw.de/shrippen/hansei/core/rules"
	"git.arianw.de/shrippen/hansei/core/vault"
)

const (
	charsPerToken     = 4
	promptOverhead    = 2500 // system prompt, tool definitions
	repeatThreshold   = 3    // similar feedback this often → rule suggestion
	similarity        = 0.5
	minWordLen        = 4
	progressTextLimit = 140
	perMillion        = 1_000_000
)

// Task starts an AI run that builds a new batch. It returns at once; progress arrives as events.
func (s *Service) Task(in TaskInput) (BatchSummary, error) {
	instruction := strings.TrimSpace(in.Instruction)
	if instruction == "" {
		return BatchSummary{}, errors.New("empty task")
	}
	scope, err := s.cleanScope(in.Scope)
	if err != nil {
		return BatchSummary{}, err
	}
	prov, err := s.provider(in.Provider)
	if err != nil {
		return BatchSummary{}, err
	}

	now := s.opts.Now()
	b := &batch.Batch{ID: batch.NewID(), Title: shortTitle(instruction), Instruction: instruction, Scope: scope,
		Source: batch.SourceAI, Provider: prov.Name(), Status: batch.StatusWorking, Created: now, Updated: now}
	s.store.Lock()
	err = s.store.Put(b)
	sum := s.summary(b)
	s.store.Unlock()
	if err != nil {
		return BatchSummary{}, err
	}
	s.publish(Event{Kind: EventBatch, Batch: b.ID})

	findOnly := in.FindOnly
	s.start(b.ID, func(ctx context.Context) {
		var out agent.Outcome
		var err error
		if findOnly {
			out, err = agent.Find(ctx, s.env(prov, b.ID), instruction, scope)
		} else {
			out, err = agent.Create(ctx, s.env(prov, b.ID), instruction, scope)
		}
		s.finishCreate(b.ID, prov, out, err)
	})
	return sum, nil
}

// cleanScope keeps allowed folders only; an empty scope means all allowed folders.
func (s *Service) cleanScope(scope []string) ([]string, error) {
	var out []string
	for _, f := range scope {
		f = strings.Trim(path.Clean("/"+strings.TrimSpace(f)), "/")
		if f == "" {
			continue
		}
		ok := false
		for _, a := range s.cfg.Allow {
			if config.Within(f, a) {
				ok = true
			}
		}
		for _, bl := range s.cfg.Block {
			if config.Within(f, bl) {
				ok = false
			}
		}
		if !ok {
			return nil, fmt.Errorf("%s is not an allowed folder", f)
		}
		out = append(out, f)
	}
	return out, nil
}

// env builds the agent environment; AI events become progress events of the batch.
// The run gets its own copy of the config, so a settings change cannot pull it away mid-run.
func (s *Service) env(prov llm.Provider, id string) agent.Env {
	s.cfgMu.Lock()
	cfg := *s.cfg
	s.cfgMu.Unlock()
	return agent.Env{Vault: s.vault, Config: &cfg, Provider: prov, Lang: s.lang, Now: s.opts.Now(),
		OnEvent: func(e llm.Event) { s.progress(id, prov, e) }}
}

func (s *Service) progress(id string, prov llm.Provider, e llm.Event) {
	switch e.Kind {
	case llm.EventUsage:
		// Live cost: what the batch used before plus this run so far.
		s.store.Lock()
		var live batch.Usage
		if b, err := s.store.Get(id); err == nil {
			live = b.Usage
		}
		s.store.Unlock()
		live.In += e.Usage.In
		live.Out += e.Usage.Out
		if pc := s.priceOf(prov); pc != nil {
			live.Cost += (float64(e.Usage.In)*pc.PriceIn + float64(e.Usage.Out)*pc.PriceOut) / perMillion
		}
		raw, _ := json.Marshal(live)
		s.publish(Event{Kind: EventUsage, Batch: id, Text: string(raw)})
	case llm.EventTool:
		text := e.Tool
		if e.Input != "" {
			text += " " + e.Input
		}
		if r := []rune(text); len(r) > progressTextLimit {
			text = string(r[:progressTextLimit]) + "…"
		}
		s.store.Lock()
		if b, err := s.store.Get(id); err == nil {
			b.Progress = text
		}
		s.store.Unlock()
		s.publish(Event{Kind: EventProgress, Batch: id, Text: text})
	case llm.EventText:
		s.publish(Event{Kind: EventAI, Batch: id, Text: e.Text})
	}
}

// start runs fn in the background and tracks it for Close and Cancel.
func (s *Service) start(id string, fn func(ctx context.Context)) {
	ctx, cancel := context.WithCancel(context.Background())
	s.runMu.Lock()
	s.running[id] = cancel
	s.runMu.Unlock()
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		defer func() {
			s.runMu.Lock()
			delete(s.running, id)
			s.runMu.Unlock()
			cancel()
			s.publish(Event{Kind: EventBatch, Batch: id})
		}()
		fn(ctx)
	}()
}

// Cancel stops a running AI task of a batch.
func (s *Service) Cancel(id string) { s.cancel(id) }

func (s *Service) cancel(id string) {
	s.runMu.Lock()
	cancel, ok := s.running[id]
	s.runMu.Unlock()
	if ok {
		cancel()
	}
}

// finishCreate stores the outcome of a create run.
func (s *Service) finishCreate(id string, prov llm.Provider, out agent.Outcome, runErr error) {
	s.store.Lock()
	defer s.store.Unlock()
	b, err := s.store.Get(id)
	if err != nil {
		return
	}
	s.addUsage(b, prov, out.Usage)
	b.Progress = ""
	b.Updated = s.opts.Now()
	if out.Title != "" {
		b.Title = out.Title
	}
	b.Topic, b.Summary = out.Topic, out.Summary
	s.addProposals(b, out.Proposals, batch.AuthorAI, "")
	for _, f := range out.Found {
		b.Found = append(b.Found, batch.Found{Path: f.Path, Reason: f.Reason})
	}
	for _, q := range out.Questions {
		b.AddMessage(batch.Message{Role: batch.AuthorAI, Text: q, Question: true, Scope: batch.ScopeBatch})
	}

	switch {
	case runErr != nil && len(b.Files) == 0:
		b.Status, b.Error = batch.StatusFailed, i18n.T(s.lang, "failed", runErr.Error())
	case len(b.Files) == 0 && len(out.Questions) == 0 && len(b.Found) == 0:
		b.Status, b.Error = batch.StatusFailed, i18n.T(s.lang, "noProposal")
	default:
		b.Status = batch.StatusReview
		if runErr != nil {
			b.Error = i18n.T(s.lang, "failed", runErr.Error())
		}
	}
	_ = s.store.Save(b)
	s.publish(Event{Kind: EventBatch, Batch: b.ID})
}

// addProposals turns proposals into files/versions of the batch. A proposal without line
// changes (e.g. only the final newline differs) adds nothing; if a revision drops every change
// of a file, the file is skipped, so it never blocks the batch.
func (s *Service) addProposals(b *batch.Batch, props []agent.Proposal, author batch.Author, note string) {
	for _, p := range props {
		f, ok := b.File(p.Path)
		changed := diff.Compare(p.Base, p.Content).Changed()
		if ok && !changed && f.Status == batch.FileOpen {
			f.Status = batch.FileSkipped
			continue
		}
		if !changed {
			continue
		}
		if !ok {
			f = &batch.File{Path: p.Path, Base: p.Base, BaseHash: vault.Hash(p.Base), Exists: p.Exists, Status: batch.FileOpen, Decisions: map[string]batch.Decision{}}
			b.Files = append(b.Files, f)
		}
		if f.Status != batch.FileOpen {
			continue // a finished file is not reopened by a revision
		}
		f.Summary = p.Summary
		f.AddVersion(batch.Version{Content: p.Content, Author: author, Note: note, Changes: p.Changes, Created: s.opts.Now()})
	}
}

func (s *Service) addUsage(b *batch.Batch, prov llm.Provider, u llm.Usage) {
	b.Usage.In += u.In
	b.Usage.Out += u.Out
	if pc := s.priceOf(prov); pc != nil {
		b.Usage.Cost += (float64(u.In)*pc.PriceIn + float64(u.Out)*pc.PriceOut) / perMillion
	}
}

// priceOf returns the configured provider with its prices, or nil.
func (s *Service) priceOf(prov llm.Provider) *config.Provider {
	if prov == nil {
		return nil
	}
	s.cfgMu.Lock()
	defer s.cfgMu.Unlock()
	if pc, ok := s.cfg.ProviderByName(prov.Name()); ok {
		return &pc
	}
	return nil
}

// Feedback stores your message and lets the AI revise the batch.
func (s *Service) Feedback(in FeedbackInput) (batch.Message, error) {
	text := strings.TrimSpace(in.Text)
	if text == "" && in.Quick == "" {
		return batch.Message{}, errors.New("empty feedback")
	}
	s.store.Lock()
	b, err := s.store.Get(in.Batch)
	if err != nil {
		s.store.Unlock()
		return batch.Message{}, err
	}
	s.runMu.Lock()
	_, running := s.running[b.ID]
	s.runMu.Unlock()
	if running {
		s.store.Unlock()
		return batch.Message{}, ErrRunning
	}
	scope := batch.Scope(in.Scope)
	if scope == "" {
		scope = batch.ScopeBatch
	}
	msg := b.AddMessage(batch.Message{Role: batch.AuthorUser, Text: text, Scope: scope, Path: in.Path, Hunk: in.Hunk,
		Quick: in.Quick, Remember: in.Remember, Created: s.opts.Now()})
	b.Revising = true
	if b.Status == batch.StatusDone || b.Status == batch.StatusFailed {
		b.Status = batch.StatusReview
	}
	s.suggestFromRepeats(b, msg)
	if err := s.store.Save(b); err != nil {
		s.store.Unlock()
		return msg, err
	}
	snapshot := copyBatch(b)
	s.store.Unlock()
	s.publish(Event{Kind: EventBatch, Batch: b.ID})

	prov, err := s.provider(b.Provider)
	if err != nil {
		s.endRevision(b.ID, nil, agent.Outcome{}, err)
		return msg, err
	}
	s.start(b.ID, func(ctx context.Context) {
		out, err := agent.Revise(ctx, s.env(prov, b.ID), snapshot, msg)
		s.endRevision(b.ID, prov, out, err)
	})

	if in.Remember {
		_, _ = s.RuleBatch(b.ID, text)
	}
	return msg, nil
}

// Regenerate asks for a new proposal for one hunk (or the whole file if hunk is empty).
func (s *Service) Regenerate(id, path, hunk string) (batch.Message, error) {
	scope := batch.ScopeFile
	if hunk != "" {
		scope = batch.ScopeHunk
	}
	return s.Feedback(FeedbackInput{Batch: id, Scope: string(scope), Path: path, Hunk: hunk, Text: i18n.T(s.lang, "regenerate")})
}

// endRevision stores the outcome of a revision run.
func (s *Service) endRevision(id string, prov llm.Provider, out agent.Outcome, runErr error) {
	s.store.Lock()
	defer s.store.Unlock()
	b, err := s.store.Get(id)
	if err != nil {
		return
	}
	if prov != nil {
		s.addUsage(b, prov, out.Usage)
	}
	b.Revising, b.Progress, b.Error = false, "", ""
	b.Updated = s.opts.Now()
	before := versionCounts(b)
	s.addProposals(b, out.Proposals, batch.AuthorAI, "feedback")
	created := newVersions(b, before)
	for _, q := range out.Questions {
		b.AddMessage(batch.Message{Role: batch.AuthorAI, Text: q, Question: true, Scope: batch.ScopeBatch})
	}
	reply := out.Reply
	if runErr != nil {
		reply = i18n.T(s.lang, "failed", runErr.Error())
	}
	if reply == "" && len(out.Proposals) > 0 {
		reply = i18n.T(s.lang, "revisedReply")
	}
	if reply != "" {
		b.AddMessage(batch.Message{Role: batch.AuthorAI, Text: reply, Scope: batch.ScopeBatch, Versions: created})
	}
	if runErr == nil {
		b.AddMessage(batch.Message{Role: batch.AuthorSystem, Text: s.revisionSummary(b, created), Scope: batch.ScopeBatch, Versions: created})
	}
	if s.finished(b) {
		b.Status = batch.StatusDone
	}
	_ = s.store.Save(b)
	s.publish(Event{Kind: EventBatch, Batch: b.ID})
}

// versionCounts remembers how many versions each file has before a revision.
func versionCounts(b *batch.Batch) map[string]int {
	out := map[string]int{}
	for _, f := range b.Files {
		out[f.Path] = len(f.Versions)
	}
	return out
}

// newVersions lists the files a revision gave a new version, e.g. {"IT/x.md": 3}.
func newVersions(b *batch.Batch, before map[string]int) map[string]int {
	out := map[string]int{}
	for _, f := range b.Files {
		if n := len(f.Versions); n > before[f.Path] {
			out[f.Path] = n
		}
	}
	return out
}

// revisionSummary says what a round changed: "v3 · Nextcloud.md: 2 Änderungen · Invoice Ninja.md unverändert".
func (s *Service) revisionSummary(b *batch.Batch, created map[string]int) string {
	if len(created) == 0 {
		return i18n.T(s.lang, "noNewVersion")
	}
	var parts []string
	for _, f := range b.Files {
		n, ok := created[f.Path]
		if !ok {
			parts = append(parts, i18n.T(s.lang, "unchangedFile", path.Base(f.Path)))
			continue
		}
		parts = append(parts, i18n.T(s.lang, "newVersion", n, path.Base(f.Path), len(f.Diff().Hunks)))
	}
	return strings.Join(parts, " · ")
}

// suggestFromRepeats adds a rule suggestion when you gave similar feedback several times.
func (s *Service) suggestFromRepeats(b *batch.Batch, msg batch.Message) {
	words := wordSet(msg.Text)
	if len(words) == 0 || msg.Remember {
		return
	}
	count := 0
	for _, other := range s.store.All() {
		for _, m := range other.Thread {
			if m.Role == batch.AuthorUser && m.ID != msg.ID && jaccard(words, wordSet(m.Text)) >= similarity {
				count++
			}
		}
	}
	if count+1 < repeatThreshold {
		return
	}
	for _, sg := range b.Suggestions {
		if jaccard(words, wordSet(sg.Text)) >= similarity {
			return
		}
	}
	b.Suggestions = append(b.Suggestions, batch.Suggestion{ID: batch.NewID(), Text: msg.Text, Count: count + 1,
		Status: batch.SuggestionOpen, Target: s.ruleFile(b.Scope)})
}

// Suggestion accepts (creates a rule batch) or dismisses a rule suggestion.
func (s *Service) Suggestion(id, sid string, accept bool) (BatchSummary, error) {
	s.store.Lock()
	b, err := s.store.Get(id)
	if err != nil {
		s.store.Unlock()
		return BatchSummary{}, err
	}
	var text string
	for i := range b.Suggestions {
		if b.Suggestions[i].ID != sid {
			continue
		}
		text = b.Suggestions[i].Text
		b.Suggestions[i].Status = batch.SuggestionDismissed
		if accept {
			b.Suggestions[i].Status = batch.SuggestionAccepted
		}
	}
	_ = s.store.Save(b)
	s.store.Unlock()
	if text == "" {
		return BatchSummary{}, errors.New("unknown suggestion")
	}
	if !accept {
		s.publish(Event{Kind: EventBatch, Batch: id})
		return BatchSummary{}, nil
	}
	return s.RuleBatch(id, text)
}

// RuleBatch lets the AI add a convention to the rulebook of the batch's folder, as its own batch.
func (s *Service) RuleBatch(fromBatch, text string) (BatchSummary, error) {
	s.store.Lock()
	b, err := s.store.Get(fromBatch)
	scope := []string(nil)
	provider := ""
	if err == nil {
		scope, provider = b.Scope, b.Provider
		if len(scope) == 0 && len(b.Files) > 0 {
			scope = []string{path.Dir(b.Files[0].Path)}
		}
	}
	s.store.Unlock()
	if err != nil {
		return BatchSummary{}, err
	}

	file := s.ruleFile(scope)
	if file == "" {
		return BatchSummary{}, errors.New(i18n.T(s.lang, "noRulebook"))
	}
	sum, err := s.Task(TaskInput{Instruction: i18n.T(s.lang, "ruleTask", file, text), Scope: []string{path.Dir(file)}, Provider: provider})
	if err != nil {
		return sum, err
	}
	s.store.Lock()
	if nb, err := s.store.Get(sum.ID); err == nil {
		nb.Source, nb.Title, nb.Topic = batch.SourceRule, i18n.T(s.lang, "ruleTitle"), "Regeln"
		_ = s.store.Save(nb)
		sum = s.summary(nb)
	}
	s.store.Unlock()
	return sum, nil
}

// ruleFile picks the first editable rulebook note for the folders.
func (s *Service) ruleFile(scope []string) string {
	folders := scope
	if len(folders) == 0 {
		folders = s.cfg.Allow
	}
	for _, f := range folders {
		for _, file := range s.cfg.RulebookFiles(f) {
			if s.vault.Allowed(file) {
				return file
			}
		}
	}
	return ""
}

// Estimate predicts the size of a task: the rulebook, the prompt and at most the notes in scope.
func (s *Service) Estimate(in TaskInput) (Estimate, error) {
	scope, err := s.cleanScope(in.Scope)
	if err != nil {
		return Estimate{}, err
	}
	prov, err := s.provider(in.Provider)
	if err != nil {
		return Estimate{}, err
	}
	book := rules.Rulebook(s.vault, s.cfg, scope)
	chars := promptOverhead*charsPerToken + len(book.Text) + len(in.Instruction)
	est := Estimate{Local: prov.Local()}
	for _, n := range s.vault.Notes() {
		if !inFolders(n.Path, scope, s.cfg.Allow) {
			continue
		}
		if s.vault.LocalOnly(n.Path) && !prov.Local() {
			est.Blocked = append(est.Blocked, n.Path)
			continue
		}
		est.Notes++
		chars += n.Size
	}
	est.Tokens = chars / charsPerToken
	if pc, ok := s.cfg.ProviderByName(prov.Name()); ok && pc.PriceIn > 0 {
		est.HasPrice, est.Currency = true, pc.Currency
		est.Cost = float64(est.Tokens) * pc.PriceIn / perMillion
	}
	return est, nil
}

func inFolders(p string, scope, fallback []string) bool {
	if len(scope) == 0 {
		scope = fallback
	}
	for _, f := range scope {
		if config.Within(p, f) {
			return true
		}
	}
	return false
}

// Findings runs the checks (cached for a few seconds).
func (s *Service) Findings(refresh bool) FindingsView {
	s.rescan(refresh)
	s.repMu.Lock()
	if s.report == nil || refresh || s.opts.Now().Sub(s.report.Checked) > findingsKeep {
		rep := rules.Check(s.vault, s.cfg, s.opts.Now())
		s.report = &rep
		_ = s.stats.Conformity(s.opts.Now(), rep.Conformity)
	}
	rep := *s.report
	s.repMu.Unlock()

	fv := FindingsView{Report: rep, Titles: s.ruleTitles(), Open: map[string]string{}}
	if fv.Findings == nil {
		fv.Findings = []rules.Finding{}
	}
	if fv.Summary == nil {
		fv.Summary = []rules.Summary{}
	}
	s.store.Lock()
	for _, b := range s.store.All() {
		if b.Rule != "" && b.Status != batch.StatusDone && b.Status != batch.StatusFailed {
			fv.Open[b.Rule] = b.ID
		}
	}
	s.store.Unlock()
	return fv
}

func (s *Service) ruleTitles() map[string]string {
	out := map[string]string{}
	for _, id := range rules.Order {
		out[string(id)] = i18n.T(s.lang, "rule."+string(id))
	}
	return out
}

// FromFinding builds a batch for all findings of one rule: code names with a known
// replacement are fixed without AI, everything else goes to the AI.
func (s *Service) FromFinding(rule string, provider string) (BatchSummary, error) {
	rep := s.Findings(true)
	var found []rules.Finding
	for _, f := range rep.Findings {
		if string(f.Rule) == rule {
			found = append(found, f)
		}
	}
	if len(found) == 0 {
		return BatchSummary{}, fmt.Errorf("no findings for %s", rule)
	}
	if rules.ID(rule) == rules.Codename {
		if sum, ok, err := s.codenameBatch(found); ok || err != nil {
			return sum, err
		}
	}

	paths := map[string]bool{}
	var list strings.Builder
	for _, f := range found {
		fmt.Fprintf(&list, "- %s:%d %s %s\n", f.Path, f.Line, f.Detail, f.Excerpt)
		paths[path.Dir(f.Path)] = true
	}
	var scope []string
	for p := range paths {
		scope = append(scope, p)
	}
	sort.Strings(scope)
	sum, err := s.Task(TaskInput{Instruction: i18n.T(s.lang, "findingTask", i18n.T(s.lang, "rule."+rule), list.String()), Scope: scope, Provider: provider})
	if err != nil {
		return sum, err
	}
	s.store.Lock()
	if b, err := s.store.Get(sum.ID); err == nil {
		b.Rule, b.Topic = rule, i18n.T(s.lang, "rule."+rule)
		_ = s.store.Save(b)
		sum = s.summary(b)
	}
	s.store.Unlock()
	return sum, nil
}

// codenameBatch replaces code names that have a configured replacement. ok=false if none has one.
func (s *Service) codenameBatch(found []rules.Finding) (BatchSummary, bool, error) {
	byPath := map[string]bool{}
	for _, f := range found {
		byPath[f.Path] = true
	}
	now := s.opts.Now()
	b := &batch.Batch{ID: batch.NewID(), Title: i18n.T(s.lang, "codenameTitle"), Topic: i18n.T(s.lang, "rule.codename"),
		Source: batch.SourceRules, Rule: string(rules.Codename), Status: batch.StatusReview, Created: now, Updated: now,
		Summary: i18n.T(s.lang, "codenameSummary")}

	paths := make([]string, 0, len(byPath))
	for p := range byPath {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, p := range paths {
		if len(b.Files) >= s.cfg.MaxBatchFiles {
			break
		}
		content, exists, err := s.vault.Read(p)
		if err != nil || !exists {
			continue
		}
		out, changes := s.replaceCodenames(content)
		if out == content {
			continue
		}
		s.addProposals(b, []agent.Proposal{{Path: p, Content: out, Base: content, Exists: true, Changes: changes}}, batch.AuthorRules, "")
	}
	if len(b.Files) == 0 {
		return BatchSummary{}, false, nil
	}
	s.store.Lock()
	defer s.store.Unlock()
	if err := s.store.Put(b); err != nil {
		return BatchSummary{}, true, err
	}
	s.publish(Event{Kind: EventBatch, Batch: b.ID})
	return s.summary(b), true, nil
}

func (s *Service) replaceCodenames(content string) (string, []batch.Change) {
	var changes []batch.Change
	olds := make([]string, 0, len(s.cfg.Checks.Codenames))
	for old := range s.cfg.Checks.Codenames {
		olds = append(olds, old)
	}
	sort.Strings(olds)
	for _, old := range olds {
		repl := s.cfg.Checks.Codenames[old]
		if repl == "" {
			continue
		}
		re := rules.WordRe(old)
		if !re.MatchString(content) {
			continue
		}
		content = rules.OutsideLinks(content, func(part string) string {
			return re.ReplaceAllString(part, "${1}"+strings.ReplaceAll(repl, "$", "$$")+"${2}")
		})
		changes = append(changes, batch.Change{Before: old, After: repl, Reason: i18n.T(s.lang, "codenameReason", old, repl), Rule: "codename"})
	}
	return content, changes
}

// Import reads new folders from the old review-queue/.
func (s *Service) Import() (int, error) {
	if s.cfg.ImportQueue == "" {
		return 0, nil
	}
	s.store.Lock()
	defer s.store.Unlock()
	known := map[string]bool{}
	for _, b := range s.store.All() {
		if b.ImportDir != "" {
			known[b.ImportDir] = true
		}
	}
	found, err := batch.LegacyBatches(s.cfg.ImportQueue, s.vault.Read, known)
	if err != nil {
		return 0, err
	}
	for _, b := range found {
		b.Summary = i18n.T(s.lang, "importSummary")
		if err := s.store.Put(b); err != nil {
			return 0, err
		}
		s.publish(Event{Kind: EventBatch, Batch: b.ID})
	}
	return len(found), nil
}

// Home returns the start page data.
func (s *Service) Home() HomeView {
	fv := s.Findings(false)
	h := HomeView{Findings: fv.Summary, Titles: fv.Titles, Notes: fv.Notes, Open: fv.Open, Checked: fv.Checked}
	h.Stats = s.stats.Summary(s.opts.Now(), fv.Conformity)
	if h.Stats.Spark == nil {
		h.Stats.Spark, h.Stats.SparkDays = []float64{}, []string{}
	}

	s.store.Lock()
	defer s.store.Unlock()
	for _, b := range s.store.All() {
		col := b.Column()
		if col != batch.ColumnReview && col != batch.ColumnFeedback {
			continue
		}
		h.Waiting++
		c := b.Counts()
		h.WaitingHunks += c.Hunks - c.Accepted - c.Rejected
		if h.Next == nil && col == batch.ColumnReview {
			sum := s.summary(b)
			h.Next = &sum
		}
	}
	return h
}

// copyBatch makes a snapshot for an AI run, so the run never races with your decisions.
func copyBatch(b *batch.Batch) *batch.Batch {
	c := *b
	c.Files = make([]*batch.File, len(b.Files))
	for i, f := range b.Files {
		fc := *f
		fc.Versions = append([]batch.Version(nil), f.Versions...)
		fc.Decisions = map[string]batch.Decision{}
		for k, v := range f.Decisions {
			fc.Decisions[k] = v
		}
		fc.Reasons = map[string]string{}
		for k, v := range f.Reasons {
			fc.Reasons[k] = v
		}
		c.Files[i] = &fc
	}
	c.Thread = append([]batch.Message(nil), b.Thread...)
	return &c
}

// shortTitle cuts a task to a provisional title until the AI names the batch.
func shortTitle(s string) string {
	s = strings.TrimSpace(strings.SplitN(s, "\n", 2)[0])
	if r := []rune(s); len(r) > 48 {
		return strings.TrimSpace(string(r[:48])) + "…"
	}
	return s
}

func wordSet(s string) map[string]bool {
	out := map[string]bool{}
	for _, w := range strings.FieldsFunc(strings.ToLower(s), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
		if len([]rune(w)) >= minWordLen {
			out[w] = true
		}
	}
	return out
}

func jaccard(a, b map[string]bool) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	inter := 0
	for w := range a {
		if b[w] {
			inter++
		}
	}
	return float64(inter) / float64(len(a)+len(b)-inter)
}
