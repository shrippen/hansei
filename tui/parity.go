package tui

import (
	"encoding/json"
	"fmt"
	"path"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"git.arianw.de/shrippen/hansei/core/batch"
	"git.arianw.de/shrippen/hansei/core/service"
)

// Features the KDE app got from the view review, with the same service calls:
// expandable findings, waiting batches, questions first, round summaries, rule sections,
// find-only tasks, journal diffs, provider tests, live cost and the mouse.

const (
	findingsShown = 8
	waitingShown  = 3
	ruleLines     = 12 // lines of a rule shown above the diff
	minutesPerDay = 24 * 60
)

// hit is a clickable area of the last frame.
type hit struct {
	y, x0, x1 int
	batch     string
	path      string
	hunk      int
}

func (m *Model) addHit(h hit) { m.hits = append(m.hits, h) }

// onClick selects what was clicked: a batch or file in the list, a change header in the diff.
func (m *Model) onClick(x, y int) tea.Cmd {
	for _, h := range m.hits {
		if y != h.y || x < h.x0 || x >= h.x1 {
			continue
		}
		switch {
		case h.hunk >= 0:
			m.hunk = h.hunk
			return nil
		case h.path != "":
			m.path, m.file = h.path, nil
			return m.loadFile()
		case h.batch != "":
			if h.batch == m.batchID {
				return nil
			}
			m.batchID, m.path, m.file, m.batch = h.batch, "", nil, nil
			return m.loadBatch(h.batch)
		}
	}
	return nil
}

// ---------- Start ----------

func (m *Model) loadFindings() tea.Cmd {
	return func() tea.Msg { return findingsMsg(m.api.Findings(false)) }
}

// findingLines lists the findings of one rule under its row.
func (m *Model) findingLines(rule string, w int) []string {
	if m.findings == nil {
		return []string{"    " + m.st.dim.Render("…")}
	}
	var out []string
	n := 0
	for _, f := range m.findings.Findings {
		if string(f.Rule) != rule {
			continue
		}
		n++
		if n > findingsShown {
			continue
		}
		where := path.Base(f.Path)
		if f.Line > 0 {
			where += fmt.Sprintf(":%d", f.Line)
		}
		detail := strings.TrimSpace(strings.Join(nonEmpty(f.Detail, f.Excerpt), " · "))
		out = append(out, "    "+m.st.strong.Render(trunc(where, 32))+"  "+m.st.dim.Render(trunc(detail, max(10, w-40))))
	}
	if n > findingsShown {
		out = append(out, "    "+m.st.dim.Render(m.t("more", n-findingsShown)))
	}
	return out
}

func nonEmpty(s ...string) []string {
	var out []string
	for _, x := range s {
		if x != "" {
			out = append(out, x)
		}
	}
	return out
}

// waitingLines shows up to three batches waiting for review.
func (m *Model) waitingLines(w int) []string {
	var out []string
	for _, b := range m.batches {
		if b.Column != "review" && b.Column != "feedback" {
			continue
		}
		if len(out) == waitingShown {
			break
		}
		open := b.Counts.Hunks - b.Counts.Accepted - b.Counts.Rejected
		line := "  " + m.st.chipTag.Render(trunc(b.Topic, 12)) + " " + m.st.strong.Render(trunc(b.Title, max(10, w-50))) +
			m.st.dim.Render(" · "+m.t("openN", open))
		if b.Questions > 0 {
			line += " " + m.st.chipWarn.Render(m.n("questions", b.Questions))
		}
		out = append(out, line)
	}
	return out
}

func (m *Model) ago(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	mins := int(time.Since(t).Minutes())
	switch {
	case mins < 60:
		return m.t("agoMin", max(1, mins))
	case mins < minutesPerDay:
		return m.t("agoHour", mins/60)
	}
	return m.t("agoDay", mins/minutesPerDay)
}

// ---------- Review ----------

// questionLines puts open AI questions above the diff.
func (m *Model) questionLines(w int) []string {
	if m.batch == nil {
		return nil
	}
	var out []string
	for _, msg := range m.batch.Thread {
		if msg.Question && !msg.Answered {
			for i, l := range wrap(msg.Text, w-4) {
				prefix := "  "
				if i == 0 {
					prefix = "? "
				}
				out = append(out, m.st.chipWarn.Render(prefix+l))
			}
		}
	}
	if len(out) > 0 {
		out = append(out, m.st.dim.Render("  "+m.t("answerHint")))
	}
	return out
}

// systemLines renders a round summary: which files got which version.
func (m *Model) systemLines(msg batch.Message, w int) []string {
	text := msg.Text
	if len(msg.Versions) > 0 {
		var parts []string
		for _, p := range sortedKeys(msg.Versions) {
			parts = append(parts, fmt.Sprintf("%s → v%d", path.Base(p), msg.Versions[p]))
		}
		text = strings.Join(parts, " · ")
	}
	var out []string
	for _, l := range wrap("── "+text+" ──", w) {
		out = append(out, m.center(m.st.dim.Render(l), w))
	}
	return out
}

func sortedKeys(mp map[string]int) []string {
	keys := make([]string, 0, len(mp))
	for k := range mp {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// collapsed: decided changes other than the current one show only their header.
func (m *Model) collapsed(f *service.FileView, i int) bool {
	return i >= 0 && i < len(f.Hunks) && i != m.hunk && f.Hunks[i].State != "pending"
}

func (m *Model) loadRule() tea.Cmd {
	h := m.currentHunk()
	if h == nil || h.Rule == "" {
		return nil
	}
	ref := h.Rule
	return func() tea.Msg {
		r, err := m.api.RuleSection(ref)
		if err != nil {
			return errMsg{err}
		}
		return ruleMsg(r)
	}
}

// ruleBox: the rulebook section behind the current change, framed, above the diff.
func (m *Model) ruleBox(w int) []string {
	r := m.rule
	title := r.File
	if r.Heading != "" {
		title += " › " + strings.TrimLeft(r.Heading, "# ")
	}
	body := []string{m.st.strong.Render(title) + m.st.dim.Render("  (Esc)")}
	for _, l := range strings.Split(strings.TrimSpace(r.Text), "\n") {
		body = append(body, wrap(l, w-6)...)
	}
	if len(body) > ruleLines {
		body = append(body[:ruleLines], "…")
	}
	box := lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(m.st.p.info).Padding(0, 1).Width(w - 2)
	return strings.Split(box.Render(strings.Join(body, "\n")), "\n")
}

// liveUsage decodes a usage event.
func (m *Model) liveUsage(e service.Event) {
	var u batch.Usage
	if json.Unmarshal([]byte(e.Text), &u) == nil {
		m.live[e.Batch] = u
	}
}

func (m *Model) usageOf(b *service.BatchView) batch.Usage {
	if u, ok := m.live[b.ID]; ok && (b.Running || b.Revising) {
		return u
	}
	return b.Usage
}

// doneLine: time, tokens and cost of the finished batch.
func (m *Model) doneLine(b service.BatchSummary) string {
	parts := []string{m.t("minutes", max(1, int(b.Updated.Sub(b.Created).Minutes())))}
	if n := b.Usage.In + b.Usage.Out; n > 0 {
		parts = append(parts, m.t("cost", (n+500)/1000))
	}
	if b.Usage.Cost > 0 {
		parts = append(parts, fmt.Sprintf("%.2f", b.Usage.Cost))
	}
	parts = append(parts, m.t("conformityNow", int(m.home.Stats.Conformity*percent+0.5)))
	return strings.Join(parts, " · ")
}

// ---------- Journal ----------

func (m *Model) journalDiff(e service.JournalView) tea.Cmd {
	return func() tea.Msg {
		c, err := m.api.JournalDiff(e.ID)
		if err != nil {
			return errMsg{err}
		}
		return journalDiffMsg{title: e.Path + " · " + e.Time.Format("02.01. 15:04"), cmp: c}
	}
}

func (m *Model) undoBatch(id string) tea.Cmd {
	return func() tea.Msg {
		if _, err := m.api.UndoBatch(id); err != nil {
			return errMsg{err}
		}
		return journalMsg(m.api.Journal(200))
	}
}

// ---------- Settings ----------

func (m *Model) checkProvider(name string) tea.Cmd {
	return func() tea.Msg {
		r := m.api.CheckProvider(name)
		if !r.OK {
			return errMsg{fmt.Errorf("%s: %s", name, r.Message)}
		}
		return toastMsg(m.t("providerOK", name, len(r.Models)))
	}
}

// ---------- Task ----------

var taskTemplates = []string{"template1", "template2", "template3"}

func (m *Model) nextTemplate() {
	cur := m.task.Value()
	next := 0
	for i, k := range taskTemplates {
		if cur == m.t(k) {
			next = (i + 1) % len(taskTemplates)
		}
	}
	if cur != "" && next == 0 && cur != m.t(taskTemplates[len(taskTemplates)-1]) {
		return // your own text stays
	}
	m.task.SetValue(m.t(taskTemplates[next]))
}

// costText: the cost of a batch, empty without a price.
func costText(u batch.Usage) string {
	if u.Cost <= 0 {
		return ""
	}
	return fmt.Sprintf("%.2f", u.Cost)
}
