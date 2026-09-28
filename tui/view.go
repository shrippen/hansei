package tui

import (
	"fmt"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"git.arianw.de/shrippen/hansei/core/batch"
	"git.arianw.de/shrippen/hansei/core/service"
)

const (
	leftWidth   = 30
	rightWidth  = 38
	minForLeft  = 96
	minForRight = 136
	headerLines = 1
	footerLines = 2
)

// View draws the screen.
func (m *Model) View() string {
	if m.w == 0 {
		return ""
	}
	m.hits = m.hits[:0]
	var body string
	switch {
	case m.help:
		body = m.viewHelp()
	case m.compare != nil:
		body = m.viewCompare()
	case m.done != nil:
		body = m.viewDone()
	default:
		switch m.screen {
		case scrStart:
			body = m.viewStart()
		case scrReview:
			body = m.viewReview()
		case scrBoard:
			body = m.viewBoard()
		case scrJournal:
			body = m.viewJournal()
		case scrSettings:
			body = m.viewSettings()
		case scrTask:
			body = m.viewTask()
		}
	}
	body = fit(body, m.w, m.bodyHeight())
	return lipgloss.JoinVertical(lipgloss.Left, m.viewHeader(), body, m.viewFooter())
}

func (m *Model) bodyHeight() int { return max(3, m.h-headerLines-footerLines) }

// fit pads or cuts a block to exactly w×h cells.
func fit(s string, w, h int) string {
	lines := strings.Split(s, "\n")
	if len(lines) > h {
		lines = lines[:h]
	}
	for i, l := range lines {
		if lipgloss.Width(l) > w {
			lines[i] = trunc(stripToWidth(l, w), w)
			continue
		}
		lines[i] = pad(l, w)
	}
	for len(lines) < h {
		lines = append(lines, strings.Repeat(" ", w))
	}
	return strings.Join(lines, "\n")
}

// stripToWidth cuts a styled line to width cells, keeping styles.
func stripToWidth(s string, w int) string {
	return lipgloss.NewStyle().MaxWidth(w).Render(s)
}

func (m *Model) viewHeader() string {
	tabs := []struct {
		s   screen
		key string
	}{{scrReview, "tab.review"}, {scrBoard, "tab.board"}, {scrStart, "tab.start"}, {scrJournal, "tab.journal"}, {scrSettings, "tab.settings"}}
	out := m.st.bar.Render(" HANSEI ") + " "
	for _, t := range tabs {
		label := " " + m.t(t.key) + " "
		if m.screen == t.s || (m.screen == scrTask && t.s == m.back) {
			out += m.st.strong.Underline(true).Render(label)
			continue
		}
		out += m.st.dim.Render(label)
	}
	right := m.providerLine()
	if m.status.Demo {
		right = "DEMO · " + right
	}
	room := m.w - lipgloss.Width(out) - 2
	if room < 8 {
		return stripToWidth(out, m.w)
	}
	right = trunc(right, room)
	gap := m.w - lipgloss.Width(out) - lipgloss.Width(right) - 1
	return out + strings.Repeat(" ", max(1, gap)) + m.st.dim.Render(right)
}

func (m *Model) providerLine() string {
	for _, p := range m.status.Providers {
		if !p.Default {
			continue
		}
		line := p.Name + " · " + p.Model
		if m.batch != nil && m.screen == scrReview && m.usageOf(m.batch).In+m.usageOf(m.batch).Out > 0 {
			u := m.usageOf(m.batch)
			line += " · " + m.t("cost", (u.In+u.Out+500)/1000)
			if u.Cost > 0 {
				line += fmt.Sprintf(" · %.2f %s", u.Cost, p.Currency)
			}
		}
		return line
	}
	return ""
}

func (m *Model) viewFooter() string {
	var top string
	switch {
	case m.inKind != inputNone:
		top = m.viewInput()
	case m.err != "":
		top = m.st.chipFail.Render("✗ " + m.err)
	case m.toast != "" && time.Now().Before(m.toastUntil):
		top = m.st.chipOK.Render("✓ " + m.toast)
	case m.reveal:
		top = m.st.chipWarn.Render("⚠ " + m.t("revealOn"))
	}
	keys := ""
	switch {
	case m.inKind == inputReason:
		keys = m.t("keys.reason")
	case m.inKind != inputNone:
		keys = m.t("keys.input")
	default:
		keys = map[screen]string{scrReview: "keys.review", scrBoard: "keys.board", scrStart: "keys.start",
			scrJournal: "keys.journal", scrSettings: "keys.settings", scrTask: "keys.task"}[m.screen]
		keys = m.t(keys)
	}
	return pad(trunc(top, m.w), m.w) + "\n" + m.st.dim.Render(trunc(keys, m.w))
}

func (m *Model) viewInput() string {
	var label string
	switch m.inKind {
	case inputFeedback:
		target := m.t("scope." + fbScopes[m.fbScope])
		label = m.st.chipInfo.Render(m.t("prompt.fb", target))
		if m.remember {
			label += m.st.chipWarn.Render(" · " + m.t("remember"))
		}
	case inputReason:
		label = m.st.chipFail.Render(m.t("prompt.reason"))
		if m.regen {
			label += m.st.chipInfo.Render(" · " + m.t("regen"))
		}
	case inputKey:
		label = m.st.chipWarn.Render(m.t("prompt.key", m.keyFor))
	case inputSetting:
		label = m.st.chipInfo.Render(m.settingPrompt())
	}
	if m.quick != "" {
		label += m.st.key.Render(" [" + m.t("quick."+quickIndex(m.quick)) + "]")
	}
	m.input.Width = max(10, m.w-lipgloss.Width(label)-4)
	return label + " " + m.input.View()
}

// ---------- Review ----------

func (m *Model) viewReview() string {
	h := m.bodyHeight()
	lw, rw := 0, 0
	if m.w >= minForLeft {
		lw = leftWidth
	}
	if m.w >= minForRight {
		rw = rightWidth
	}
	cw := m.w - lw - rw
	if lw > 0 {
		cw--
	}
	if rw > 0 {
		cw--
	}

	center := m.viewDiffPane(cw, h, lw+min(lw, 1))
	parts := []string{}
	sep := m.st.border.Render(strings.TrimSuffix(strings.Repeat("│\n", h), "\n"))
	if lw > 0 {
		parts = append(parts, fit(m.viewBatchList(lw, h), lw, h), sep)
	}
	parts = append(parts, fit(center, cw, h))
	if rw > 0 {
		parts = append(parts, sep, fit(m.viewThread(rw, h), rw, h))
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, parts...)
}

func (m *Model) viewBatchList(w, h int) string {
	var b strings.Builder
	b.WriteString(m.st.label.Render(strings.ToUpper(m.t("batches"))) + "\n")
	list := m.reviewable()
	if len(list) == 0 {
		for _, l := range wrap(m.t("noBatches"), w-1) {
			b.WriteString(m.st.dim.Render(l) + "\n")
		}
		return b.String()
	}
	y := headerLines + 1
	for _, bs := range list {
		active := bs.ID == m.batchID
		c := bs.Counts
		mark := "▸ "
		if active {
			mark = m.st.key.Render("▾ ")
		}
		topic := m.st.chipTag.Render(trunc(bs.Topic, 12))
		prog := " " + m.t("progressShort", c.Done, c.Files, c.Accepted+c.Rejected, c.Hunks)
		b.WriteString(mark + topic + m.st.dim.Render(prog) + "\n")
		m.addHit(hit{y: y, x0: 0, x1: w, batch: bs.ID, hunk: -1})
		m.addHit(hit{y: y + 1, x0: 0, x1: w, batch: bs.ID, hunk: -1})
		y += 3
		title := trunc(bs.Title, w-3)
		if active {
			b.WriteString("  " + m.st.strong.Render(title) + "\n")
		} else {
			b.WriteString("  " + m.st.muted.Render(title) + "\n")
		}
		b.WriteString("  " + m.progressBar(c, w-4) + "\n")
		if bs.Revising || bs.Running {
			b.WriteString("  " + m.st.chipWarn.Render(trunc(m.t("working", m.progress[bs.ID]), w-3)) + "\n")
			y++
		}
		if !active || m.batch == nil {
			continue
		}
		for _, f := range m.batch.Files {
			icon, st := m.fileIcon(f)
			name := trunc(path.Base(f.Path), w-5)
			line := "  " + st.Render(icon) + " " + name
			if f.Path == m.path {
				line = "  " + st.Render(icon) + " " + m.st.sel.Render(name)
			}
			b.WriteString(line + "\n")
			m.addHit(hit{y: y, x0: 0, x1: w, path: f.Path, hunk: -1})
			y++
		}
	}
	return b.String()
}

func (m *Model) fileIcon(f service.FileSummary) (string, lipgloss.Style) {
	switch {
	case f.Status == "applied":
		return "●", m.st.chipOK
	case f.Status == "skipped" || f.Status == "undone":
		return "–", m.st.chipDim
	case f.Status == "stale":
		return "!", m.st.chipWarn
	case f.Feedback:
		return "◆", m.st.chipInfo
	}
	return "○", m.st.chipDim
}

// progressBar shows accepted (aqua), rejected (red) and open hunks.
func (m *Model) progressBar(c batch.Counts, w int) string {
	if c.Hunks == 0 || w <= 0 {
		return m.st.filler.Render(strings.Repeat("░", max(0, w)))
	}
	acc := c.Accepted * w / c.Hunks
	rej := c.Rejected * w / c.Hunks
	rest := max(0, w-acc-rej)
	return m.st.chipOK.Render(strings.Repeat("█", acc)) + m.st.chipFail.Render(strings.Repeat("█", rej)) + m.st.filler.Render(strings.Repeat("░", rest))
}

func (m *Model) viewDiffPane(w, h, x0 int) string {
	if m.batch == nil {
		return m.st.dim.Render(strings.Join(wrap(m.t("noBatches"), w), "\n"))
	}
	if m.file == nil {
		return m.viewBatchIntro(w)
	}
	f := m.file

	// Title line: file, folder, version, layout.
	title := m.st.title.Render(path.Base(f.Path)) + " " + m.st.dim.Render(path.Dir(f.Path)+"/")
	info := m.t("version", f.Current, len(f.Versions)) + " · " + f.Mode
	if len(f.Hunks) > 0 {
		info = m.t("hunk", m.hunk+1, len(f.Hunks)) + " · " + info
	}
	head := title + strings.Repeat(" ", max(1, w-lipgloss.Width(title)-lipgloss.Width(info))) + m.st.dim.Render(info)
	lines := []string{head}
	lines = append(lines, m.questionLines(w)...)
	// The rule behind a change (i) sits in a box above the diff.
	if m.rule != nil {
		lines = append(lines, m.ruleBox(w)...)
	}
	if f.Stale {
		lines = append(lines, m.st.chipWarn.Render("⚠ "+m.t("stale")))
	}
	if f.Status != "open" {
		lines = append(lines, m.st.chipOK.Render("● "+m.t("file."+f.Status)))
	}
	if f.Summary != "" {
		for _, l := range wrap(f.Summary, w) {
			lines = append(lines, m.st.muted.Render(l))
		}
	}
	lines = append(lines, m.st.border.Render(strings.Repeat("─", w)))

	diffLines, at := m.renderDiff(f, f.Rows, w, true)
	avail := h - len(lines)
	maxScroll := max(0, len(diffLines)-avail)
	m.scroll = clamp(m.scroll, 0, maxScroll)
	end := min(len(diffLines), m.scroll+avail)
	// Change headers are clickable.
	for i, a := range at {
		if a >= m.scroll && a < end {
			m.addHit(hit{y: headerLines + len(lines) + a - m.scroll, x0: x0, x1: x0 + w, hunk: i})
		}
	}
	lines = append(lines, diffLines[m.scroll:end]...)
	return strings.Join(lines, "\n")
}

// scrollToHunk moves the viewport so the current hunk header sits near the top.
func (m *Model) scrollToHunk() {
	if m.file == nil || len(m.file.Hunks) == 0 {
		return
	}
	_, at := m.renderDiff(m.file, m.file.Rows, max(40, m.w-leftWidth-rightWidth-2), true)
	if m.hunk < len(at) {
		m.scroll = max(0, at[m.hunk]-3)
	}
}

func (m *Model) viewBatchIntro(w int) string {
	b := m.batch
	var lines []string
	lines = append(lines, m.st.title.Render(b.Title))
	if b.Error != "" {
		lines = append(lines, m.st.chipFail.Render(b.Error))
	}
	if b.Running || b.Status == "working" {
		lines = append(lines, m.st.chipWarn.Render(m.t("working", m.progress[b.ID])))
	}
	for _, l := range wrap(b.Summary, w) {
		lines = append(lines, m.st.muted.Render(l))
	}
	lines = append(lines, m.questionLines(w)...)
	// A find-only task lists the notes it found.
	for _, f := range b.Found {
		lines = append(lines, "  "+m.st.strong.Render(trunc(f.Path, w/2))+"  "+m.st.dim.Render(trunc(f.Reason, max(10, w/2-4))))
	}
	return strings.Join(lines, "\n")
}

func (m *Model) viewThread(w, h int) string {
	var lines []string
	scope := []string{}
	for i, s := range fbScopes {
		label := m.t("scope." + s)
		if m.inKind == inputFeedback && i == m.fbScope {
			label = m.st.bar.Render(" " + label + " ")
		}
		scope = append(scope, label)
	}
	lines = append(lines, m.st.label.Render(strings.ToUpper(m.t("feedback"))))
	lines = append(lines, m.st.dim.Render(strings.Join(scope, " ▸ ")))
	if m.batch == nil {
		return strings.Join(lines, "\n")
	}

	var body []string
	for _, msg := range m.batch.Thread {
		if msg.Role == "system" {
			body = append(body, m.systemLines(msg, w)...)
			continue
		}
		who, st := m.t("you"), m.st.chipInfo
		if msg.Role != "user" {
			who, st = m.t("ai"), m.st.key
		}
		text := msg.Text
		if msg.Quick != "" {
			text = "[" + m.t("quick."+quickIndex(msg.Quick)) + "] " + text
		}
		wrapped := wrap(text, w-4)
		for i, l := range wrapped {
			prefix := "   "
			if i == 0 {
				prefix = st.Render(fmt.Sprintf("%-3s", who))
			}
			style := m.st.base
			if msg.Question && !msg.Answered {
				style = m.st.chipWarn
			}
			body = append(body, prefix+style.Render(l))
		}
	}
	if t := m.aiText[m.batchID]; t != "" {
		body = append(body, m.st.key.Render(m.t("aiWriting")))
		for _, l := range wrap(t, w-4) {
			body = append(body, "   "+m.st.dim.Render(l))
		}
	}
	for _, s := range m.batch.Suggestions {
		if s.Status == "open" {
			for _, l := range wrap(m.t("suggestion", s.Text), w-2) {
				body = append(body, m.st.chipWarn.Render(l))
			}
		}
	}

	// Keep the latest messages visible.
	room := h - len(lines) - 1
	if len(body) > room {
		body = body[len(body)-room:]
	}
	lines = append(lines, body...)
	if m.inKind == inputNone {
		for len(lines) < h-1 {
			lines = append(lines, "")
		}
		lines = append(lines, m.st.dim.Render(trunc(m.t("quick"), w)))
	}
	return strings.Join(lines, "\n")
}

// ---------- Overlays ----------

func (m *Model) viewCompare() string {
	c := m.compare
	f := m.file
	if m.compareFile != nil {
		f = m.compareFile
	}
	if f == nil {
		return ""
	}
	title := m.t("compare", c.From, c.To)
	if m.compareTitle != "" {
		title = m.compareTitle + m.st.dim.Render("  (Esc)")
	}
	lines := []string{m.st.title.Render(title), ""}
	dl, _ := m.renderDiff(f, c.Rows, m.w, false)
	return strings.Join(append(lines, dl...), "\n")
}

func (m *Model) viewDone() string {
	d := m.done
	c := d.Batch.Counts
	rounds := 0
	if m.batch != nil {
		for _, msg := range m.batch.Thread {
			if msg.Role == "user" {
				rounds++
			}
		}
	}
	box := lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(m.st.p.accent).Padding(1, 3)
	content := strings.Join([]string{
		m.st.chipOK.Render("● " + m.t("doneTitle")),
		"",
		m.st.title.Render(d.Batch.Title),
		"",
		m.t("doneStats2", m.n("doneFiles", c.Files), c.Accepted, c.Rejected, m.n("rounds", rounds)),
		m.st.dim.Render(m.doneLine(d.Batch)),
		"",
		m.st.dim.Render(m.t("doneKeys")),
	}, "\n")
	return lipgloss.Place(m.w, m.bodyHeight(), lipgloss.Center, lipgloss.Center, box.Render(content))
}

func (m *Model) viewHelp() string {
	// The current screen's keys first, then the rest, each group with its title.
	groups := []struct{ title, keys string }{
		{"help.review", "keys.review"}, {"help.board", "keys.board"}, {"help.start", "keys.start"},
		{"help.journal", "keys.journal"}, {"help.settings", "keys.settings"}, {"help.task", "keys.task"},
		{"help.input", "keys.input"}, {"help.reason", "keys.reason"}, {"help.global", "keys.global"},
	}
	current := map[screen]string{scrReview: "keys.review", scrBoard: "keys.board", scrStart: "keys.start",
		scrJournal: "keys.journal", scrSettings: "keys.settings", scrTask: "keys.task"}[m.screen]
	sort.SliceStable(groups, func(i, j int) bool { return groups[i].keys == current && groups[j].keys != current })
	lines := []string{m.st.title.Render(m.t("help")) + m.st.dim.Render("  (Esc)"), ""}
	for _, g := range groups {
		lines = append(lines, m.st.key.Render(strings.ToUpper(m.t(g.title))))
		for _, part := range strings.Split(m.t(g.keys), " · ") {
			k, v, _ := strings.Cut(part, " ")
			lines = append(lines, "  "+m.st.strong.Render(fmt.Sprintf("%-9s", k))+" "+m.st.base.Render(v))
		}
		lines = append(lines, "")
	}
	return columns(lines, m.w, m.bodyHeight())
}

// columns flows lines into as many columns as needed.
func columns(lines []string, w, h int) string {
	if len(lines) <= h {
		return strings.Join(lines, "\n")
	}
	n := (len(lines) + h - 1) / h
	cw := w / n
	var cols []string
	for i := 0; i < n; i++ {
		part := lines[i*h : min(len(lines), (i+1)*h)]
		cols = append(cols, fit(strings.Join(part, "\n"), cw, h))
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, cols...)
}
