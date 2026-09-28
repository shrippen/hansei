package service

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"git.arianw.de/shrippen/hansei/core/batch"
	"git.arianw.de/shrippen/hansei/core/config"
	"git.arianw.de/shrippen/hansei/core/i18n"
	"git.arianw.de/shrippen/hansei/core/llm"
	"git.arianw.de/shrippen/hansei/core/redact"
	"git.arianw.de/shrippen/hansei/core/secrets"
)

const (
	folderDepth          = 3
	providerCheckTimeout = 90 * time.Second
)

// Settings returns the editable settings with the top-level folders of the vault.
func (s *Service) Settings() SettingsView {
	sv := SettingsView{Vault: s.cfg.Vault, Allow: s.cfg.Allow, Block: s.cfg.Block, LocalOnly: s.cfg.LocalOnly,
		Provider: s.cfg.Provider, Providers: s.providerViews(), Style: string(s.cfg.Style),
		MaxBatchFiles: s.cfg.MaxBatchFiles, ReviewDays: s.cfg.Checks.ReviewDays, ReviewField: s.cfg.Checks.ReviewField,
		Codenames: s.cfg.Checks.Codenames, Required: s.cfg.Checks.Required, Rulebooks: s.cfg.Rulebooks,
		DefaultRules: s.cfg.DefaultRules, Path: s.cfg.Path()}
	if sv.Codenames == nil {
		sv.Codenames = map[string]string{}
	}
	if sv.Required == nil {
		sv.Required = map[string][]string{}
	}
	if sv.Rulebooks == nil {
		sv.Rulebooks = []config.Rulebook{}
	}
	sv.Folders = s.folderTree("", 0)
	return sv
}

// folderTree lists folders up to folderDepth levels. Blocked folders appear, but nothing
// below them is read, not even their sub folder names.
func (s *Service) folderTree(parent string, depth int) []FolderView {
	if depth >= folderDepth {
		return nil
	}
	entries, err := os.ReadDir(filepath.Join(s.cfg.Vault, filepath.FromSlash(parent)))
	if err != nil {
		return nil
	}
	var out []FolderView
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		rel := strings.TrimPrefix(parent+"/"+e.Name(), "/")
		probe := rel + "/x.md"
		fv := FolderView{Name: e.Name(), Path: rel, Depth: depth, Blocked: s.vault.Blocked(probe), LocalOnly: s.vault.LocalOnly(probe)}
		fv.Allowed = !fv.Blocked && s.vault.Allowed(probe)
		if fv.Allowed {
			fv.Rulebook = s.cfg.RulebookFiles(rel)
		}
		out = append(out, fv)
		if !fv.Blocked {
			out = append(out, s.folderTree(rel, depth+1)...)
		}
	}
	return out
}

// SetSettings changes and saves settings. Blocking always wins over allowing.
func (s *Service) SetSettings(p SettingsPatch) (SettingsView, error) {
	next := *s.cfg
	if p.Allow != nil {
		next.Allow = *p.Allow
	}
	if p.Block != nil {
		next.Block = *p.Block
	}
	if p.LocalOnly != nil {
		next.LocalOnly = *p.LocalOnly
	}
	if p.Style != nil {
		next.Style = config.Style(*p.Style)
	}
	if p.Codenames != nil {
		next.Checks.Codenames = *p.Codenames
	}
	if p.Required != nil {
		next.Checks.Required = *p.Required
	}
	if p.ReviewDays != nil {
		next.Checks.ReviewDays = *p.ReviewDays
	}
	if p.Rulebooks != nil {
		next.Rulebooks = *p.Rulebooks
	}
	if p.Provider != nil {
		next.Provider = *p.Provider
	}
	if p.Providers != nil {
		next.Providers = nil
		for _, pv := range *p.Providers {
			next.Providers = append(next.Providers, config.Provider{Name: strings.TrimSpace(pv.Name), Kind: config.ProviderKind(pv.Kind),
				Model: pv.Model, BaseURL: pv.BaseURL, Fallback: pv.Fallback, Local: pv.Local, PriceIn: pv.PriceIn, PriceOut: pv.PriceOut, Currency: pv.Currency})
		}
	}
	reload, _ := config.Load(s.cfg.Path()) // keep the file's path and fields the patch does not cover
	if reload == nil {
		reload = s.cfg
	}
	reload.Allow, reload.Block, reload.LocalOnly = next.Allow, next.Block, next.LocalOnly
	reload.Style, reload.Provider, reload.Providers = next.Style, next.Provider, next.Providers
	reload.Checks.Codenames, reload.Checks.Required, reload.Checks.ReviewDays = next.Checks.Codenames, next.Checks.Required, next.Checks.ReviewDays
	reload.Rulebooks = next.Rulebooks
	if err := reload.Validate(); err != nil {
		return SettingsView{}, err
	}
	for _, pv := range reload.Providers {
		if pv.Name == "" || pv.Model == "" {
			return SettingsView{}, fmt.Errorf("provider needs a name and a model")
		}
	}
	if err := reload.Save(); err != nil {
		return SettingsView{}, err
	}

	// Apply in place: normalise lists through a reload, rebuild the vault scope and providers.
	fresh, err := config.Load(reload.Path())
	if err != nil {
		return SettingsView{}, err
	}
	s.cfgMu.Lock()
	*s.cfg = *fresh
	s.cfgMu.Unlock()
	s.provMu.Lock()
	s.providers = map[string]llm.Provider{}
	s.provMu.Unlock()
	s.vault.SetScope(s.cfg)
	s.rescan(true)
	s.repMu.Lock()
	s.report = nil // checks may have changed
	s.repMu.Unlock()
	s.publish(Event{Kind: EventSettings})
	return s.Settings(), nil
}

// CheckProvider sends a tiny request to a provider and lists its models, so a wrong key or a
// stopped Ollama shows up in the settings rather than on the first task.
func (s *Service) CheckProvider(name string) ProviderCheck {
	prov, err := s.provider(name)
	if err != nil {
		return ProviderCheck{Message: err.Error(), Models: []string{}}
	}
	ctx, cancel := context.WithTimeout(context.Background(), providerCheckTimeout)
	defer cancel()
	out := ProviderCheck{Models: []string{}}
	if lister, ok := prov.(llm.Lister); ok {
		if models, err := lister.Models(ctx); err == nil {
			out.Models = models
		}
	}
	res, err := prov.Run(ctx, llm.Request{System: "Answer with the single word OK.", Prompt: "OK?", MaxTurns: 1})
	if err != nil {
		out.Message = err.Error()
		return out
	}
	out.OK = true
	out.Message = strings.TrimSpace(res.Text)
	return out
}

// RuleSection finds a section of a rulebook note, e.g. "Design.md › Secrets" or "IT/Design.md".
func (s *Service) RuleSection(ref string) (RuleSectionView, error) {
	file, section, _ := strings.Cut(ref, "›")
	file, section = strings.TrimSpace(file), strings.TrimSpace(section)
	for _, rf := range s.cfg.RuleFiles() {
		if rf != file && path.Base(rf) != file && strings.TrimSuffix(path.Base(rf), ".md") != file {
			continue
		}
		text, err := s.vault.ReadRule(rf)
		if err != nil {
			return RuleSectionView{}, err
		}
		heading, body := sectionOf(text, section)
		return RuleSectionView{File: rf, Heading: heading, Text: redact.Mask(body)}, nil
	}
	return RuleSectionView{}, fmt.Errorf("no rulebook note matches %q", file)
}

// sectionOf returns the heading and text of the first section whose heading contains name
// (the whole note when name is empty or not found).
func sectionOf(text, name string) (string, string) {
	lines := strings.Split(text, "\n")
	if name == "" {
		return "", text
	}
	want := strings.ToLower(name)
	for i, l := range lines {
		t := strings.TrimSpace(l)
		level := len(t) - len(strings.TrimLeft(t, "#"))
		if level == 0 || !strings.Contains(strings.ToLower(t), want) {
			continue
		}
		end := len(lines)
		for j := i + 1; j < len(lines); j++ {
			u := strings.TrimSpace(lines[j])
			l2 := len(u) - len(strings.TrimLeft(u, "#"))
			if l2 > 0 && l2 <= level && strings.HasPrefix(u[l2:], " ") {
				end = j
				break
			}
		}
		return strings.TrimSpace(strings.TrimLeft(t, "# ")), strings.TrimSpace(strings.Join(lines[i+1:end], "\n"))
	}
	return "", text
}

// SetKey stores an API key for a provider in the keyring.
func (s *Service) SetKey(provider, key string) error {
	if _, ok := s.cfg.ProviderByName(provider); !ok {
		return fmt.Errorf("provider %q is not configured", provider)
	}
	if err := secrets.Set(provider, strings.TrimSpace(key)); err != nil {
		return err
	}
	s.provMu.Lock()
	delete(s.providers, provider)
	s.provMu.Unlock()
	s.publish(Event{Kind: EventSettings})
	return nil
}

// writeStatusNote updates the optional status note that andon reads (via the vault sync).
func (s *Service) writeStatusNote() {
	if s.cfg.StatusNote == "" || !s.vault.Allowed(s.cfg.StatusNote) {
		return
	}
	s.store.Lock()
	var review, feedback, done int
	for _, b := range s.store.All() {
		switch b.Column() {
		case batch.ColumnReview:
			review++
		case batch.ColumnFeedback:
			feedback++
		case batch.ColumnDone:
			done++
		}
	}
	s.store.Unlock()
	rep := s.Findings(false)
	var b strings.Builder
	fmt.Fprintf(&b, "---\nhansei_review: %d\nhansei_feedback: %d\nhansei_done: %d\nhansei_conformity: %.2f\nhansei_updated: %s\n---\n# %s\n",
		review, feedback, done, rep.Conformity, s.opts.Now().Format("2006-01-02T15:04:05Z07:00"), i18n.T(s.lang, "statusNoteTitle"))
	_ = s.vault.Write(s.cfg.StatusNote, b.String())
}
