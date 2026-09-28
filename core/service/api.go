package service

import "git.arianw.de/shrippen/hansei/core/batch"

// API is what every front end uses. *Service implements it in-process; rpc.Client
// implements it over the daemon's socket. Keep both in sync with rpc.Methods.
type API interface {
	Status() StatusView
	Home() HomeView
	Findings(refresh bool) FindingsView
	Batches() []BatchSummary
	Batch(id string) (BatchView, error)
	File(id, path, mode string, context int, reveal bool) (FileView, error)
	Compare(id, path string, from, to int, mode string, context int) (CompareView, error)
	Decide(id, path, hunk, decision, reason string) (DecideResult, error)
	DecideFile(id, path, decision string) (DecideResult, error)
	EditHunk(id, path, hunk, text string) (FileSummary, error)
	EditFile(id, path, content string) (FileSummary, error)
	SetVersion(id, path string, n int) (FileSummary, error)
	Reopen(id, path string) (FileSummary, error)
	UndoFile(id, path string) (FileSummary, error)
	UndoBatch(id string) (BatchSummary, error)
	Journal(limit int) []JournalView
	Discard(id string) error
	Cancel(id string)
	Task(in TaskInput) (BatchSummary, error)
	Estimate(in TaskInput) (Estimate, error)
	Feedback(in FeedbackInput) (batch.Message, error)
	Regenerate(id, path, hunk string) (batch.Message, error)
	Suggestion(id, sid string, accept bool) (BatchSummary, error)
	FromFinding(rule, provider string) (BatchSummary, error)
	Import() (int, error)
	Settings() SettingsView
	SetSettings(p SettingsPatch) (SettingsView, error)
	SetKey(provider, key string) error
	CheckProvider(name string) ProviderCheck
	RuleSection(ref string) (RuleSectionView, error)
	JournalDiff(id string) (CompareView, error)
	Subscribe() (<-chan Event, func())
}

var _ API = (*Service)(nil)
