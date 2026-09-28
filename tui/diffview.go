package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"

	"git.arianw.de/shrippen/hansei/core/diff"
	"git.arianw.de/shrippen/hansei/core/service"
)

const (
	numWidth   = 4
	gutter     = numWidth + 3 // "1234 + "
	fillerRune = "╱"
)

// piece is styled text of a known display width.
type piece struct {
	text  string
	style lipgloss.Style
}

// renderDiff turns rows into terminal lines. In split mode every row is wrapped so both
// sides get the same height; a side without a line gets a hatched filler. That keeps old and
// new locked to each other while scrolling, deletions included.
func (m *Model) renderDiff(f *service.FileView, rows []diff.Row, width int, withHeaders bool) ([]string, []int) {
	var lines []string
	hunkAt := make([]int, len(f.Hunks))
	half := (width - 1) / 2

	// Decided changes fold to their header, with the context between folded neighbours.
	prev := make([]int, len(rows))
	next := make([]int, len(rows))
	last := -1
	for i, r := range rows {
		if r.Kind == diff.RowHunk {
			last = r.Hunk
		}
		prev[i] = last
	}
	last = -1
	for i := len(rows) - 1; i >= 0; i-- {
		if rows[i].Kind == diff.RowHunk {
			last = rows[i].Hunk
		}
		next[i] = last
	}
	folded := func(i int, r diff.Row) bool {
		if !withHeaders || r.Kind == diff.RowHunk {
			return false
		}
		if r.Hunk >= 0 {
			return m.collapsed(f, r.Hunk)
		}
		p, n := prev[i], next[i]
		return (p >= 0 || n >= 0) && (p < 0 || m.collapsed(f, p)) && (n < 0 || m.collapsed(f, n))
	}

	for i, r := range rows {
		if folded(i, r) {
			continue
		}
		switch r.Kind {
		case diff.RowGap:
			lines = append(lines, m.center(m.st.dim.Render(m.t("gap", r.Gap)), width))
		case diff.RowHunk:
			if r.Hunk < len(hunkAt) {
				hunkAt[r.Hunk] = len(lines)
			}
			if withHeaders && r.Hunk < len(f.Hunks) {
				lines = append(lines, m.hunkHeader(f, r.Hunk, width)...)
			}
		default:
			if f.Mode == string(diff.ModeUnified) {
				lines = append(lines, m.unifiedRow(r, width)...)
				continue
			}
			left := m.side(r, true, half)
			right := m.side(r, false, width-1-half)
			h := max(len(left), len(right))
			for i := 0; i < h; i++ {
				lines = append(lines, pad(at(left, i, m.blank(r, true, half)), half)+m.st.border.Render("│")+at(right, i, m.blank(r, false, width-1-half)))
			}
		}
	}
	return lines, hunkAt
}

// hunkHeader is the bar above each hunk: number, heading, reason and state.
func (m *Model) hunkHeader(f *service.FileView, i, width int) []string {
	h := f.Hunks[i]
	focused := i == m.hunk
	mark := m.st.dim.Render("  ")
	if focused {
		mark = m.st.key.Render("▌ ")
	}
	state := m.stateChip(h.State)
	title := m.st.strong.Render(m.t("hunk", i+1, len(f.Hunks)))
	if h.Heading != "" {
		title += m.st.dim.Render(" · " + h.Heading)
	}
	if h.Feedback > 0 {
		title += m.st.chipInfo.Render(fmt.Sprintf(" · ◆%d", h.Feedback))
	}
	head := mark + title
	gap := width - lipgloss.Width(head) - lipgloss.Width(state)
	out := []string{head + strings.Repeat(" ", max(1, gap)) + state}

	if m.collapsed(f, i) {
		return out
	}
	why := h.Reason
	if h.Rule != "" {
		why = strings.TrimSpace(why + " · " + h.Rule + " (i)")
	}
	if h.Rejected != "" {
		why = strings.TrimSpace(why + " · ✗ " + h.Rejected)
	}
	if why != "" {
		for _, l := range wrap(why, width-4) {
			out = append(out, "    "+m.st.muted.Render(l))
		}
	}
	return out
}

func (m *Model) stateChip(state string) string {
	label := m.t("state." + state)
	switch state {
	case "accepted":
		return m.st.chipOK.Render("● " + label)
	case "rejected":
		return m.st.chipFail.Render("✗ " + label)
	}
	return m.st.chipDim.Render("○ " + label)
}

// side renders one half of a split row, wrapped to width.
func (m *Model) side(r diff.Row, old bool, width int) []string {
	no, segs, sign := r.NewNo, r.New, " "
	plain, word := m.st.base, m.st.base
	switch {
	case old && (r.Kind == diff.RowDelete || r.Kind == diff.RowChange):
		sign, plain, word = "-", m.st.del, m.st.delWord
	case !old && (r.Kind == diff.RowInsert || r.Kind == diff.RowChange):
		sign, plain, word = "+", m.st.add, m.st.addWord
	case r.Kind == diff.RowContext:
		plain = m.st.muted
	}
	if old {
		no, segs = r.OldNo, r.Old
	}
	if no == 0 {
		return nil
	}

	var ps []piece
	for _, s := range segs {
		st := plain
		if s.Changed && r.Kind == diff.RowChange {
			st = word
		}
		ps = append(ps, piece{s.Text, st})
	}
	prefix := m.st.dim.Render(fmt.Sprintf("%*d ", numWidth, no)) + plain.Render(sign) + " "
	return wrapPieces(ps, width-gutter, prefix, strings.Repeat(" ", gutter))
}

// blank is the filler for a missing side: hatched for deletions/insertions, empty otherwise.
func (m *Model) blank(r diff.Row, old bool, width int) string {
	missing := (old && r.Kind == diff.RowInsert) || (!old && r.Kind == diff.RowDelete)
	if !missing {
		return strings.Repeat(" ", width)
	}
	return m.st.filler.Render(strings.Repeat(fillerRune, width))
}

func (m *Model) unifiedRow(r diff.Row, width int) []string {
	var out []string
	emit := func(no int, sign string, segs []diff.Seg, plain, word lipgloss.Style) {
		var ps []piece
		for _, s := range segs {
			st := plain
			if s.Changed {
				st = word
			}
			ps = append(ps, piece{s.Text, st})
		}
		prefix := m.st.dim.Render(fmt.Sprintf("%*d ", numWidth, no)) + plain.Render(sign) + " "
		out = append(out, wrapPieces(ps, width-gutter, prefix, strings.Repeat(" ", gutter))...)
	}
	switch r.Kind {
	case diff.RowDelete:
		emit(r.OldNo, "-", r.Old, m.st.del, m.st.delWord)
	case diff.RowInsert:
		emit(r.NewNo, "+", r.New, m.st.add, m.st.addWord)
	case diff.RowContext:
		emit(r.NewNo, " ", r.New, m.st.muted, m.st.muted)
	}
	return out
}

// wrapPieces breaks styled pieces into lines of width cells; the first line gets prefix.
func wrapPieces(ps []piece, width int, prefix, indent string) []string {
	if width < 4 {
		width = 4
	}
	var lines []string
	var cur strings.Builder
	used := 0
	flush := func() {
		p := indent
		if len(lines) == 0 {
			p = prefix
		}
		lines = append(lines, p+cur.String()+strings.Repeat(" ", max(0, width-used)))
		cur.Reset()
		used = 0
	}
	for _, p := range ps {
		text := strings.ReplaceAll(p.text, "\t", "    ")
		for text != "" {
			room := width - used
			if room <= 0 {
				flush()
				room = width
			}
			part := runewidth.Truncate(text, room, "")
			if part == "" { // a wide rune that does not fit
				flush()
				continue
			}
			cur.WriteString(p.style.Render(part))
			used += runewidth.StringWidth(part)
			text = text[len(part):]
		}
	}
	if used > 0 || len(lines) == 0 {
		flush()
	}
	return lines
}

// wrap breaks plain text at spaces.
func wrap(s string, width int) []string {
	if width < 8 {
		width = 8
	}
	var out []string
	var line string
	for _, w := range strings.Fields(s) {
		if line == "" {
			line = w
			continue
		}
		if runewidth.StringWidth(line+" "+w) > width {
			out = append(out, line)
			line = w
			continue
		}
		line += " " + w
	}
	if line != "" {
		out = append(out, line)
	}
	for i, l := range out {
		if runewidth.StringWidth(l) > width {
			out[i] = runewidth.Truncate(l, width, "…")
		}
	}
	return out
}

func at(lines []string, i int, fallback string) string {
	if i < len(lines) {
		return lines[i]
	}
	return fallback
}

// pad fills a styled string with spaces to width cells (or cuts it).
func pad(s string, width int) string {
	w := lipgloss.Width(s)
	if w >= width {
		return s
	}
	return s + strings.Repeat(" ", width-w)
}

func (m *Model) center(s string, width int) string {
	w := lipgloss.Width(s)
	if w >= width {
		return s
	}
	left := (width - w) / 2
	return strings.Repeat(" ", left) + s + strings.Repeat(" ", width-w-left)
}

func trunc(s string, width int) string {
	if width <= 0 {
		return ""
	}
	return runewidth.Truncate(s, width, "…")
}
