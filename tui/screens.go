package tui

import (
	"fmt"
	"path"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"git.arianw.de/shrippen/hansei/core/service"
)

const (
	boardColumns = 5
	cardHeight   = 4
	sparkRunes   = "▁▂▃▄▅▆▇█"
	findingBar   = 20  // cells of a findings bar
	wideStart    = 120 // columns from which the start page spells out its actions
	emptyColumn  = 6   // width of a board column without cards
	percent      = 100
)

// ---------- Start ----------

func (m *Model) viewStart() string {
	h := m.home
	w := m.w
	tileW := max(24, (w-4)/3)

	conf := fmt.Sprintf("%d %%", int(h.Stats.Conformity*percent+0.5))
	confSub := ""
	if h.Stats.HasWeekAgo {
		confSub = m.t("thisWeek", int((h.Stats.Conformity-h.Stats.WeekAgo)*percent+0.5))
	}
	tiles := []string{
		m.tile(m.t("conformity"), conf, confSub+"  "+m.spark(h.Stats.Spark), tileW),
		m.tile(m.t("waiting"), fmt.Sprint(h.Waiting), m.n("waitingN", h.Waiting, h.WaitingHunks), tileW),
		m.tile(m.t("streak"), fmt.Sprint(h.Stats.Streak), m.n("streakN", h.Stats.Streak)+" · "+m.t("today", h.Stats.ReviewedToday), tileW),
	}
	var lines []string
	lines = append(lines, lipgloss.JoinHorizontal(lipgloss.Top, tiles[0], " ", tiles[1], " ", tiles[2]), "")
	if waiting := m.waitingLines(w); len(waiting) > 0 {
		lines = append(lines, m.st.label.Render(strings.ToUpper(m.t("waiting")))+m.st.dim.Render("  Tab"))
		lines = append(lines, waiting...)
		lines = append(lines, "")
	}
	lines = append(lines, m.st.label.Render(strings.ToUpper(m.t("findings"))))
	if len(h.Findings) == 0 {
		lines = append(lines, m.st.chipOK.Render(m.t("noFindings")))
	}
	maxCount := 1
	for _, f := range h.Findings {
		maxCount = max(maxCount, f.Count)
	}
	barW := min(findingBar, max(8, w/6))
	for i, f := range h.Findings {
		title := h.Titles[string(f.Rule)]
		n := max(1, f.Count*barW/maxCount)
		bar := m.ruleStyle(string(f.Rule)).Render(strings.Repeat("█", n)) + m.st.filler.Render(strings.Repeat("░", barW-n))
		open := "▸ "
		if m.expanded == string(f.Rule) {
			open = "▾ "
		}
		row := open + fmt.Sprintf("%-32s ", trunc(title, 32)) + bar + "  " + m.st.dim.Render(m.countText(f.Count, len(f.Paths)))
		action := m.t("createBatch")
		if h.Open[string(f.Rule)] != "" {
			action = m.t("openBatch")
		}
		if i == m.row {
			hint := m.st.key.Render("⏎ "+action) + m.st.dim.Render("  ␣ "+m.t("details"))
			if w < wideStart {
				hint = m.st.key.Render("⏎")
			}
			row = m.st.key.Render("▌ ") + m.st.strong.Render(row) + "  " + hint
		} else {
			row = "  " + row
		}
		lines = append(lines, row)
		if m.expanded == string(f.Rule) {
			lines = append(lines, m.findingLines(string(f.Rule), w)...)
		}
	}
	foot := []string{m.status.VaultName, strings.Join(m.status.Allowed, ", "), m.n("notes", m.status.Notes)}
	if ago := m.ago(h.Checked); ago != "" {
		foot = append(foot, m.t("checked", ago))
	}
	lines = append(lines, "", m.st.dim.Render(strings.Join(nonEmpty(foot...), " · ")))
	return strings.Join(lines, "\n")
}

// tile is a figure with a label; the text below wraps to two lines instead of being cut.
func (m *Model) tile(label, big, sub string, w int) string {
	box := lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(m.st.p.border).Width(w-2).Padding(0, 1)
	// Styled text (the sparkline) must not be split: wrap only what does not fit.
	lines := []string{sub}
	if lipgloss.Width(sub) > w-4 {
		lines = wrap(sub, w-4)
	}
	if len(lines) > 2 {
		lines = append(lines[:1], trunc(strings.Join(lines[1:], " "), w-4))
	}
	for len(lines) < 2 {
		lines = append(lines, "")
	}
	return box.Render(m.st.label.Render(strings.ToUpper(label)) + "\n" + m.st.title.Render(big) + "\n" + m.st.dim.Render(strings.Join(lines, "\n")))
}

// countText: "3 Befunde" or "3 Befunde in 2 Notizen", like the app.
func (m *Model) countText(findings, notes int) string {
	if findings == notes {
		return m.n("findingsN", findings)
	}
	return m.t("inNotes", m.n("findingsN", findings), m.n("notes", notes))
}

// ruleStyle colours a check by how urgent it is, like the app.
func (m *Model) ruleStyle(rule string) lipgloss.Style {
	switch rule {
	case "secret":
		return m.st.chipFail
	case "codename", "frontmatter":
		return m.st.chipHl
	case "review":
		return m.st.chipInfo
	}
	return m.st.chipWarn
}

func (m *Model) spark(vals []float64) string {
	runes := []rune(sparkRunes)
	var b strings.Builder
	for _, v := range vals {
		i := int(v * float64(len(runes)-1))
		b.WriteRune(runes[clamp(i, 0, len(runes)-1)])
	}
	return m.st.chipOK.Render(b.String())
}

// ---------- Board ----------

type card struct {
	id, rule    string
	title, meta string
	info        string
	topic, note string
	noteStyle   lipgloss.Style
	bar         string
}

type column struct {
	key   string
	style lipgloss.Style
	cards []card
}

func (m *Model) boardColumns() []column {
	cols := []column{
		{key: "col.findings", style: m.st.chipTag},
		{key: "col.working", style: m.st.chipWarn},
		{key: "col.review", style: m.st.chipTag},
		{key: "col.feedback", style: m.st.chipInfo},
		{key: "col.done", style: m.st.chipOK},
	}
	for _, f := range m.home.Findings {
		cols[0].cards = append(cols[0].cards, card{rule: string(f.Rule), title: m.home.Titles[string(f.Rule)],
			meta: m.countText(f.Count, len(f.Paths))})
	}
	for _, b := range m.batches {
		c := card{id: b.ID, title: b.Title, topic: b.Topic,
			meta: m.n("doneFiles", b.Counts.Files) + " · " + m.n("hunks", b.Counts.Hunks), bar: m.progressBar(b.Counts, 18)}
		// Age, provider and cost, like the app's cards.
		c.info = strings.Join(nonEmpty(m.ago(b.Created), b.Provider, costText(b.Usage)), " · ")
		i := -1
		switch b.Column {
		case "working":
			i = 1
			c.note, c.noteStyle = trunc(m.progress[b.ID], 30), m.st.chipWarn
		case "review":
			i = 2
		case "feedback":
			i = 3
			if b.Questions > 0 {
				c.note, c.noteStyle = m.n("questions", b.Questions), m.st.chipInfo
			} else {
				c.note, c.noteStyle = m.t("revising", maxVersions(b)+1), m.st.chipInfo
			}
		case "done":
			i = 4
			c.meta = b.Updated.Format("02.01. 15:04") + " · " + m.n("doneFiles", b.Counts.Files)
			c.info = strings.Join(nonEmpty(b.Provider, costText(b.Usage)), " · ")
		case "failed":
			i = 1
			c.note, c.noteStyle = trunc(b.Error, 40), m.st.chipFail
		}
		if i >= 0 {
			cols[i].cards = append(cols[i].cards, c)
		}
	}
	return cols
}

func maxVersions(b service.BatchSummary) int {
	n := 1
	for _, f := range b.Files {
		n = max(n, f.Versions)
	}
	return n
}

func (m *Model) selectedCard(cols []column) *card {
	if m.col >= len(cols) || m.row >= len(cols[m.col].cards) || m.row < 0 {
		return nil
	}
	return &cols[m.col].cards[m.row]
}

func (m *Model) viewBoard() string {
	cols := m.boardColumns()
	m.col = clamp(m.col, 0, len(cols)-1)
	m.row = clamp(m.row, 0, len(cols[m.col].cards)-1)
	// Empty columns shrink to a narrow strip; the others share the rest.
	empty := 0
	for i, c := range cols {
		if len(c.cards) == 0 && i != m.col {
			empty++
		}
	}
	full := max(1, len(cols)-empty)
	cwFull := max(18, (m.w-empty*emptyColumn-full+1)/full)
	h := m.bodyHeight()

	var rendered []string
	for i, c := range cols {
		cw := cwFull
		if len(c.cards) == 0 && i != m.col {
			cw = emptyColumn
			strip := []string{c.style.Render("0"), c.style.Render(strings.Repeat("━", cw-1))}
			for _, r := range []rune(strings.ToUpper(m.t(c.key))) {
				strip = append(strip, m.st.dim.Render(string(r)))
			}
			rendered = append(rendered, fit(strings.Join(strip, "\n"), cw, h))
			continue
		}
		var lines []string
		head := m.st.label.Render(strings.ToUpper(m.t(c.key))) + c.style.Render(fmt.Sprintf("  %d", len(c.cards)))
		lines = append(lines, head, c.style.Render(strings.Repeat("━", cw-1)))
		for j, cd := range c.cards {
			lines = append(lines, m.renderCard(cd, cw-1, i == m.col && j == m.row)...)
		}
		rendered = append(rendered, fit(strings.Join(lines, "\n"), cw, h))
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, rendered...)
}

func (m *Model) renderCard(c card, w int, selected bool) []string {
	border := m.st.p.border
	if selected {
		border = m.st.p.accent
	}
	var body []string
	if c.topic != "" {
		body = append(body, m.st.chipTag.Render(trunc(c.topic, w-4)))
	}
	body = append(body, m.st.strong.Render(trunc(c.title, w-4)))
	if c.note != "" {
		body = append(body, c.noteStyle.Render(trunc(c.note, w-4)))
	}
	body = append(body, m.st.dim.Render(trunc(c.meta, w-4)))
	if c.info != "" {
		body = append(body, m.st.dim.Render(trunc(c.info, w-4)))
	}
	if c.bar != "" {
		body = append(body, c.bar)
	}
	box := lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(border).Width(w-2).Padding(0, 1)
	return strings.Split(box.Render(strings.Join(body, "\n")), "\n")
}

// ---------- Journal ----------

func (m *Model) viewJournal() string {
	if len(m.journal) == 0 {
		return m.st.dim.Render(m.t("journalEmpty"))
	}
	lines := []string{m.st.label.Render(strings.ToUpper(m.t("tab.journal")))}
	h := m.bodyHeight() - 1
	start := max(0, m.ji-h+2)
	lastBatch := ""
	for i := start; i < len(m.journal) && len(lines) < h+1; i++ {
		e := m.journal[i]
		// A header per batch.
		if e.Batch != lastBatch {
			lastBatch = e.Batch
			lines = append(lines, m.st.label.Render(trunc(e.Title, m.w-4)))
		}
		mark := ""
		if e.Undone {
			mark = m.st.chipDim.Render(" " + m.t("undoneMark"))
		}
		if e.New {
			mark += m.st.chipInfo.Render(" " + m.t("newMark"))
		}
		row := fmt.Sprintf("%s  %-48s %s%s", e.Time.Format("02.01. 15:04"), trunc(e.Path, 48),
			m.st.add.Render(fmt.Sprintf("+%d", e.Added))+" "+m.st.del.Render(fmt.Sprintf("-%d", e.Removed)), mark)
		if i == m.ji {
			row = m.st.key.Render("▌ ") + row
		} else {
			row = "  " + row
		}
		lines = append(lines, row)
	}
	return strings.Join(lines, "\n")
}

// ---------- Settings ----------

func (m *Model) styleChoice(current string) string {
	var parts []string
	for _, s := range []string{styleSystem, styleKanteLight, styleKante} {
		label := m.t("style." + s)
		if s == current {
			parts = append(parts, m.st.bar.Render(" "+label+" "))
			continue
		}
		parts = append(parts, m.st.dim.Render(" "+label+" "))
	}
	return strings.Join(parts, "")
}

func (m *Model) selRow(selected bool, row string) string {
	if selected {
		return m.st.key.Render("▌ ") + row
	}
	return "  " + row
}

// ---------- Task ----------

func (m *Model) viewTask() string {
	lines := []string{m.st.title.Render(m.t("newTask")), ""}
	lines = append(lines, m.task.View(), "")
	lines = append(lines, m.st.label.Render(strings.ToUpper(m.t("scope")))+m.st.dim.Render("  "+m.t("scopeAll")))
	// The folders as a list with boxes; sub folders indented.
	for i, f := range m.status.Scopes {
		box := "[ ]"
		switch {
		case m.scopeSel[f]:
			box = m.st.key.Render("[x]")
		case m.scopeCovered(f):
			box = m.st.dim.Render("[x]")
		}
		depth := strings.Count(f, "/")
		name := strings.Repeat("  ", depth) + path.Base(f)
		row := box + " " + name
		if m.taskField == 1 && i == m.scopeRow {
			row = m.st.key.Render("▌") + row
		} else {
			row = " " + row
		}
		lines = append(lines, row)
	}

	prov := ""
	if m.taskProv < len(m.status.Providers) {
		p := m.status.Providers[m.taskProv]
		prov = p.Name + " · " + p.Model
	}
	lines = append(lines, "", m.st.label.Render(strings.ToUpper(m.t("provider"))+"  ")+m.st.strong.Render(prov)+m.st.dim.Render("  (Ctrl+P)"))
	find := "○ "
	if m.findOnly {
		find = "● "
	}
	lines = append(lines, m.st.label.Render(strings.ToUpper(m.t("findOnly"))+"  ")+m.st.strong.Render(find)+m.st.dim.Render("  (Ctrl+F) · "+m.t("templates")+" (Ctrl+T)"))
	if e := m.estimate; e != nil {
		est := m.t("estimate", (e.Tokens+500)/1000)
		if e.HasPrice {
			est += " · " + m.t("estimateCost", e.Cost, e.Currency)
		}
		if len(e.Blocked) > 0 {
			est += " · " + m.st.chipWarn.Render(m.t("localBlocked", len(e.Blocked)))
		}
		lines = append(lines, "", m.st.muted.Render(est))
	}
	action := m.t("createBatch")
	if m.findOnly {
		action = m.t("findNotes")
	}
	lines = append(lines, "", m.st.key.Render("Ctrl+S ")+m.st.strong.Render(action))
	return strings.Join(lines, "\n")
}
