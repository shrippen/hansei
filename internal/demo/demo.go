//go:build demo

// Package demo builds a vault from the shared Studio Weber world (it_docs) and runs Hansei on
// it with a scripted AI. It exists only in demo builds (go build -tags demo) for screenshots;
// release builds contain none of it (scripts/check-release.sh verifies that).
package demo

import (
	_ "embed"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"git.arianw.de/shrippen/hansei/core/batch"
	"git.arianw.de/shrippen/hansei/core/config"
	"git.arianw.de/shrippen/hansei/core/diff"
	"git.arianw.de/shrippen/hansei/core/llm"
	"git.arianw.de/shrippen/hansei/core/rpc"
	"git.arianw.de/shrippen/hansei/core/service"
	"git.arianw.de/shrippen/hansei/core/stats"
	"git.arianw.de/shrippen/hansei/core/vault"
	"git.arianw.de/shrippen/hansei/internal/cli"
	"git.arianw.de/shrippen/hansei/tui"
)

//go:embed world.json
var worldJSON []byte

const (
	todayEnv   = "DEMO_TODAY"
	demoMarker = "hansei-demo" // scripts/check-release.sh looks for this string
	daysInWeek = 7
)

func init() {
	cli.Extra["demo"] = cli.Command{
		Usage: "hansei demo [--lang de|en] [--dir DIR] prepare|daemon|tui   (demo build only)",
		Run:   run,
	}
}

type text map[string]string

// UnmarshalJSON takes {de, en} or a plain string (the same in both languages).
func (t *text) UnmarshalJSON(b []byte) error {
	var s string
	if json.Unmarshal(b, &s) == nil {
		*t = text{"de": s, "en": s}
		return nil
	}
	var m map[string]string
	if err := json.Unmarshal(b, &m); err != nil {
		return err
	}
	*t = m
	return nil
}

func (t text) in(lang string) string {
	if v, ok := t[lang]; ok {
		return v
	}
	return t["de"]
}

type world struct {
	IT struct {
		Allow    []string            `json:"allow"`
		Block    []string            `json:"block"`
		Required map[string][]string `json:"required"`
		Hosts    []struct {
			Name string `json:"name"`
			Old  string `json:"old"`
		} `json:"hosts"`
		Rulebook noteDef    `json:"rulebook"`
		Agent    noteDef    `json:"agent"`
		Notes    []noteDef  `json:"notes"`
		Private  []noteDef  `json:"private"`
		Batches  []batchDef `json:"batches"`
		History  []float64  `json:"conformity_history"`
		Reviewed []int      `json:"reviewed_days"`
		Script   aiScript   `json:"ai_script"`
		Review   struct {
			Field string `json:"field"`
			Days  int    `json:"days"`
		} `json:"review"`
		Providers []struct {
			Name     string `json:"name"`
			Kind     string `json:"kind"`
			Model    string `json:"model"`
			BaseURL  string `json:"base_url"`
			Fallback string `json:"fallback"`
			Local    bool   `json:"local"`
		} `json:"providers"`
		Provider     string   `json:"provider"`
		LocalOnly    []string `json:"local_only"`
		BatchSpacing int      `json:"batch_spacing_min"`
	} `json:"it_docs"`
}

// loadWorld reads the embedded world.
func loadWorld() (world, error) {
	var w world
	err := json.Unmarshal(worldJSON, &w)
	return w, err
}

// codeNames maps the hosts' old code names to their sea mark names.
func codeNames(w world) map[string]string {
	out := map[string]string{}
	for _, h := range w.IT.Hosts {
		out[h.Old] = h.Name
	}
	return out
}

type noteDef struct {
	Path    string `json:"path"`
	Content text   `json:"content"`
}

type batchDef struct {
	ID          string `json:"id"`
	Source      string `json:"source"`
	Provider    string `json:"provider"`
	Title       text   `json:"title"`
	Topic       text   `json:"topic"`
	Instruction text   `json:"instruction"`
	Summary     text   `json:"summary"`
	Done        bool   `json:"done"`
	Day         int    `json:"day"`
	Usage       struct {
		In  int `json:"in"`
		Out int `json:"out"`
	} `json:"usage"`
	Files []struct {
		Path     string `json:"path"`
		Summary  text   `json:"summary"`
		Accept   []int  `json:"accept"`
		Versions []struct {
			Author  string                    `json:"author"`
			Note    string                    `json:"note"`
			Edits   map[string][][2]string    `json:"edits"`
			Changes map[string][]batch.Change `json:"changes"`
		} `json:"versions"`
	} `json:"files"`
	Thread []struct {
		Role     string `json:"role"`
		Scope    string `json:"scope"`
		Path     string `json:"path"`
		Hunk     int    `json:"hunk"`
		Quick    string `json:"quick"`
		Question bool   `json:"question"`
		Text     text   `json:"text"`
	} `json:"thread"`
	Suggestions []struct {
		Text  text `json:"text"`
		Count int  `json:"count"`
	} `json:"suggestions"`
}

func run(args []string) error {
	fs := flag.NewFlagSet("demo", flag.ExitOnError)
	lang := fs.String("lang", "de", "de or en")
	dir := fs.String("dir", "", "demo folder (default $XDG_RUNTIME_DIR/hansei-demo-LANG)")
	_ = fs.Parse(args)
	if *dir == "" {
		base := os.Getenv("XDG_RUNTIME_DIR")
		if base == "" {
			base = os.TempDir()
		}
		*dir = filepath.Join(base, demoMarker+"-"+*lang)
	}
	cmd := fs.Arg(0)
	if cmd == "" {
		cmd = "tui"
	}

	// Everything points into the demo folder: config, data and socket.
	os.Setenv("HANSEI_CONFIG", filepath.Join(*dir, "config.yaml"))
	os.Setenv("HANSEI_DATA", filepath.Join(*dir, "data"))
	os.Setenv("HANSEI_SOCKET", filepath.Join(*dir, "hansei.sock"))

	switch cmd {
	case "prepare":
		return Prepare(*dir, *lang)
	case "daemon":
		if err := ensure(*dir, *lang); err != nil {
			return err
		}
		svc, err := open(*lang)
		if err != nil {
			return err
		}
		defer svc.Close()
		srv, err := rpc.Listen(svc, os.Getenv("HANSEI_SOCKET"))
		if err != nil {
			return err
		}
		fmt.Println("demo daemon on", os.Getenv("HANSEI_SOCKET"))
		return srv.Serve()
	case "tui":
		if err := ensure(*dir, *lang); err != nil {
			return err
		}
		svc, err := open(*lang)
		if err != nil {
			return err
		}
		defer svc.Close()
		st := svc.Status()
		return tui.Run(svc, *lang, st.Style)
	}
	return fmt.Errorf("unknown demo command %q", cmd)
}

func ensure(dir, lang string) error {
	if _, err := os.Stat(filepath.Join(dir, "config.yaml")); err == nil {
		return nil
	}
	return Prepare(dir, lang)
}

// open starts the service with the scripted AI.
func open(lang string) (*service.Service, error) {
	cfg, err := config.Load("")
	if err != nil {
		return nil, err
	}
	w, err := loadWorld()
	if err != nil {
		return nil, err
	}
	return service.Open(cfg, service.Options{Lang: lang, Demo: true, Now: now, Version: "demo",
		Providers: func(p config.Provider) (llm.Provider, error) { return newScript(p.Name, lang, w), nil }})
}

// now honours DEMO_TODAY, keeping the real time of day.
func now() time.Time {
	t := time.Now()
	day, err := time.ParseInLocation(time.DateOnly, os.Getenv(todayEnv), time.Local)
	if err != nil {
		return t
	}
	return time.Date(day.Year(), day.Month(), day.Day(), t.Hour(), t.Minute(), t.Second(), 0, time.Local)
}

// anchor is the Monday of the week that contains today.
func anchor() time.Time {
	t := now()
	wd := (int(t.Weekday()) + daysInWeek - 1) % daysInWeek
	return time.Date(t.Year(), t.Month(), t.Day()-wd, 0, 0, 0, 0, time.Local)
}

var dayRe = regexp.MustCompile(`\{\{day:(-?\d+)\}\}`)

func dates(s string) string {
	a := anchor()
	return dayRe.ReplaceAllStringFunc(s, func(m string) string {
		n, _ := strconv.Atoi(dayRe.FindStringSubmatch(m)[1])
		return a.AddDate(0, 0, n).Format(time.DateOnly)
	})
}

// Prepare writes the vault, the config and the prepared batches into dir (replacing it).
func Prepare(dir, lang string) error {
	var w world
	if err := json.Unmarshal(worldJSON, &w); err != nil {
		return err
	}
	if !strings.Contains(dir, demoMarker) && os.Getenv("HANSEI_DEMO_FORCE") == "" {
		return errors.New("refusing to replace a folder whose name does not contain " + demoMarker)
	}
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	root := filepath.Join(dir, "vault")
	all := append([]noteDef{w.IT.Rulebook, w.IT.Agent}, w.IT.Notes...)
	all = append(all, w.IT.Private...)
	for _, n := range all {
		if err := writeFile(filepath.Join(root, filepath.FromSlash(n.Path)), dates(n.Content.in(lang))); err != nil {
			return err
		}
	}

	c := config.New(filepath.Join(dir, "config.yaml"), root)
	c.Allow, c.Block, c.Language = w.IT.Allow, w.IT.Block, lang
	c.DefaultRules = []string{w.IT.Agent.Path}
	c.Rulebooks = []config.Rulebook{{Folder: "IT", Files: []string{w.IT.Rulebook.Path, w.IT.Agent.Path}}}
	c.Checks.Codenames = codeNames(w)
	c.Checks.Required = w.IT.Required
	c.Checks.ReviewField, c.Checks.ReviewDays = w.IT.Review.Field, w.IT.Review.Days
	c.Providers = nil
	for _, p := range w.IT.Providers {
		c.Providers = append(c.Providers, config.Provider{Name: p.Name, Kind: config.ProviderKind(p.Kind), Model: p.Model,
			BaseURL: p.BaseURL, Fallback: p.Fallback, Local: p.Local})
	}
	c.Provider = w.IT.Provider
	// DEMO_THEME picks the style for screenshots; like every install, the default is System.
	if st := config.Style(os.Getenv("DEMO_THEME")); st == config.StyleKante || st == config.StyleKanteLight {
		c.Style = st
	}
	c.LocalOnly = w.IT.LocalOnly
	if err := c.Save(); err != nil {
		return err
	}

	data := filepath.Join(dir, "data")
	if err := seedBatches(w, data, root, lang); err != nil {
		return err
	}
	if err := seedStats(w, data); err != nil {
		return err
	}
	return applyDone(w, lang)
}

func writeFile(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

// seedBatches stores the prepared batches with their versions, decisions and threads.
func seedBatches(w world, data, root, lang string) error {
	store, err := batch.OpenStore(data)
	if err != nil {
		return err
	}
	base := now()
	for i, bd := range w.IT.Batches {
		created := base.Add(-time.Duration((len(w.IT.Batches)-i)*w.IT.BatchSpacing) * time.Minute)
		if bd.Done {
			created = base.AddDate(0, 0, bd.Day)
		}
		b := &batch.Batch{ID: "demo-" + bd.ID, Title: bd.Title.in(lang), Topic: bd.Topic.in(lang),
			Instruction: bd.Instruction.in(lang), Summary: bd.Summary.in(lang), Source: batch.Source(bd.Source),
			Provider: w.IT.Provider, Status: batch.StatusReview, Created: created, Updated: created,
			Usage: batch.Usage{In: bd.Usage.In, Out: bd.Usage.Out}}

		for _, fd := range bd.Files {
			raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(fd.Path)))
			if err != nil {
				return err
			}
			f := &batch.File{Path: fd.Path, Base: string(raw), BaseHash: vault.Hash(string(raw)), Exists: true,
				Status: batch.FileOpen, Decisions: map[string]batch.Decision{}, Summary: fd.Summary.in(lang)}
			for _, vd := range fd.Versions {
				content := string(raw)
				for _, e := range vd.Edits[lang] {
					content = strings.Replace(content, e[0], e[1], 1)
				}
				f.AddVersion(batch.Version{Content: content, Author: batch.Author(vd.Author), Note: vd.Note, Changes: vd.Changes[lang], Created: created})
			}
			hunks := f.Diff().Hunks
			for _, i := range fd.Accept {
				if i < len(hunks) {
					f.Decide(hunks[i].ID, batch.Accepted)
				}
			}
			b.Files = append(b.Files, f)
		}

		for j, td := range bd.Thread {
			m := batch.Message{Role: batch.Author(td.Role), Text: td.Text.in(lang), Scope: batch.Scope(td.Scope), Path: td.Path,
				Quick: td.Quick, Question: td.Question, Created: created.Add(time.Duration(j+1) * 3 * time.Minute)}
			if m.Scope == batch.ScopeHunk {
				if f, ok := b.File(td.Path); ok {
					// Anchor the message to the hunk as it looks in the first version.
					first := diff.Compare(f.Base, f.Versions[0].Content).Hunks
					if td.Hunk < len(first) {
						m.Hunk = hunkIn(f, first[td.Hunk])
					}
				}
			}
			b.AddMessage(m)
		}
		for _, sd := range bd.Suggestions {
			b.Suggestions = append(b.Suggestions, batch.Suggestion{ID: batch.NewID(), Text: sd.Text.in(lang), Count: sd.Count, Status: batch.SuggestionOpen})
		}
		b.Updated = created.Add(time.Duration(len(bd.Thread)+1) * 3 * time.Minute)
		if err := store.Put(b); err != nil {
			return err
		}
	}
	return nil
}

// hunkIn maps a hunk of the first version to the matching hunk ID of the current version.
func hunkIn(f *batch.File, h diff.Hunk) string {
	for _, cur := range f.Diff().Hunks {
		if strings.Join(cur.OldLines, "\n") == strings.Join(h.OldLines, "\n") {
			return cur.ID
		}
	}
	return h.ID
}

// seedStats writes the conformity history and the review streak.
func seedStats(w world, data string) error {
	st, err := stats.Open(data)
	if err != nil {
		return err
	}
	days := map[string]stats.Day{}
	t := now()
	for i, v := range w.IT.History {
		d := t.AddDate(0, 0, (i-len(w.IT.History))*daysInWeek/2)
		days[d.Format(time.DateOnly)] = stats.Day{Conformity: v, HasConf: true}
	}
	for i, n := range w.IT.Reviewed {
		d := t.AddDate(0, 0, i-len(w.IT.Reviewed)+1)
		day := days[d.Format(time.DateOnly)]
		day.Reviewed = n
		days[d.Format(time.DateOnly)] = day
	}
	return st.Seed(days)
}

// applyDone runs the done batches through the service so the vault and journal match.
func applyDone(w world, lang string) error {
	svc, err := open(lang)
	if err != nil {
		return err
	}
	defer svc.Close()
	for _, bd := range w.IT.Batches {
		if !bd.Done {
			continue
		}
		id := "demo-" + bd.ID
		b, err := svc.Batch(id)
		if err != nil {
			return err
		}
		for _, f := range b.Files {
			if _, err := svc.DecideFile(id, f.Path, string(batch.Accepted)); err != nil {
				return err
			}
		}
	}
	return nil
}
