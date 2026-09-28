// Package config loads and saves Hansei's settings (config.yaml).
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Style names the three visual styles; System (the platform theme) is the default.
type Style string

const (
	StyleSystem     Style = "system"
	StyleKanteLight Style = "kante-light"
	StyleKante      Style = "kante"
)

// ProviderKind selects the client implementation for a provider.
type ProviderKind string

const (
	KindAnthropic ProviderKind = "anthropic"
	KindOpenAI    ProviderKind = "openai" // any OpenAI-compatible endpoint (Ollama, LM Studio, …)
	KindDemo      ProviderKind = "demo"   // only available in demo builds
)

const (
	defaultMaxBatchFiles = 15
	defaultReviewDays    = 180
	defaultMaxTurns      = 40
	configEnv            = "HANSEI_CONFIG"
	dataEnv              = "HANSEI_DATA"
	fileMode             = 0o600
	opus5In              = 5.0  // $ per million input tokens
	opus5Out             = 25.0 // $ per million output tokens
	usd                  = "$"
	dirMode              = 0o700
)

// Provider is one configured AI backend.
type Provider struct {
	Name     string       `yaml:"name" json:"name"`
	Kind     ProviderKind `yaml:"kind" json:"kind"`
	Model    string       `yaml:"model" json:"model"`
	BaseURL  string       `yaml:"base_url,omitempty" json:"baseUrl,omitempty"`
	Fallback string       `yaml:"fallback,omitempty" json:"fallback,omitempty"`
	Local    bool         `yaml:"local,omitempty" json:"local"`
	PriceIn  float64      `yaml:"price_in,omitempty" json:"priceIn,omitempty"`   // € or $ per million input tokens, optional
	PriceOut float64      `yaml:"price_out,omitempty" json:"priceOut,omitempty"` // per million output tokens, optional
	Currency string       `yaml:"currency,omitempty" json:"currency,omitempty"`
}

// Rulebook ties a folder to the notes that hold its conventions (e.g. IT → IT/Design.md).
type Rulebook struct {
	Folder string   `yaml:"folder" json:"folder"`
	Files  []string `yaml:"files" json:"files"`
}

// Checks configures the rule checks that run without AI.
type Checks struct {
	Codenames     map[string]string   `yaml:"codenames,omitempty" json:"codenames,omitempty"` // old name → replacement ("" = unknown)
	Required      map[string][]string `yaml:"required,omitempty" json:"required,omitempty"`   // folder → required frontmatter keys
	ReviewField   string              `yaml:"review_field,omitempty" json:"reviewField,omitempty"`
	ReviewDays    int                 `yaml:"review_days,omitempty" json:"reviewDays,omitempty"`
	Secrets       *bool               `yaml:"secrets,omitempty" json:"secrets,omitempty"`
	DeadLinks     *bool               `yaml:"dead_links,omitempty" json:"deadLinks,omitempty"`
	ComposeCopies *bool               `yaml:"compose_copies,omitempty" json:"composeCopies,omitempty"`
}

// Config is the whole settings file.
type Config struct {
	Vault         string     `yaml:"vault" json:"vault"`
	Allow         []string   `yaml:"allow" json:"allow"`
	Block         []string   `yaml:"block" json:"block"`
	LocalOnly     []string   `yaml:"local_only,omitempty" json:"localOnly"`
	DefaultRules  []string   `yaml:"default_rules,omitempty" json:"defaultRules"`
	Rulebooks     []Rulebook `yaml:"rulebooks,omitempty" json:"rulebooks"`
	Checks        Checks     `yaml:"checks" json:"checks"`
	Provider      string     `yaml:"provider,omitempty" json:"provider"`
	Providers     []Provider `yaml:"providers,omitempty" json:"providers"`
	Style         Style      `yaml:"style,omitempty" json:"style"`
	Language      string     `yaml:"language,omitempty" json:"language,omitempty"`
	MaxBatchFiles int        `yaml:"max_batch_files,omitempty" json:"maxBatchFiles"`
	MaxTurns      int        `yaml:"max_turns,omitempty" json:"maxTurns"`
	ImportQueue   string     `yaml:"import_queue,omitempty" json:"importQueue,omitempty"`
	StatusNote    string     `yaml:"status_note,omitempty" json:"statusNote,omitempty"`

	path string
}

// Path returns the file the config was loaded from.
func (c *Config) Path() string { return c.path }

// DefaultPath is $HANSEI_CONFIG or $XDG_CONFIG_HOME/hansei/config.yaml.
func DefaultPath() string {
	if p := os.Getenv(configEnv); p != "" {
		return p
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = filepath.Join(os.Getenv("HOME"), ".config")
	}
	return filepath.Join(dir, "hansei", "config.yaml")
}

// DataDir is $HANSEI_DATA or $XDG_DATA_HOME/hansei; batches, journal and stats live there.
func DataDir() string {
	if p := os.Getenv(dataEnv); p != "" {
		return p
	}
	if x := os.Getenv("XDG_DATA_HOME"); x != "" {
		return filepath.Join(x, "hansei")
	}
	return filepath.Join(os.Getenv("HOME"), ".local", "share", "hansei")
}

// ErrNoConfig means the config file does not exist yet.
var ErrNoConfig = errors.New("config: not found")

// Load reads the config at path (DefaultPath if empty) and fills defaults.
func Load(path string) (*Config, error) {
	if path == "" {
		path = DefaultPath()
	}
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("%w: %s", ErrNoConfig, path)
	}
	if err != nil {
		return nil, err
	}

	c := &Config{}
	if err := yaml.Unmarshal(raw, c); err != nil {
		return nil, fmt.Errorf("config %s: %w", path, err)
	}
	c.path = path
	c.fill()
	return c, c.Validate()
}

// New returns a config with defaults for the given vault, not yet saved.
func New(path, vault string) *Config {
	if path == "" {
		path = DefaultPath()
	}
	c := &Config{Vault: vault, path: path}
	c.fill()
	return c
}

// fill sets defaults for missing values and normalises folder lists.
func (c *Config) fill() {
	if c.Style == "" {
		c.Style = StyleSystem
	}
	if c.MaxBatchFiles <= 0 {
		c.MaxBatchFiles = defaultMaxBatchFiles
	}
	if c.MaxTurns <= 0 {
		c.MaxTurns = defaultMaxTurns
	}
	if c.Checks.ReviewDays <= 0 {
		c.Checks.ReviewDays = defaultReviewDays
	}
	if len(c.Providers) == 0 {
		c.Providers = []Provider{{Name: "claude", Kind: KindAnthropic, Model: "claude-opus-5", Fallback: "default",
			PriceIn: opus5In, PriceOut: opus5Out, Currency: usd}}
	}
	if c.Provider == "" {
		c.Provider = c.Providers[0].Name
	}
	c.Vault = expandHome(c.Vault)
	c.ImportQueue = expandHome(c.ImportQueue)
	c.Allow = cleanFolders(c.Allow)
	c.Block = cleanFolders(c.Block)
	c.LocalOnly = cleanFolders(c.LocalOnly)
}

// Validate reports settings that make the vault unusable.
func (c *Config) Validate() error {
	if c.Vault == "" {
		return errors.New("config: vault is not set")
	}
	if !filepath.IsAbs(c.Vault) {
		return fmt.Errorf("config: vault must be an absolute path: %s", c.Vault)
	}
	if !c.Style.valid() {
		return fmt.Errorf("config: unknown style %q", c.Style)
	}
	if _, ok := c.ProviderByName(c.Provider); !ok {
		return fmt.Errorf("config: provider %q is not configured", c.Provider)
	}
	return nil
}

func (s Style) valid() bool {
	return s == StyleSystem || s == StyleKanteLight || s == StyleKante
}

// ProviderByName finds a configured provider.
func (c *Config) ProviderByName(name string) (Provider, bool) {
	for _, p := range c.Providers {
		if p.Name == name {
			return p, true
		}
	}
	return Provider{}, false
}

// Save writes the config atomically with private permissions (it may name key locations).
func (c *Config) Save() error {
	if err := os.MkdirAll(filepath.Dir(c.path), dirMode); err != nil {
		return err
	}
	raw, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	tmp := c.path + ".tmp"
	if err := os.WriteFile(tmp, raw, fileMode); err != nil {
		return err
	}
	return os.Rename(tmp, c.path)
}

// RulebookFiles returns the rule notes for a folder: the longest matching rulebook, else the defaults.
func (c *Config) RulebookFiles(folder string) []string {
	best, bestLen := []string(nil), -1
	for _, rb := range c.Rulebooks {
		f := cleanFolder(rb.Folder)
		if !Within(folder, f) || len(f) <= bestLen {
			continue
		}
		best, bestLen = rb.Files, len(f)
	}
	if best == nil {
		return c.DefaultRules
	}
	return best
}

// RuleFiles lists every rulebook note of the config (default rules and all rulebooks).
func (c *Config) RuleFiles() []string {
	seen := map[string]bool{}
	var out []string
	for _, f := range c.DefaultRules {
		if !seen[f] {
			seen[f] = true
			out = append(out, f)
		}
	}
	for _, rb := range c.Rulebooks {
		for _, f := range rb.Files {
			if !seen[f] {
				seen[f] = true
				out = append(out, f)
			}
		}
	}
	return out
}

// RequiredKeys returns the frontmatter keys required for a note path (longest folder match).
func (c *Config) RequiredKeys(notePath string) []string {
	best, bestLen := []string(nil), -1
	for folder, keys := range c.Checks.Required {
		f := cleanFolder(folder)
		if !Within(notePath, f) || len(f) <= bestLen {
			continue
		}
		best, bestLen = keys, len(f)
	}
	return best
}

// CheckOn reports whether an optional check is enabled (default on).
func CheckOn(flag *bool) bool { return flag == nil || *flag }

// Within reports whether rel lies in folder (folder "" is the vault root).
// Example: Within("IT/Dienste/a.md", "IT") == true, Within("ITX/a.md", "IT") == false.
func Within(rel, folder string) bool {
	if folder == "" {
		return true
	}
	return rel == folder || strings.HasPrefix(rel, folder+"/")
}

func cleanFolders(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, f := range in {
		f = cleanFolder(f)
		if f == "" || seen[f] {
			continue
		}
		seen[f] = true
		out = append(out, f)
	}
	sort.Strings(out)
	return out
}

func cleanFolder(f string) string {
	f = filepath.ToSlash(filepath.Clean(strings.TrimSpace(f)))
	f = strings.Trim(f, "/")
	if f == "." {
		return ""
	}
	return f
}

func expandHome(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		return filepath.Join(os.Getenv("HOME"), strings.TrimPrefix(p, "~"))
	}
	return p
}
