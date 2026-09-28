package service

import (
	"errors"
	"fmt"
	"strings"

	"git.arianw.de/shrippen/hansei/core/batch"
	"git.arianw.de/shrippen/hansei/core/diff"
	"git.arianw.de/shrippen/hansei/core/i18n"
	"git.arianw.de/shrippen/hansei/core/journal"
	"git.arianw.de/shrippen/hansei/core/redact"
	"git.arianw.de/shrippen/hansei/core/vault"
)

const defaultContext = 3

var (
	// ErrNoFile means the path is not part of the batch.
	ErrNoFile = errors.New("service: file is not part of this batch")
	// ErrNoHunk means the hunk does not exist in the current version.
	ErrNoHunk = errors.New("service: change not found in the current version")
	// ErrRunning means the AI is still working on the batch.
	ErrRunning = errors.New("service: the AI is still working on this batch")
)

// Batches lists all batches, newest first.
func (s *Service) Batches() []BatchSummary {
	s.store.Lock()
	defer s.store.Unlock()
	all := s.store.All()
	out := make([]BatchSummary, 0, len(all))
	for _, b := range all {
		out = append(out, s.summary(b))
	}
	return out
}

// Batch returns one batch with its thread.
func (s *Service) Batch(id string) (BatchView, error) {
	s.store.Lock()
	defer s.store.Unlock()
	b, err := s.store.Get(id)
	if err != nil {
		return BatchView{}, err
	}
	thread := make([]batch.Message, len(b.Thread))
	suggestions := append([]batch.Suggestion{}, b.Suggestions...)
	for i, m := range b.Thread {
		m.Text = redact.Mask(m.Text)
		thread[i] = m
	}
	return BatchView{BatchSummary: s.summary(b), Instruction: redact.Mask(b.Instruction), Summary: b.Summary,
		Scope: append([]string{}, b.Scope...), Thread: thread, Suggestions: suggestions, Found: append([]batch.Found{}, b.Found...)}, nil
}

func (s *Service) summary(b *batch.Batch) BatchSummary {
	s.runMu.Lock()
	_, running := s.running[b.ID]
	s.runMu.Unlock()
	sum := BatchSummary{
		ID: b.ID, Title: b.Title, Topic: b.Topic, Status: string(b.Status), Column: string(b.Column()), Source: string(b.Source),
		Provider: b.Provider, Progress: b.Progress, Error: b.Error, Counts: b.Counts(), Questions: b.OpenQuestions(), Found: len(b.Found),
		Revising: b.Revising, Running: running, Created: b.Created, Updated: b.Updated, Usage: b.Usage, Rule: b.Rule,
		Files: []FileSummary{},
	}
	for _, f := range b.Files {
		sum.Files = append(sum.Files, s.fileSummary(f))
	}
	return sum
}

func (s *Service) fileSummary(f *batch.File) FileSummary {
	d := f.Diff()
	fs := FileSummary{Path: f.Path, Status: string(f.Status), Hunks: len(d.Hunks), Versions: len(f.Versions), New: !f.Exists}
	for _, h := range d.Hunks {
		switch f.Decision(h.ID) {
		case batch.Pending:
			fs.Open++
		case batch.Accepted:
			fs.Accepted++
		}
	}
	if len(f.Versions) > 1 {
		fs.Feedback = true
	}
	if f.Status == batch.FileOpen && s.stale(f) {
		fs.Status = "stale"
	}
	return fs
}

// stale reports whether the vault note changed since the proposal was made.
func (s *Service) stale(f *batch.File) bool {
	current, exists, err := s.vault.Read(f.Path)
	if err != nil {
		return true
	}
	if !exists {
		return f.Exists
	}
	return vault.Hash(current) != f.BaseHash
}

// File returns the review view of one file. mode is "split" or "unified"; context is the
// number of unchanged lines around each change (-1: the whole note).
func (s *Service) File(id, path, mode string, context int, reveal bool) (FileView, error) {
	s.store.Lock()
	defer s.store.Unlock()
	b, f, err := s.file(id, path)
	if err != nil {
		return FileView{}, err
	}
	return s.fileView(b, f, mode, context, reveal), nil
}

func (s *Service) fileView(b *batch.Batch, f *batch.File, mode string, context int, reveal bool) FileView {
	m := diff.ModeSplit
	if mode == string(diff.ModeUnified) {
		m = diff.ModeUnified
	}
	if context == 0 {
		context = defaultContext
	}
	latest := f.Latest()
	res := f.Diff()

	fv := FileView{Batch: b.ID, Path: f.Path, Status: string(f.Status), Exists: f.Exists, Summary: f.Summary,
		Mode: string(m), Rows: res.Rows(m, context), Current: latest.N, Content: latest.Content, Base: f.Base, Revealed: reveal,
		Hunks: []HunkView{}, Versions: []VersionView{}}
	if fv.Rows == nil {
		fv.Rows = []diff.Row{}
	}
	if f.Status == batch.FileOpen {
		fv.Stale = s.stale(f)
	}
	if !reveal {
		fv.Rows = maskRows(fv.Rows, f.Base, latest.Content)
		fv.Content = redact.Mask(latest.Content)
		fv.Base = redact.Mask(f.Base)
	}

	feedback := map[string]int{}
	for _, msg := range b.Thread {
		if msg.Scope == batch.ScopeHunk && msg.Path == f.Path {
			feedback[msg.Hunk]++
		}
	}
	for _, h := range res.Hunks {
		reason, rule := f.Reason(h)
		fv.Hunks = append(fv.Hunks, HunkView{
			ID: h.ID, Index: h.Index, Heading: h.Heading, State: string(f.Decision(h.ID)), Reason: reason, Rule: rule,
			Rejected: f.Reasons[h.ID], OldStart: h.OldStart + 1, OldCount: len(h.OldLines), NewStart: h.NewStart + 1,
			NewCount: len(h.NewLines), Feedback: feedback[h.ID],
		})
	}
	for _, v := range f.Versions {
		fv.Versions = append(fv.Versions, VersionView{N: v.N, Author: string(v.Author), Note: v.Note, Created: v.Created})
	}
	return fv
}

// maskRows hides secret values in the rows. Lines are masked in the context of the whole
// text, so multi-line secrets (private keys) are caught too.
func maskRows(rows []diff.Row, oldText, newText string) []diff.Row {
	oldMasked := strings.Split(redact.Mask(oldText), "\n")
	newMasked := strings.Split(redact.Mask(newText), "\n")
	pick := func(lines []string, no int, segs []diff.Seg) (string, bool) {
		if no <= 0 || no > len(lines) {
			return "", false
		}
		orig := joinSegs(segs)
		if lines[no-1] == orig {
			return orig, false
		}
		return lines[no-1], true
	}

	out := make([]diff.Row, len(rows))
	for i, r := range rows {
		o, oChanged := pick(oldMasked, r.OldNo, r.Old)
		n, nChanged := pick(newMasked, r.NewNo, r.New)
		switch {
		case !oChanged && !nChanged:
		case r.Kind == diff.RowChange:
			r.Old, r.New = diff.Words(o, n)
		default:
			if oChanged {
				r.Old = []diff.Seg{{Text: o, Changed: r.Kind == diff.RowDelete}}
			}
			if nChanged {
				r.New = []diff.Seg{{Text: n, Changed: r.Kind == diff.RowInsert}}
			}
		}
		out[i] = r
	}
	return out
}

func joinSegs(segs []diff.Seg) string {
	var b strings.Builder
	for _, s := range segs {
		b.WriteString(s.Text)
	}
	return b.String()
}

// Compare shows how a file changed between two versions (0 = the vault note).
func (s *Service) Compare(id, path string, from, to int, mode string, context int) (CompareView, error) {
	s.store.Lock()
	defer s.store.Unlock()
	_, f, err := s.file(id, path)
	if err != nil {
		return CompareView{}, err
	}
	text := func(n int) (string, error) {
		if n == 0 {
			return f.Base, nil
		}
		if n < 1 || n > len(f.Versions) {
			return "", fmt.Errorf("version %d does not exist", n)
		}
		return f.Versions[n-1].Content, nil
	}
	a, err := text(from)
	if err != nil {
		return CompareView{}, err
	}
	b, err := text(to)
	if err != nil {
		return CompareView{}, err
	}
	m := diff.ModeSplit
	if mode == string(diff.ModeUnified) {
		m = diff.ModeUnified
	}
	if context == 0 {
		context = defaultContext
	}
	rows := maskRows(diff.Compare(a, b).Rows(m, context), a, b)
	if rows == nil {
		rows = []diff.Row{}
	}
	return CompareView{From: from, To: to, Rows: rows}, nil
}

// Decide accepts, rejects or reopens one hunk. When the last hunk of a file is decided,
// the file is written to the vault (if anything was accepted) and journaled.
func (s *Service) Decide(id, path, hunk, decision, reason string) (DecideResult, error) {
	s.store.Lock()
	defer s.store.Unlock()
	b, f, err := s.file(id, path)
	if err != nil {
		return DecideResult{}, err
	}
	if err := s.editable(b, f); err != nil {
		return DecideResult{}, err
	}
	found := false
	for _, h := range f.Diff().Hunks {
		if h.ID == hunk {
			found = true
		}
	}
	if !found {
		return DecideResult{}, ErrNoHunk
	}

	d := batch.Decision(decision)
	before := f.Decision(hunk)
	f.Decide(hunk, d)
	if f.Reasons == nil {
		f.Reasons = map[string]string{}
	}
	if d == batch.Rejected && reason != "" {
		f.Reasons[hunk] = reason
	} else {
		delete(f.Reasons, hunk)
	}
	if before == batch.Pending && d != batch.Pending {
		_ = s.stats.Reviewed(s.opts.Now(), 1)
	}
	return s.afterDecision(b, f)
}

// DecideFile accepts or rejects all open hunks of a file at once.
func (s *Service) DecideFile(id, path, decision string) (DecideResult, error) {
	s.store.Lock()
	defer s.store.Unlock()
	b, f, err := s.file(id, path)
	if err != nil {
		return DecideResult{}, err
	}
	if err := s.editable(b, f); err != nil {
		return DecideResult{}, err
	}
	n := 0
	for _, h := range f.Diff().Hunks {
		if f.Decision(h.ID) != batch.Pending {
			continue
		}
		f.Decide(h.ID, batch.Decision(decision))
		n++
	}
	_ = s.stats.Reviewed(s.opts.Now(), n)
	return s.afterDecision(b, f)
}

func (s *Service) editable(b *batch.Batch, f *batch.File) error {
	s.runMu.Lock()
	_, running := s.running[b.ID]
	s.runMu.Unlock()
	if running {
		return ErrRunning
	}
	if f.Status != batch.FileOpen {
		return errors.New(i18n.T(s.lang, "notOpen"))
	}
	return nil
}

// afterDecision writes a fully decided file and closes the batch when nothing is open.
func (s *Service) afterDecision(b *batch.Batch, f *batch.File) (DecideResult, error) {
	var res DecideResult
	b.Updated = s.opts.Now()

	if f.Open() == 0 {
		written, err := s.apply(b, f)
		if err != nil {
			_ = s.store.Save(b)
			return res, err
		}
		res.Written = written
		res.Skipped = !written
		if written {
			res.Message = i18n.T(s.lang, "written", f.Path)
		} else {
			res.Message = i18n.T(s.lang, "skipped", f.Path)
		}
	}

	if s.finished(b) {
		b.Status = batch.StatusDone
		res.Done = true
	}
	if err := s.store.Save(b); err != nil {
		return res, err
	}
	res.File = s.fileSummary(f)
	res.Batch = s.summary(b)
	s.publish(Event{Kind: EventBatch, Batch: b.ID})
	// One toast per decision; a written file offers undo.
	undo := ""
	if res.Written {
		undo = f.Path
	}
	switch {
	case res.Done:
		s.publish(Event{Kind: EventToast, Batch: b.ID, Path: undo,
			Text: strings.TrimPrefix(res.Message+" · ", " · ") + i18n.T(s.lang, "batchDone", b.Title)})
		s.writeStatusNote()
	case res.Message != "":
		s.publish(Event{Kind: EventToast, Batch: b.ID, Path: undo, Text: res.Message})
	}
	return res, nil
}

func (s *Service) finished(b *batch.Batch) bool {
	if b.Status != batch.StatusReview || b.Revising || len(b.Files) == 0 {
		return false
	}
	for _, f := range b.Files {
		if f.Status == batch.FileOpen {
			return false
		}
	}
	return true
}

// apply writes the accepted hunks of a decided file. Nothing accepted: the file is skipped.
func (s *Service) apply(b *batch.Batch, f *batch.File) (bool, error) {
	res := f.Diff()
	accepted := func(h diff.Hunk) bool { return f.Decision(h.ID) == batch.Accepted }
	any := false
	for _, h := range res.Hunks {
		if accepted(h) {
			any = true
		}
	}
	if !any {
		f.Status = batch.FileSkipped
		return false, nil
	}

	current, exists, err := s.vault.Read(f.Path)
	if err != nil {
		return false, err
	}
	var out string
	if exists == f.Exists && vault.Hash(current) == f.BaseHash {
		out = res.Apply(accepted)
	} else {
		// The note changed behind the proposal: place the hunks in the new content if they still fit.
		out, err = res.Rebase(current, accepted)
		if err != nil {
			return false, errors.New(i18n.T(s.lang, "stale", f.Path))
		}
	}

	if err := s.vault.Write(f.Path, out); err != nil {
		return false, err
	}
	jid, err := s.journal.Add(journal.Entry{Batch: b.ID, Title: b.Title, Path: f.Path, Existed: exists, Before: current, After: out, AfterHash: vault.Hash(out)})
	if err != nil {
		return true, err
	}
	f.Status, f.Applied, f.Journal = batch.FileApplied, s.opts.Now(), jid
	s.repMu.Lock()
	s.report = nil
	s.repMu.Unlock()
	return true, nil
}

// EditHunk replaces the new side of one hunk with your own text; this creates a version.
func (s *Service) EditHunk(id, path, hunk, text string) (FileSummary, error) {
	s.store.Lock()
	defer s.store.Unlock()
	b, f, err := s.file(id, path)
	if err != nil {
		return FileSummary{}, err
	}
	if err := s.editable(b, f); err != nil {
		return FileSummary{}, err
	}
	res := f.Diff()
	var target *diff.Hunk
	for i := range res.Hunks {
		if res.Hunks[i].ID == hunk {
			target = &res.Hunks[i]
		}
	}
	if target == nil {
		return FileSummary{}, ErrNoHunk
	}

	// Rebuild the proposal: every other hunk as proposed, this one with your text.
	replaced := strings.Split(strings.TrimSuffix(text, "\n"), "\n")
	if text == "" {
		replaced = nil
	}
	content := res.ApplyWith(func(h diff.Hunk) []string {
		if h.ID == hunk {
			return replaced
		}
		return h.NewLines
	})
	return s.addVersion(b, f, content, batch.AuthorUser)
}

// EditFile replaces the whole proposal with your own content (the TUI's $EDITOR).
func (s *Service) EditFile(id, path, content string) (FileSummary, error) {
	s.store.Lock()
	defer s.store.Unlock()
	b, f, err := s.file(id, path)
	if err != nil {
		return FileSummary{}, err
	}
	if err := s.editable(b, f); err != nil {
		return FileSummary{}, err
	}
	return s.addVersion(b, f, content, batch.AuthorUser)
}

func (s *Service) addVersion(b *batch.Batch, f *batch.File, content string, author batch.Author) (FileSummary, error) {
	if content == f.Latest().Content {
		return s.fileSummary(f), nil
	}
	f.AddVersion(batch.Version{Content: content, Author: author, Created: s.opts.Now()})
	// Your own edit counts as accepted for the hunks it produces.
	if author == batch.AuthorUser {
		for _, h := range f.Diff().Hunks {
			if f.Decision(h.ID) == batch.Pending {
				f.Decide(h.ID, batch.Accepted)
			}
		}
	}
	b.Updated = s.opts.Now()
	if err := s.store.Save(b); err != nil {
		return FileSummary{}, err
	}
	s.publish(Event{Kind: EventBatch, Batch: b.ID})
	return s.fileSummary(f), nil
}

// SetVersion makes an earlier version current again ("Zurück zu v2").
func (s *Service) SetVersion(id, path string, n int) (FileSummary, error) {
	s.store.Lock()
	defer s.store.Unlock()
	b, f, err := s.file(id, path)
	if err != nil {
		return FileSummary{}, err
	}
	if err := s.editable(b, f); err != nil {
		return FileSummary{}, err
	}
	if n < 1 || n > len(f.Versions) {
		return FileSummary{}, fmt.Errorf("version %d does not exist", n)
	}
	f.Current = n - 1
	b.Updated = s.opts.Now()
	if err := s.store.Save(b); err != nil {
		return FileSummary{}, err
	}
	s.publish(Event{Kind: EventBatch, Batch: b.ID})
	return s.fileSummary(f), nil
}

// Reopen puts a decided file back into review: decisions stay, nothing is written yet.
// It is used after an undo or to change a skipped file.
func (s *Service) Reopen(id, path string) (FileSummary, error) {
	s.store.Lock()
	defer s.store.Unlock()
	b, f, err := s.file(id, path)
	if err != nil {
		return FileSummary{}, err
	}
	if f.Status == batch.FileApplied {
		return FileSummary{}, errors.New(i18n.T(s.lang, "notOpen"))
	}
	current, exists, err := s.vault.Read(f.Path)
	if err != nil {
		return FileSummary{}, err
	}
	f.Base, f.BaseHash, f.Exists, f.Status = current, vault.Hash(current), exists, batch.FileOpen
	f.Decisions = map[string]batch.Decision{}
	b.Status = batch.StatusReview
	b.Updated = s.opts.Now()
	if err := s.store.Save(b); err != nil {
		return FileSummary{}, err
	}
	s.publish(Event{Kind: EventBatch, Batch: b.ID})
	return s.fileSummary(f), nil
}

// UndoFile restores the note as it was before the batch wrote it.
func (s *Service) UndoFile(id, path string) (FileSummary, error) {
	s.store.Lock()
	defer s.store.Unlock()
	b, f, err := s.file(id, path)
	if err != nil {
		return FileSummary{}, err
	}
	if err := s.undo(b, f); err != nil {
		return FileSummary{}, err
	}
	b.Status = batch.StatusReview
	b.Updated = s.opts.Now()
	if err := s.store.Save(b); err != nil {
		return FileSummary{}, err
	}
	s.publish(Event{Kind: EventBatch, Batch: b.ID})
	s.toast(i18n.T(s.lang, "undone", f.Path))
	return s.fileSummary(f), nil
}

// UndoBatch restores every note the batch wrote.
func (s *Service) UndoBatch(id string) (BatchSummary, error) {
	s.store.Lock()
	defer s.store.Unlock()
	b, err := s.store.Get(id)
	if err != nil {
		return BatchSummary{}, err
	}
	var errs []error
	for _, f := range b.Files {
		if f.Status != batch.FileApplied {
			continue
		}
		if err := s.undo(b, f); err != nil {
			errs = append(errs, err)
		}
	}
	b.Status = batch.StatusReview
	b.Updated = s.opts.Now()
	if err := s.store.Save(b); err != nil {
		return BatchSummary{}, err
	}
	s.publish(Event{Kind: EventBatch, Batch: b.ID})
	s.toast(i18n.T(s.lang, "undone", b.Title))
	return s.summary(b), errors.Join(errs...)
}

func (s *Service) undo(b *batch.Batch, f *batch.File) error {
	if f.Status != batch.FileApplied || f.Journal == "" {
		return fmt.Errorf("%s was not written by this batch", f.Path)
	}
	e, err := s.journal.Get(f.Journal)
	if err != nil {
		return err
	}
	current, _, err := s.vault.Read(f.Path)
	if err != nil {
		return err
	}
	if vault.Hash(current) != e.AfterHash {
		return errors.New(i18n.T(s.lang, "changedSince", f.Path))
	}
	if e.Existed {
		err = s.vault.Write(f.Path, e.Before)
	} else {
		err = s.vault.Remove(f.Path)
	}
	if err != nil {
		return err
	}
	if err := s.journal.MarkUndone(e.ID); err != nil {
		return err
	}
	f.Status = batch.FileUndone
	s.repMu.Lock()
	s.report = nil
	s.repMu.Unlock()
	return nil
}

// Journal lists recent writes.
func (s *Service) Journal(limit int) []JournalView {
	out := []JournalView{}
	for _, e := range s.journal.List(limit) {
		r := diff.Compare(e.Before, e.After)
		added, removed := 0, 0
		for _, h := range r.Hunks {
			added += len(h.NewLines)
			removed += len(h.OldLines)
		}
		out = append(out, JournalView{ID: e.ID, Batch: e.Batch, Title: e.Title, Path: e.Path, Time: e.Time,
			Undone: e.Undone, New: !e.Existed, Added: added, Removed: removed})
	}
	return out
}

// JournalDiff shows what one write changed (secrets masked).
func (s *Service) JournalDiff(id string) (CompareView, error) {
	e, err := s.journal.Get(id)
	if err != nil {
		return CompareView{}, err
	}
	rows := maskRows(diff.Compare(e.Before, e.After).Rows(diff.ModeSplit, defaultContext), e.Before, e.After)
	if rows == nil {
		rows = []diff.Row{}
	}
	return CompareView{Rows: rows}, nil
}

// Discard throws a batch away (nothing already written is touched).
func (s *Service) Discard(id string) error {
	s.cancel(id)
	s.store.Lock()
	defer s.store.Unlock()
	b, err := s.store.Get(id)
	if err != nil {
		return err
	}
	b.Status = batch.StatusDiscarded
	b.Updated = s.opts.Now()
	if err := s.store.Save(b); err != nil {
		return err
	}
	s.publish(Event{Kind: EventBatch, Batch: b.ID})
	return nil
}

// file finds a batch and one of its files; the store lock must be held.
func (s *Service) file(id, path string) (*batch.Batch, *batch.File, error) {
	b, err := s.store.Get(id)
	if err != nil {
		return nil, nil, err
	}
	f, ok := b.File(path)
	if !ok {
		return nil, nil, ErrNoFile
	}
	return b, f, nil
}
