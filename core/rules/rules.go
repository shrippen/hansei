// Package rules checks notes against the conventions that need no AI and loads the
// rulebook notes (e.g. IT/Design.md) that the AI must follow.
package rules

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"git.arianw.de/shrippen/hansei/core/config"
	"git.arianw.de/shrippen/hansei/core/redact"
	"git.arianw.de/shrippen/hansei/core/vault"
)

// ID names a check.
type ID string

const (
	Secret      ID = "secret"      // plain-text password, token, key
	Codename    ID = "codename"    // old host name that should be replaced
	Frontmatter ID = "frontmatter" // required frontmatter key missing
	Review      ID = "review"      // review date older than the limit
	DeadLink    ID = "dead-link"   // wikilink to nothing
	Compose     ID = "compose"     // full docker compose copy in the body
)

// Order is the display order of the checks, most urgent first.
var Order = []ID{Secret, Codename, Frontmatter, DeadLink, Compose, Review}

const (
	deprecatedKey = "deprecated"
	excerptMax    = 160
	hoursPerDay   = 24
)

// Finding is one problem in one note.
type Finding struct {
	Rule    ID     `json:"rule"`
	Path    string `json:"path"`
	Line    int    `json:"line"`
	Excerpt string `json:"excerpt"` // secrets are always masked
	Detail  string `json:"detail"`  // e.g. "ZeroNAS→Regis", a missing key, a link target, age in days
}

// Summary counts findings of one rule.
type Summary struct {
	Rule  ID       `json:"rule"`
	Count int      `json:"count"`
	Paths []string `json:"paths"`
}

// Report is the result of a full check.
type Report struct {
	Findings   []Finding `json:"findings"`
	Summary    []Summary `json:"summary"`
	Notes      int       `json:"notes"`
	Clean      int       `json:"clean"`
	Conformity float64   `json:"conformity"` // share of notes without findings, 0..1
	Checked    time.Time `json:"checked"`
}

var (
	// linkTargetRe matches link targets and embeds, where a code name is a file name, not a mention.
	linkTargetRe      = regexp.MustCompile(`\]\([^)]*\)|!?\[\[[^\]]*\]\]|<[^>]*\.(?:png|jpe?g|gif|svg|webp|pdf)>`)
	composeServicesRe = regexp.MustCompile(`(?m)^\s*services:\s*$`)
	composeImageRe    = regexp.MustCompile(`(?m)^\s+image:\s*\S+`)
)

// Check runs all enabled checks over the allowed notes.
func Check(v *vault.Vault, c *config.Config, now time.Time) Report {
	rep := Report{Checked: now}
	dirty := map[string]bool{}
	books := rulebookFiles(c)
	names := compileNames(c.Checks.Codenames)

	for _, n := range v.Notes() {
		content, ok := v.Content(n.Path)
		if !ok {
			continue
		}
		rep.Notes++
		found := checkNote(v, c, names, n, content, now, books[n.Path])
		if len(found) > 0 {
			dirty[n.Path] = true
		}
		rep.Findings = append(rep.Findings, found...)
	}

	rep.Clean = rep.Notes - len(dirty)
	if rep.Notes > 0 {
		rep.Conformity = float64(rep.Clean) / float64(rep.Notes)
	}
	rep.Summary = summarize(rep.Findings)
	return rep
}

// checkNote runs the checks for one note.
func checkNote(v *vault.Vault, c *config.Config, names []codename, n *vault.Note, content string, now time.Time, rulebook bool) []Finding {
	lines := strings.Split(content, "\n")
	var out []Finding

	if config.CheckOn(c.Checks.Secrets) {
		out = append(out, secrets(n.Path, content, lines)...)
	}
	// Deprecated notes are history: only secrets and dead links still matter there.
	// Rulebooks name the old code names on purpose.
	if !isDeprecated(n) {
		if !rulebook {
			out = append(out, codenames(n.Path, lines, names)...)
		}
		out = append(out, required(n, c.RequiredKeys(n.Path))...)
		if f, ok := review(n, c.Checks.ReviewField, c.Checks.ReviewDays, now); ok {
			out = append(out, f)
		}
	}
	if config.CheckOn(c.Checks.DeadLinks) {
		out = append(out, deadLinks(v, n, lines)...)
	}
	if config.CheckOn(c.Checks.ComposeCopies) {
		out = append(out, composeCopies(n.Path, lines)...)
	}
	return out
}

func secrets(p, content string, lines []string) []Finding {
	var out []Finding
	for _, m := range redact.Find(content) {
		line := strings.Count(content[:m.Start], "\n")
		out = append(out, Finding{Rule: Secret, Path: p, Line: line + 1, Excerpt: excerpt(lines[line]), Detail: string(m.Kind)})
	}
	return out
}

// codename is one old name with its whole-word pattern.
type codename struct {
	old, repl string
	re        *regexp.Regexp
}

// compileNames builds the patterns once per check, sorted for stable output.
func compileNames(names map[string]string) []codename {
	keys := make([]string, 0, len(names))
	for k := range names {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]codename, 0, len(keys))
	for _, k := range keys {
		out = append(out, codename{old: k, repl: names[k], re: WordRe(k)})
	}
	return out
}

// WordRe matches name as a whole word, e.g. "ZeroNAS" but not "ZeroNASBackup".
func WordRe(name string) *regexp.Regexp {
	return regexp.MustCompile(`(^|[^\pL\pN_])` + regexp.QuoteMeta(name) + `($|[^\pL\pN_])`)
}

// codenames finds old names outside link targets.
func codenames(p string, lines []string, names []codename) []Finding {
	var out []Finding
	for _, cn := range names {
		for i, line := range lines {
			if !cn.re.MatchString(linkTargetRe.ReplaceAllString(line, "")) {
				continue
			}
			detail := cn.old
			if cn.repl != "" {
				detail = cn.old + "→" + cn.repl
			}
			out = append(out, Finding{Rule: Codename, Path: p, Line: i + 1, Excerpt: excerpt(line), Detail: detail})
		}
	}
	return out
}

func required(n *vault.Note, keys []string) []Finding {
	var out []Finding
	for _, k := range keys {
		if val, ok := n.Front[k]; ok && vault.FrontString(val) != "" {
			continue
		}
		if _, ok := n.Front[k]; ok {
			continue // present but empty counts as set; the owner decided to leave it open
		}
		out = append(out, Finding{Rule: Frontmatter, Path: n.Path, Line: 1, Detail: k})
	}
	return out
}

// review flags a review date older than days. Notes without the field are left to the
// frontmatter check, since not every folder requires it.
func review(n *vault.Note, field string, days int, now time.Time) (Finding, bool) {
	if field == "" {
		return Finding{}, false
	}
	raw, ok := n.Front[field]
	if !ok {
		return Finding{}, false
	}
	date, ok := parseDate(raw)
	if !ok {
		return Finding{}, false
	}
	age := int(now.Sub(date).Hours() / hoursPerDay)
	if age <= days {
		return Finding{}, false
	}
	return Finding{Rule: Review, Path: n.Path, Line: 1, Excerpt: field + ": " + date.Format(time.DateOnly), Detail: fmt.Sprint(age)}, true
}

func parseDate(v any) (time.Time, bool) {
	switch t := v.(type) {
	case time.Time:
		return t, true
	case string:
		d, err := time.Parse(time.DateOnly, strings.TrimSpace(t))
		return d, err == nil
	}
	return time.Time{}, false
}

func deadLinks(v *vault.Vault, n *vault.Note, lines []string) []Finding {
	var out []Finding
	for _, target := range n.Links {
		if v.LinkExists(target) {
			continue
		}
		line := 0
		for i, l := range lines {
			if strings.Contains(l, "[["+target) {
				line = i
				break
			}
		}
		out = append(out, Finding{Rule: DeadLink, Path: n.Path, Line: line + 1, Excerpt: excerpt(lines[line]), Detail: target})
	}
	return out
}

// composeCopies finds fenced blocks that look like a full compose file (services: + image:).
func composeCopies(p string, lines []string) []Finding {
	var out []Finding
	start := -1
	for i, l := range lines {
		t := strings.TrimSpace(l)
		if !strings.HasPrefix(t, "```") && !strings.HasPrefix(t, "~~~") {
			continue
		}
		if start < 0 {
			start = i
			continue
		}
		block := strings.Join(lines[start+1:i], "\n")
		if composeServicesRe.MatchString(block) && composeImageRe.MatchString(block) {
			out = append(out, Finding{Rule: Compose, Path: p, Line: start + 1, Excerpt: excerpt(lines[start]), Detail: fmt.Sprint(i - start - 1)})
		}
		start = -1
	}
	return out
}

// OutsideLinks applies fn to the parts of text that are not link targets or embeds,
// so a replacement never breaks a file reference.
// Example: OutsideLinks("ZeroNAS ![[ZeroNAS.png]]", upper) → "ZERONAS ![[ZeroNAS.png]]".
func OutsideLinks(text string, fn func(string) string) string {
	var b strings.Builder
	last := 0
	for _, m := range linkTargetRe.FindAllStringIndex(text, -1) {
		b.WriteString(fn(text[last:m[0]]))
		b.WriteString(text[m[0]:m[1]])
		last = m[1]
	}
	b.WriteString(fn(text[last:]))
	return b.String()
}

// rulebookFiles collects every rule note of the config.
func rulebookFiles(c *config.Config) map[string]bool {
	out := map[string]bool{}
	for _, f := range c.RuleFiles() {
		out[f] = true
	}
	return out
}

func isDeprecated(n *vault.Note) bool {
	v, ok := n.Front[deprecatedKey]
	if !ok {
		return false
	}
	b, ok := v.(bool)
	return ok && b
}

func summarize(findings []Finding) []Summary {
	by := map[ID]*Summary{}
	paths := map[ID]map[string]bool{}
	for _, f := range findings {
		s, ok := by[f.Rule]
		if !ok {
			s = &Summary{Rule: f.Rule}
			by[f.Rule] = s
			paths[f.Rule] = map[string]bool{}
		}
		s.Count++
		if !paths[f.Rule][f.Path] {
			paths[f.Rule][f.Path] = true
			s.Paths = append(s.Paths, f.Path)
		}
	}
	var out []Summary
	for _, id := range Order {
		if s, ok := by[id]; ok {
			sort.Strings(s.Paths)
			out = append(out, *s)
		}
	}
	return out
}

func excerpt(line string) string {
	line = strings.TrimSpace(redact.Mask(line))
	if r := []rune(line); len(r) > excerptMax {
		return string(r[:excerptMax]) + "…"
	}
	return line
}

// Book is the combined rulebook for a set of folders.
type Book struct {
	Files []string `json:"files"`
	Text  string   `json:"-"`
}

// Rulebook loads the rule notes for the given folders (deduplicated, in config order).
func Rulebook(v *vault.Vault, c *config.Config, folders []string) Book {
	var book Book
	seen := map[string]bool{}
	if len(folders) == 0 {
		folders = c.Allow
	}
	var b strings.Builder
	for _, f := range folders {
		for _, file := range c.RulebookFiles(f) {
			if seen[file] {
				continue
			}
			seen[file] = true
			text, err := v.ReadRule(file)
			if err != nil {
				continue
			}
			book.Files = append(book.Files, file)
			fmt.Fprintf(&b, "<rulebook path=%q>\n%s\n</rulebook>\n", file, redact.New().Redact(text))
		}
	}
	book.Text = b.String()
	return book
}
