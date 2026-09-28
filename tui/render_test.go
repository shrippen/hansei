package tui

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"git.arianw.de/shrippen/hansei/core/service"
	"git.arianw.de/shrippen/hansei/core/testvault"
)

var ansiRe = regexp.MustCompile(`\x1b\[[0-9;:]*[A-Za-z]`)

// run executes a command chain synchronously (only load commands, no ticks or event waits).
func run(m *Model, cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	msg := cmd()
	switch msg := msg.(type) {
	case nil, tickMsg:
		return
	case tea.BatchMsg:
		for _, c := range msg {
			run(m, c)
		}
		return
	default:
		_, next := m.Update(msg)
		run(m, next)
	}
}

func render(t *testing.T, m *Model, name string) string {
	t.Helper()
	out := ansiRe.ReplaceAllString(m.View(), "")
	if dir := os.Getenv("HANSEI_RENDER"); dir != "" {
		_ = os.WriteFile(filepath.Join(dir, name+".txt"), []byte(out), 0o644)
	}
	for i, l := range strings.Split(out, "\n") {
		if w := len([]rune(l)); w > m.w {
			t.Errorf("%s line %d is %d wide (> %d)", name, i, w, m.w)
		}
	}
	return out
}

func TestScreens(t *testing.T) {
	c := testvault.New(t)
	svc, err := service.Open(c, service.Options{DataDir: t.TempDir(), Lang: "de"})
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()
	sum, err := svc.FromFinding("codename", "")
	if err != nil {
		t.Fatal(err)
	}

	m := New(svc, "de", "kante")
	m.Update(tea.WindowSizeMsg{Width: 150, Height: 40})
	run(m, tea.Batch(m.loadStatus(), m.loadHome(), m.loadBatches()))

	m.screen = scrStart
	if out := render(t, m, "start"); !strings.Contains(out, "Alte Codenamen") || !strings.Contains(out, "REGELKONFORM") {
		t.Errorf("start screen:\n%s", out)
	}

	m.screen = scrReview
	run(m, m.pickBatch())
	out := render(t, m, "review")
	if !strings.Contains(out, "Änderung 1/1") || !strings.Contains(out, "SW-NAS01") || !strings.Contains(out, "Nebelhorn") {
		t.Errorf("review screen:\n%s", out)
	}
	if m.batchID != sum.ID {
		t.Errorf("batch %s, want %s", m.batchID, sum.ID)
	}

	// Accept with the key: the file is written and the next one is shown.
	run(m, m.onKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")}))
	first := m.path
	if m.path == "" {
		t.Fatal("no file after accept")
	}
	run(m, m.onKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")}))
	if m.done == nil {
		t.Errorf("batch not finished after accepting both files (at %s)", first)
	}
	render(t, m, "done")
	m.done = nil

	m.screen = scrBoard
	if out := render(t, m, "board"); !strings.Contains(out, "ÜBERNOMMEN") {
		t.Errorf("board:\n%s", out)
	}

	m.screen = scrJournal
	run(m, m.loadJournal())
	if out := render(t, m, "journal"); !strings.Contains(out, "+1") {
		t.Errorf("journal:\n%s", out)
	}

	m.screen = scrSettings
	run(m, m.loadSettings())
	if out := render(t, m, "settings"); !strings.Contains(out, "gesperrt") || !strings.Contains(out, "freigegeben") {
		t.Errorf("settings:\n%s", out)
	}

	run(m, m.openTask())
	render(t, m, "task")

	// Narrow terminal: panes collapse without overflowing.
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m.screen = scrReview
	render(t, m, "narrow")
}

func TestParityFeatures(t *testing.T) {
	c := testvault.New(t)
	svc, err := service.Open(c, service.Options{DataDir: t.TempDir(), Lang: "de"})
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	m := New(svc, "de", "system")
	m.Update(tea.WindowSizeMsg{Width: 160, Height: 40})
	run(m, tea.Batch(m.loadStatus(), m.loadHome(), m.loadBatches()))

	// Start: space shows the findings of a check below its row.
	m.screen = scrStart
	for i, f := range m.home.Findings {
		if f.Rule == "codename" {
			m.row = i
		}
	}
	run(m, m.onKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")}))
	if out := render(t, m, "start-open"); !strings.Contains(out, "▾") || !strings.Contains(out, ".md") {
		t.Errorf("findings not expanded:\n%s", out)
	}

	// Enter creates the batch; a second Enter opens it instead of a duplicate.
	run(m, m.onKey(tea.KeyMsg{Type: tea.KeyEnter}))
	first := m.batchID
	run(m, m.loadHome())
	m.screen = scrStart
	run(m, m.onKey(tea.KeyMsg{Type: tea.KeyEnter}))
	if m.batchID != first || len(m.batches) != 1 {
		t.Errorf("second Enter made a new batch: %s vs %s, %d batches", m.batchID, first, len(m.batches))
	}

	// Mouse: a click on the second file of the list opens it.
	render(t, m, "review-click")
	var target *hit
	for i, h := range m.hits {
		if h.path != "" && h.path != m.path {
			target = &m.hits[i]
		}
	}
	if target == nil {
		t.Fatal("no clickable file in the batch list")
	}
	run(m, m.onClick(target.x0+1, target.y))
	if m.path != target.path {
		t.Errorf("click opened %q, want %q", m.path, target.path)
	}

	// Journal: Enter shows the diff of a write.
	run(m, m.onKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("A")}))
	m.screen = scrJournal
	run(m, m.loadJournal())
	run(m, m.onKey(tea.KeyMsg{Type: tea.KeyEnter}))
	if m.compare == nil || m.compareFile == nil {
		t.Fatal("journal diff not shown")
	}
	render(t, m, "journal-diff")
}
