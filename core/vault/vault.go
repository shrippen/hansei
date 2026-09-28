// Package vault reads and writes the Obsidian vault within the configured folder scope.
//
// Scope:
//
//	allowed   indexed, readable, writable, visible to the AI
//	other     only the file name is known (to resolve wikilinks), content never read
//	blocked   never read, never shown; names are kept privately so links into
//	          blocked notes do not count as dead links
package vault

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"git.arianw.de/shrippen/hansei/core/config"
)

const (
	noteExt   = ".md"
	fileMode  = 0o644
	dirMode   = 0o755
	hashBytes = 12
)

var (
	// ErrOutOfScope means the path is not in an allowed folder or is blocked.
	ErrOutOfScope = errors.New("vault: note is outside the allowed folders")
	// ErrBadPath means the path is not a plain relative Markdown path.
	ErrBadPath = errors.New("vault: invalid note path")

	linkRe = regexp.MustCompile(`!?\[\[([^\]\|#\^]+)(?:[#\^][^\]\|]*)?(?:\|[^\]]*)?\]\]`)
)

// Note is one indexed Markdown file in an allowed folder.
type Note struct {
	Path     string         `json:"path"`
	Title    string         `json:"title"`
	Front    map[string]any `json:"front,omitempty"`
	FrontEnd int            `json:"-"` // line index after the closing '---', 0 without frontmatter
	Headings []string       `json:"headings,omitempty"`
	Links    []string       `json:"links,omitempty"`
	Size     int            `json:"size"`
	Hash     string         `json:"hash"`
	Modified time.Time      `json:"modified"`
}

// Hit is one search result line.
type Hit struct {
	Path string `json:"path"`
	Line int    `json:"line"`
	Text string `json:"text"`
}

// Vault is the scoped view on the vault folder.
type Vault struct {
	root  string
	scope atomic.Pointer[scope]

	mu        sync.RWMutex
	notes     map[string]*Note
	contents  map[string]string
	names     map[string][]string // lower-case base name (without .md) → note paths, all non-blocked notes
	files     map[string]bool     // lower-case base names of all non-blocked files (attachments)
	hidden    map[string]bool     // lower-case base names inside blocked folders, never exposed
	backlinks map[string][]string
	scanned   time.Time
}

// scope is swapped as a whole when the settings change.
type scope struct {
	allow, block, localOnly []string
	rules                   map[string]bool // rulebook notes: allowed as single files, e.g. agent.md in the root
}

// Open prepares a vault from the config; call Scan before use.
func Open(c *config.Config) (*Vault, error) {
	info, err := os.Stat(c.Vault)
	if err != nil {
		return nil, fmt.Errorf("vault: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("vault: %s is not a folder", c.Vault)
	}
	v := &Vault{root: c.Vault}
	v.SetScope(c)
	return v, nil
}

// SetScope takes the folder lists from the config; call Scan afterwards.
func (v *Vault) SetScope(c *config.Config) {
	rules := map[string]bool{}
	for _, f := range c.RuleFiles() {
		rules[f] = true
	}
	v.scope.Store(&scope{allow: c.Allow, block: c.Block, localOnly: c.LocalOnly, rules: rules})
}

// Root returns the vault folder.
func (v *Vault) Root() string { return v.root }

// Scanned returns the time of the last scan.
func (v *Vault) Scanned() time.Time {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.scanned
}

// Scan walks the vault and rebuilds the index.
func (v *Vault) Scan() error {
	notes := map[string]*Note{}
	contents := map[string]string{}
	names := map[string][]string{}
	files := map[string]bool{}
	hidden := map[string]bool{}

	err := filepath.WalkDir(v.root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // unreadable entries are skipped, not fatal
		}
		rel, _ := filepath.Rel(v.root, p)
		rel = filepath.ToSlash(rel)
		if rel == "." {
			return nil
		}

		// Hidden folders (.obsidian, .git, .trash) are never part of the vault content.
		if strings.HasPrefix(d.Name(), ".") {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() || d.Type()&fs.ModeSymlink != 0 {
			return nil
		}

		base := strings.ToLower(strings.TrimSuffix(d.Name(), noteExt))
		if v.blocked(rel) {
			hidden[base] = true
			return nil
		}
		files[strings.ToLower(d.Name())] = true
		if !strings.HasSuffix(rel, noteExt) {
			return nil
		}
		names[base] = append(names[base], rel)
		if !v.allowed(rel) {
			return nil
		}

		// Allowed notes are read and indexed.
		raw, err := os.ReadFile(p)
		if err != nil {
			return nil
		}
		info, _ := d.Info()
		n := parse(rel, string(raw))
		if info != nil {
			n.Modified = info.ModTime()
		}
		notes[rel] = n
		contents[rel] = string(raw)
		return nil
	})
	if err != nil {
		return err
	}

	v.mu.Lock()
	defer v.mu.Unlock()
	v.notes, v.contents, v.names, v.files, v.hidden = notes, contents, names, files, hidden
	v.backlinks = v.buildBacklinks()
	v.scanned = time.Now()
	return nil
}

// buildBacklinks maps each resolved note path to the allowed notes linking to it.
func (v *Vault) buildBacklinks() map[string][]string {
	out := map[string][]string{}
	for p, n := range v.notes {
		for _, l := range n.Links {
			target, ok := v.resolveLocked(l)
			if !ok {
				continue
			}
			out[target] = append(out[target], p)
		}
	}
	for k := range out {
		sort.Strings(out[k])
	}
	return out
}

// Notes returns all allowed notes sorted by path.
func (v *Vault) Notes() []*Note {
	v.mu.RLock()
	defer v.mu.RUnlock()
	out := make([]*Note, 0, len(v.notes))
	for _, n := range v.notes {
		out = append(out, n)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

// Note returns one allowed note from the index.
func (v *Vault) Note(rel string) (*Note, bool) {
	v.mu.RLock()
	defer v.mu.RUnlock()
	n, ok := v.notes[rel]
	return n, ok
}

// Allowed reports whether rel may be read and written.
func (v *Vault) Allowed(rel string) bool {
	return CheckPath(rel) == nil && v.allowed(rel) && !v.blocked(rel)
}

// Blocked reports whether rel lies in a blocked folder.
func (v *Vault) Blocked(rel string) bool { return v.blocked(rel) }

// LocalOnly reports whether rel may only be sent to local models.
func (v *Vault) LocalOnly(rel string) bool {
	for _, f := range v.scope.Load().localOnly {
		if config.Within(rel, f) {
			return true
		}
	}
	return false
}

func (v *Vault) allowed(rel string) bool {
	if v.blocked(rel) {
		return false
	}
	sc := v.scope.Load()
	if sc.rules[rel] {
		return true
	}
	for _, f := range sc.allow {
		if config.Within(rel, f) {
			return true
		}
	}
	return false
}

func (v *Vault) blocked(rel string) bool {
	for _, f := range v.scope.Load().block {
		if config.Within(rel, f) {
			return true
		}
	}
	return false
}

// CheckPath rejects absolute paths, parent references, hidden parts and non-Markdown files.
func CheckPath(rel string) error {
	if rel == "" || strings.Contains(rel, "\\") || strings.ContainsRune(rel, 0) || path.IsAbs(rel) {
		return ErrBadPath
	}
	if path.Clean(rel) != rel || !strings.HasSuffix(rel, noteExt) {
		return ErrBadPath
	}
	for _, part := range strings.Split(rel, "/") {
		if part == ".." || strings.HasPrefix(part, ".") {
			return ErrBadPath
		}
	}
	return nil
}

// Read returns the current content of an allowed note ("" and no error if it does not exist yet).
func (v *Vault) Read(rel string) (string, bool, error) {
	if err := v.guard(rel); err != nil {
		return "", false, err
	}
	raw, err := os.ReadFile(filepath.Join(v.root, filepath.FromSlash(rel)))
	if errors.Is(err, os.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return string(raw), true, nil
}

// ReadRule reads a rulebook note; rule notes may live outside the allowed folders but never in blocked ones.
func (v *Vault) ReadRule(rel string) (string, error) {
	if err := CheckPath(rel); err != nil {
		return "", err
	}
	if v.blocked(rel) {
		return "", ErrOutOfScope
	}
	full, err := v.inside(rel)
	if err != nil {
		return "", err
	}
	raw, err := os.ReadFile(full)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

// Write stores content atomically; new notes get their folders created.
func (v *Vault) Write(rel, content string) error {
	if err := v.guard(rel); err != nil {
		return err
	}
	full, err := v.inside(rel)
	if err != nil {
		return err
	}
	mode := os.FileMode(fileMode)
	if info, err := os.Stat(full); err == nil {
		mode = info.Mode().Perm()
	}
	if err := os.MkdirAll(filepath.Dir(full), dirMode); err != nil {
		return err
	}
	tmp := filepath.Join(filepath.Dir(full), "."+filepath.Base(full)+".hansei-tmp")
	if err := os.WriteFile(tmp, []byte(content), mode); err != nil {
		return err
	}
	if err := os.Rename(tmp, full); err != nil {
		os.Remove(tmp)
		return err
	}
	v.reindex(rel, content)
	return nil
}

// Remove deletes a note (used only to undo the creation of a new note).
func (v *Vault) Remove(rel string) error {
	if err := v.guard(rel); err != nil {
		return err
	}
	full, err := v.inside(rel)
	if err != nil {
		return err
	}
	if err := os.Remove(full); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	v.mu.Lock()
	delete(v.notes, rel)
	delete(v.contents, rel)
	v.mu.Unlock()
	return nil
}

// guard checks path syntax and scope.
func (v *Vault) guard(rel string) error {
	if err := CheckPath(rel); err != nil {
		return err
	}
	if !v.allowed(rel) {
		return ErrOutOfScope
	}
	_, err := v.inside(rel)
	return err
}

// inside resolves rel and makes sure symlinks do not lead out of the vault.
func (v *Vault) inside(rel string) (string, error) {
	full := filepath.Join(v.root, filepath.FromSlash(rel))
	dir := filepath.Dir(full)
	for {
		if _, err := os.Lstat(dir); err == nil {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	realDir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return "", err
	}
	realRoot, err := filepath.EvalSymlinks(v.root)
	if err != nil {
		return "", err
	}
	if realDir != realRoot && !strings.HasPrefix(realDir, realRoot+string(filepath.Separator)) {
		return "", ErrOutOfScope
	}
	if info, err := os.Lstat(full); err == nil && info.Mode()&fs.ModeSymlink != 0 {
		return "", ErrOutOfScope
	}
	return full, nil
}

// reindex updates one note after a write.
func (v *Vault) reindex(rel, content string) {
	n := parse(rel, content)
	n.Modified = time.Now()
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.notes == nil {
		return
	}
	v.notes[rel] = n
	v.contents[rel] = content
	base := strings.ToLower(strings.TrimSuffix(path.Base(rel), noteExt))
	if !contains(v.names[base], rel) {
		v.names[base] = append(v.names[base], rel)
	}
	v.files[strings.ToLower(path.Base(rel))] = true
	v.backlinks = v.buildBacklinks()
}

// Content returns the indexed content of an allowed note.
func (v *Vault) Content(rel string) (string, bool) {
	v.mu.RLock()
	defer v.mu.RUnlock()
	c, ok := v.contents[rel]
	return c, ok
}

// Search finds lines containing all words of query (case-insensitive) in allowed notes.
func (v *Vault) Search(query string, within []string, limit int) []Hit {
	words := strings.Fields(strings.ToLower(query))
	if len(words) == 0 {
		return nil
	}
	v.mu.RLock()
	defer v.mu.RUnlock()

	paths := make([]string, 0, len(v.contents))
	for p := range v.contents {
		if inAny(p, within) {
			paths = append(paths, p)
		}
	}
	sort.Strings(paths)

	var hits []Hit
	for _, p := range paths {
		lowerPath := strings.ToLower(p)
		sc := bufio.NewScanner(strings.NewReader(v.contents[p]))
		sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for line := 1; sc.Scan(); line++ {
			text := sc.Text()
			if !matchAll(strings.ToLower(text)+" "+lowerPath, words) {
				continue
			}
			hits = append(hits, Hit{Path: p, Line: line, Text: strings.TrimSpace(text)})
			if limit > 0 && len(hits) >= limit {
				return hits
			}
		}
	}
	return hits
}

// Backlinks returns allowed notes that link to rel.
func (v *Vault) Backlinks(rel string) []string {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return append([]string(nil), v.backlinks[rel]...)
}

// Resolve maps a wikilink target to a note path; ok=false for unknown or out-of-scope targets.
func (v *Vault) Resolve(target string) (string, bool) {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.resolveLocked(target)
}

func (v *Vault) resolveLocked(target string) (string, bool) {
	t := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(target), noteExt))
	if t == "" {
		return "", false
	}
	base := strings.ToLower(path.Base(t))
	cands := v.names[base]
	if len(cands) == 0 {
		return "", false
	}

	// Links with a folder part must match the path suffix (Obsidian's resolution).
	if strings.Contains(t, "/") {
		suffix := strings.ToLower(t) + noteExt
		for _, c := range cands {
			if strings.HasSuffix(strings.ToLower(c), suffix) {
				return c, true
			}
		}
		return "", false
	}
	return shortest(cands), true
}

// LinkExists reports whether a wikilink points to anything: a note, an attachment or a blocked note.
func (v *Vault) LinkExists(target string) bool {
	v.mu.RLock()
	defer v.mu.RUnlock()
	t := strings.TrimSpace(target)
	lower := strings.ToLower(path.Base(t))
	if ext := path.Ext(lower); ext != "" && ext != noteExt {
		return v.files[lower] || v.hidden[strings.TrimSuffix(lower, ext)]
	}
	if _, ok := v.resolveLocked(t); ok {
		return true
	}
	return v.hidden[strings.TrimSuffix(lower, noteExt)]
}

// Folders lists the top-level folders of the vault that are not hidden.
func (v *Vault) Folders() []string {
	entries, err := os.ReadDir(v.root)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
			out = append(out, e.Name())
		}
	}
	sort.Strings(out)
	return out
}

// Hash is a short content fingerprint used to detect changes behind a proposal's back.
func Hash(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:hashBytes])
}

func parse(rel, content string) *Note {
	n := &Note{Path: rel, Title: strings.TrimSuffix(path.Base(rel), noteExt), Size: len(content), Hash: Hash(content)}
	front, end := Frontmatter(content)
	n.Front, n.FrontEnd = front, end

	inFence := false
	for i, line := range strings.Split(content, "\n") {
		if i < end {
			continue
		}
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		if strings.HasPrefix(trimmed, "#") && strings.Contains(trimmed, " ") && strings.Trim(strings.SplitN(trimmed, " ", 2)[0], "#") == "" {
			n.Headings = append(n.Headings, trimmed)
		}
	}
	n.Links = Links(content)
	return n
}

// Links returns the wikilink targets of a text in order of appearance (with duplicates removed).
func Links(content string) []string {
	var out []string
	seen := map[string]bool{}
	for _, m := range linkRe.FindAllStringSubmatch(content, -1) {
		// In tables Obsidian escapes the alias pipe: [[target\|alias]].
		t := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(m[1]), "\\"))
		if t == "" || seen[t] {
			continue
		}
		seen[t] = true
		out = append(out, t)
	}
	return out
}

func matchAll(s string, words []string) bool {
	for _, w := range words {
		if !strings.Contains(s, w) {
			return false
		}
	}
	return true
}

func inAny(p string, folders []string) bool {
	if len(folders) == 0 {
		return true
	}
	for _, f := range folders {
		if config.Within(p, f) {
			return true
		}
	}
	return false
}

func shortest(paths []string) string {
	best := paths[0]
	for _, p := range paths[1:] {
		if len(p) < len(best) || (len(p) == len(best) && p < best) {
			best = p
		}
	}
	return best
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
