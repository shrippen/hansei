// Package service is the single entry point for everything Hansei does. The TUI calls it
// in-process; the daemon exposes the same methods over JSON-RPC to the KDE app and scripts.
//
//	TUI ─┐                 ┌─ vault (read/write in scope) ─ journal (undo)
//	RPC ─┴─▶ Service ──────┼─ batch store (proposals, decisions, thread)
//	                       ├─ rules (checks without AI, rulebooks)
//	                       └─ agent ─ llm provider (Claude, OpenAI-compatible)
package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"git.arianw.de/shrippen/hansei/core/batch"
	"git.arianw.de/shrippen/hansei/core/config"
	"git.arianw.de/shrippen/hansei/core/i18n"
	"git.arianw.de/shrippen/hansei/core/journal"
	"git.arianw.de/shrippen/hansei/core/llm"
	"git.arianw.de/shrippen/hansei/core/rules"
	"git.arianw.de/shrippen/hansei/core/secrets"
	"git.arianw.de/shrippen/hansei/core/stats"
	"git.arianw.de/shrippen/hansei/core/vault"
)

const (
	lockName     = "hansei.lock"
	rescanAfter  = 30 * time.Second
	subscriberQ  = 256
	findingsKeep = 10 * time.Second
	scopeDepth   = 3
)

// ErrBusy means another Hansei process already owns the data folder (usually the daemon).
var ErrBusy = errors.New("service: another Hansei process is running")

// ProviderFactory builds a provider from its config; tests and the demo replace it.
type ProviderFactory func(p config.Provider) (llm.Provider, error)

// Options configure Open.
type Options struct {
	DataDir   string
	Lang      string
	Version   string
	Demo      bool
	Now       func() time.Time
	Providers ProviderFactory
}

// Service owns the vault, the batches and the AI runs.
type Service struct {
	cfgMu   sync.Mutex // guards swapping cfg in SetSettings against copies for AI runs
	cfg     *config.Config
	opts    Options
	lang    string
	vault   *vault.Vault
	store   *batch.Store
	journal *journal.Journal
	stats   *stats.Stats
	lock    *os.File

	provMu    sync.Mutex
	providers map[string]llm.Provider

	subMu sync.Mutex
	subs  map[int]chan Event
	subN  int

	runMu   sync.Mutex
	running map[string]context.CancelFunc
	wg      sync.WaitGroup

	repMu  sync.Mutex
	report *rules.Report
}

// Open starts the service for a config. It takes an exclusive lock on the data folder.
func Open(cfg *config.Config, opts Options) (*Service, error) {
	if opts.DataDir == "" {
		opts.DataDir = config.DataDir()
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.Providers == nil {
		opts.Providers = DefaultProvider
	}
	if err := os.MkdirAll(opts.DataDir, 0o700); err != nil {
		return nil, err
	}
	lock, err := takeLock(opts.DataDir)
	if err != nil {
		return nil, err
	}

	s := &Service{cfg: cfg, opts: opts, lang: i18n.Lang(firstNonEmpty(opts.Lang, cfg.Language)), lock: lock,
		providers: map[string]llm.Provider{}, subs: map[int]chan Event{}, running: map[string]context.CancelFunc{}}
	if err := s.load(); err != nil {
		lock.Close()
		return nil, err
	}
	return s, nil
}

func (s *Service) load() error {
	v, err := vault.Open(s.cfg)
	if err != nil {
		return err
	}
	if err := v.Scan(); err != nil {
		return err
	}
	s.vault = v
	if s.store, err = batch.OpenStore(s.opts.DataDir); err != nil {
		return err
	}
	if s.journal, err = journal.Open(s.opts.DataDir); err != nil {
		return err
	}
	if s.stats, err = stats.Open(s.opts.DataDir); err != nil {
		return err
	}
	s.recoverRuns()
	if s.cfg.ImportQueue != "" {
		_, _ = s.Import()
	}
	return nil
}

// recoverRuns marks runs that were cut off by a restart.
func (s *Service) recoverRuns() {
	s.store.Lock()
	defer s.store.Unlock()
	for _, b := range s.store.All() {
		if b.Status != batch.StatusWorking && !b.Revising {
			continue
		}
		if b.Status == batch.StatusWorking {
			b.Status = batch.StatusFailed
			b.Error = i18n.T(s.lang, "interrupted")
		}
		b.Revising, b.Progress = false, ""
		_ = s.store.Save(b)
	}
}

// takeLock makes sure only one process writes the data folder.
func takeLock(dir string) (*os.File, error) {
	f, err := os.OpenFile(filepath.Join(dir, lockName), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		return nil, ErrBusy
	}
	return f, nil
}

// Close cancels running AI tasks, waits for them and releases the lock.
func (s *Service) Close() {
	s.runMu.Lock()
	for _, cancel := range s.running {
		cancel()
	}
	s.runMu.Unlock()
	s.wg.Wait()
	s.subMu.Lock()
	for id, ch := range s.subs {
		close(ch)
		delete(s.subs, id)
	}
	s.subMu.Unlock()
	if s.lock != nil {
		s.lock.Close()
	}
}

// Lang returns the language of texts the core produces.
func (s *Service) Lang() string { return s.lang }

// Subscribe returns a channel of events and a function to stop.
func (s *Service) Subscribe() (<-chan Event, func()) {
	s.subMu.Lock()
	defer s.subMu.Unlock()
	s.subN++
	id := s.subN
	ch := make(chan Event, subscriberQ)
	s.subs[id] = ch
	return ch, func() {
		s.subMu.Lock()
		defer s.subMu.Unlock()
		if c, ok := s.subs[id]; ok {
			close(c)
			delete(s.subs, id)
		}
	}
}

// publish sends an event without blocking; slow subscribers miss events and refresh later.
func (s *Service) publish(e Event) {
	s.subMu.Lock()
	defer s.subMu.Unlock()
	for _, ch := range s.subs {
		select {
		case ch <- e:
		default:
		}
	}
}

func (s *Service) toast(text string) { s.publish(Event{Kind: EventToast, Text: text}) }

// rescan refreshes the vault index when it is older than rescanAfter.
func (s *Service) rescan(force bool) {
	if !force && s.opts.Now().Sub(s.vault.Scanned()) < rescanAfter {
		return
	}
	if err := s.vault.Scan(); err == nil {
		s.repMu.Lock()
		s.report = nil
		s.repMu.Unlock()
	}
}

// Status returns the header information.
func (s *Service) Status() StatusView {
	s.rescan(false)
	s.runMu.Lock()
	running := len(s.running)
	s.runMu.Unlock()
	return StatusView{
		Version: s.opts.Version, Vault: s.cfg.Vault, VaultName: filepath.Base(s.cfg.Vault), Allowed: s.cfg.Allow, Blocked: s.cfg.Block, Scopes: s.scopes(),
		Notes: len(s.vault.Notes()), Scanned: s.vault.Scanned(), Provider: s.cfg.Provider,
		Providers: s.providerViews(), Style: string(s.cfg.Style), Lang: s.lang, Demo: s.opts.Demo, Running: running,
	}
}

// scopes lists folders of allowed notes up to scopeDepth levels, e.g. IT, IT/Dienste, IT/Dienste/Regis.
func (s *Service) scopes() []string {
	seen := map[string]bool{}
	for _, f := range s.cfg.Allow {
		seen[f] = true
	}
	for _, n := range s.vault.Notes() {
		parts := strings.Split(path.Dir(n.Path), "/")
		for i := 1; i <= len(parts) && i <= scopeDepth; i++ {
			seen[strings.Join(parts[:i], "/")] = true
		}
	}
	out := make([]string, 0, len(seen))
	for f := range seen {
		if f != "." {
			out = append(out, f)
		}
	}
	sort.Strings(out)
	return out
}

func (s *Service) providerViews() []ProviderView {
	out := make([]ProviderView, 0, len(s.cfg.Providers))
	for _, p := range s.cfg.Providers {
		local := p.Local || (p.Kind == config.KindOpenAI && llm.LocalURL(p.BaseURL))
		hasKey := p.Kind != config.KindAnthropic || secrets.Has(p.Name) || os.Getenv("ANTHROPIC_API_KEY") != ""
		if s.opts.Demo {
			hasKey = p.Kind == config.KindDemo // the demo never looks at real keys
		}
		out = append(out, ProviderView{
			Name: p.Name, Kind: string(p.Kind), Model: p.Model, BaseURL: p.BaseURL, Fallback: p.Fallback, Local: local,
			HasKey:  hasKey,
			Default: p.Name == s.cfg.Provider, PriceIn: p.PriceIn, PriceOut: p.PriceOut, Currency: p.Currency,
		})
	}
	return out
}

// provider returns (and caches) the provider with that name.
func (s *Service) provider(name string) (llm.Provider, error) {
	if name == "" {
		name = s.cfg.Provider
	}
	s.provMu.Lock()
	defer s.provMu.Unlock()
	if p, ok := s.providers[name]; ok {
		return p, nil
	}
	pc, ok := s.cfg.ProviderByName(name)
	if !ok {
		return nil, fmt.Errorf("provider %q is not configured", name)
	}
	p, err := s.opts.Providers(pc)
	if err != nil {
		return nil, err
	}
	s.providers[name] = p
	return p, nil
}

// DefaultProvider builds the real providers.
func DefaultProvider(p config.Provider) (llm.Provider, error) {
	key, err := secrets.Get(p.Name)
	if err != nil && !errors.Is(err, secrets.ErrNone) {
		key = "" // keyring unavailable (e.g. no session bus): fall back to the SDK's own lookup
	}
	switch p.Kind {
	case config.KindAnthropic:
		return llm.NewClaude(p, key), nil
	case config.KindOpenAI:
		return llm.NewOpenAI(p, key), nil
	}
	return nil, fmt.Errorf("provider kind %q is not available in this build", p.Kind)
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
