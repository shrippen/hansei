package diff

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"unicode"
)

// Mode selects the row layout.
type Mode string

const (
	ModeSplit   Mode = "split"   // side by side, one row per aligned line pair
	ModeUnified Mode = "unified" // git style
)

// RowKind tells a UI how to draw a row.
type RowKind string

const (
	RowContext RowKind = "ctx"    // unchanged line, both sides
	RowChange  RowKind = "change" // changed line pair, both sides with word spans
	RowDelete  RowKind = "del"    // line only on the left; the right side is a filler
	RowInsert  RowKind = "add"    // line only on the right; the left side is a filler
	RowGap     RowKind = "gap"    // hidden unchanged lines
	RowHunk    RowKind = "hunk"   // header before each hunk
)

// AllContext shows the whole file instead of a few lines around each hunk.
const AllContext = -1

const idBytes = 5

// ErrConflict means a hunk cannot be placed in a note that changed since the proposal.
var ErrConflict = errors.New("diff: note changed, hunk no longer fits")

// Seg is a piece of a line; Changed marks the words that differ.
type Seg struct {
	Text    string `json:"t"`
	Changed bool   `json:"c,omitempty"`
}

// Row is one line of the diff view. In split mode both sides share the row,
// so both panes scroll together even across deletions (fillers keep them aligned).
type Row struct {
	Kind  RowKind `json:"kind"`
	Hunk  int     `json:"hunk"`
	OldNo int     `json:"oldNo,omitempty"`
	NewNo int     `json:"newNo,omitempty"`
	Old   []Seg   `json:"old,omitempty"`
	New   []Seg   `json:"new,omitempty"`
	Gap   int     `json:"gap,omitempty"`
}

// Hunk is one contiguous change.
type Hunk struct {
	ID       string   `json:"id"`
	Index    int      `json:"index"`
	OldStart int      `json:"oldStart"` // 0-based line index in the old text
	OldLines []string `json:"oldLines"`
	NewStart int      `json:"newStart"`
	NewLines []string `json:"newLines"`
	Before   string   `json:"-"` // line before the hunk in the old text, for rebasing
	After    string   `json:"-"` // line after the hunk in the old text
	HasPrev  bool     `json:"-"`
	HasNext  bool     `json:"-"`
	Heading  string   `json:"heading"`
}

// Result is the full comparison of two texts.
type Result struct {
	Hunks []Hunk
	old   []string
	new   []string
	edits []edit
	oldNL bool
}

// Compare diffs old against new.
func Compare(oldText, newText string) *Result {
	a, oldNL := splitLines(oldText)
	b, _ := splitLines(newText)
	r := &Result{old: a, new: b, edits: myers(a, b), oldNL: oldNL}
	r.Hunks = r.hunks()
	return r
}

// Changed reports whether the texts differ in any line.
func (r *Result) Changed() bool { return len(r.Hunks) > 0 }

// hunks groups the edit script into runs of non-equal steps.
func (r *Result) hunks() []Hunk {
	var out []Hunk
	seen := map[string]int{}
	front := frontEnd(r.old)

	for i := 0; i < len(r.edits); {
		if r.edits[i].kind == opEqual {
			i++
			continue
		}
		start := i
		for i < len(r.edits) && r.edits[i].kind != opEqual {
			i++
		}
		h := Hunk{Index: len(out)}
		h.OldStart, h.NewStart = r.positions(start)
		for _, e := range r.edits[start:i] {
			if e.kind == opDelete {
				h.OldLines = append(h.OldLines, r.old[e.a])
				continue
			}
			h.NewLines = append(h.NewLines, r.new[e.b])
		}
		if h.OldStart > 0 {
			h.Before, h.HasPrev = r.old[h.OldStart-1], true
		}
		if end := h.OldStart + len(h.OldLines); end < len(r.old) {
			h.After, h.HasNext = r.old[end], true
		}
		h.Heading = heading(r.old, h.OldStart, front)
		h.ID = hunkID(h, seen)
		out = append(out, h)
	}
	return out
}

// positions returns the old/new line index where the edit at i begins.
func (r *Result) positions(i int) (int, int) {
	oldPos, newPos := 0, 0
	for _, e := range r.edits[:i] {
		switch e.kind {
		case opEqual:
			oldPos, newPos = e.a+1, e.b+1
		case opDelete:
			oldPos = e.a + 1
		case opInsert:
			newPos = e.b + 1
		}
	}
	return oldPos, newPos
}

// hunkID is stable across versions as long as the change and its anchor line stay the same,
// so a decision on a hunk survives a revision that leaves the hunk untouched.
func hunkID(h Hunk, seen map[string]int) string {
	sum := sha256.Sum256([]byte(h.Before + "\x1e" + strings.Join(h.OldLines, "\n") + "\x1f" + strings.Join(h.NewLines, "\n")))
	id := "h" + hex.EncodeToString(sum[:idBytes])
	seen[id]++
	if n := seen[id]; n > 1 {
		id = fmt.Sprintf("%s-%d", id, n)
	}
	return id
}

// Rows lays out the comparison for a UI. context is the number of unchanged lines
// around each hunk (AllContext shows everything).
func (r *Result) Rows(mode Mode, context int) []Row {
	visible := r.visibleEquals(context)
	var rows []Row
	hidden := 0
	hunk := 0

	flushGap := func() {
		if hidden > 0 {
			rows = append(rows, Row{Kind: RowGap, Hunk: -1, Gap: hidden})
			hidden = 0
		}
	}

	for i := 0; i < len(r.edits); {
		e := r.edits[i]
		if e.kind == opEqual {
			if !visible[i] {
				hidden++
				i++
				continue
			}
			flushGap()
			text := r.old[e.a]
			rows = append(rows, Row{Kind: RowContext, Hunk: -1, OldNo: e.a + 1, NewNo: e.b + 1, Old: plain(text), New: plain(text)})
			i++
			continue
		}

		// One change block becomes a header plus aligned rows.
		flushGap()
		start := i
		for i < len(r.edits) && r.edits[i].kind != opEqual {
			i++
		}
		rows = append(rows, Row{Kind: RowHunk, Hunk: hunk})
		rows = append(rows, r.blockRows(r.edits[start:i], hunk, mode)...)
		hunk++
	}
	flushGap()
	return rows
}

// blockRows pairs deleted and inserted lines of one block.
func (r *Result) blockRows(block []edit, hunk int, mode Mode) []Row {
	var dels, ins []edit
	for _, e := range block {
		if e.kind == opDelete {
			dels = append(dels, e)
			continue
		}
		ins = append(ins, e)
	}
	pairs := min(len(dels), len(ins))

	var rows []Row
	if mode == ModeUnified {
		oldSegs, newSegs := make([][]Seg, len(dels)), make([][]Seg, len(ins))
		for i := range dels {
			oldSegs[i] = plain(r.old[dels[i].a])
		}
		for i := range ins {
			newSegs[i] = plain(r.new[ins[i].b])
		}
		for i := 0; i < pairs; i++ {
			oldSegs[i], newSegs[i] = Words(r.old[dels[i].a], r.new[ins[i].b])
		}
		for i, e := range dels {
			rows = append(rows, Row{Kind: RowDelete, Hunk: hunk, OldNo: e.a + 1, Old: oldSegs[i]})
		}
		for i, e := range ins {
			rows = append(rows, Row{Kind: RowInsert, Hunk: hunk, NewNo: e.b + 1, New: newSegs[i]})
		}
		return rows
	}

	for i := 0; i < pairs; i++ {
		o, n := Words(r.old[dels[i].a], r.new[ins[i].b])
		rows = append(rows, Row{Kind: RowChange, Hunk: hunk, OldNo: dels[i].a + 1, NewNo: ins[i].b + 1, Old: o, New: n})
	}
	for _, e := range dels[pairs:] {
		rows = append(rows, Row{Kind: RowDelete, Hunk: hunk, OldNo: e.a + 1, Old: plain(r.old[e.a])})
	}
	for _, e := range ins[pairs:] {
		rows = append(rows, Row{Kind: RowInsert, Hunk: hunk, NewNo: e.b + 1, New: plain(r.new[e.b])})
	}
	return rows
}

// visibleEquals marks unchanged edits within context lines of a change.
func (r *Result) visibleEquals(context int) []bool {
	vis := make([]bool, len(r.edits))
	if context == AllContext {
		for i := range vis {
			vis[i] = true
		}
		return vis
	}
	for i, e := range r.edits {
		if e.kind == opEqual {
			continue
		}
		for j := i - 1; j >= 0 && j >= i-context && r.edits[j].kind == opEqual; j-- {
			vis[j] = true
		}
		for j := i + 1; j < len(r.edits) && j <= i+context && r.edits[j].kind == opEqual; j++ {
			vis[j] = true
		}
	}
	return vis
}

// Apply builds the new text from old, taking the new lines only for accepted hunks.
func (r *Result) Apply(accepted func(Hunk) bool) string {
	return r.ApplyWith(func(h Hunk) []string {
		if accepted(h) {
			return h.NewLines
		}
		return h.OldLines
	})
}

// ApplyWith builds a text from old where each hunk is replaced by the lines lines(h) returns.
func (r *Result) ApplyWith(lines func(Hunk) []string) string {
	var out []string
	h := 0
	for i := 0; i < len(r.edits); {
		if r.edits[i].kind == opEqual {
			out = append(out, r.old[r.edits[i].a])
			i++
			continue
		}
		for i < len(r.edits) && r.edits[i].kind != opEqual {
			i++
		}
		out = append(out, lines(r.Hunks[h])...)
		h++
	}
	return joinLines(out, r.oldNL || len(r.old) == 0)
}

// Rebase applies accepted hunks to current, a text that changed since the diff was made.
// Each hunk must still find its old lines (and anchor lines) exactly once.
func (r *Result) Rebase(current string, accepted func(Hunk) bool) (string, error) {
	lines, nl := splitLines(current)
	type place struct {
		at  int
		old int
		new []string
	}
	var places []place
	for _, h := range r.Hunks {
		if !accepted(h) {
			continue
		}
		at, err := locate(lines, h)
		if err != nil {
			return "", err
		}
		places = append(places, place{at, len(h.OldLines), h.NewLines})
	}

	// Apply from the bottom so earlier indexes stay valid.
	for i := len(places) - 1; i >= 0; i-- {
		p := places[i]
		if i > 0 && places[i-1].at+places[i-1].old > p.at {
			return "", ErrConflict
		}
		tail := append([]string(nil), lines[p.at+p.old:]...)
		lines = append(append(lines[:p.at], p.new...), tail...)
	}
	return joinLines(lines, nl), nil
}

// locate finds the unique position of a hunk's old lines (with anchors) in lines.
func locate(lines []string, h Hunk) (int, error) {
	found := -1
	for at := 0; at+len(h.OldLines) <= len(lines); at++ {
		if !matchAt(lines, h, at) {
			continue
		}
		if found >= 0 {
			return 0, ErrConflict
		}
		found = at
	}
	if found < 0 {
		return 0, ErrConflict
	}
	return found, nil
}

func matchAt(lines []string, h Hunk, at int) bool {
	for i, l := range h.OldLines {
		if lines[at+i] != l {
			return false
		}
	}
	if h.HasPrev && (at == 0 || lines[at-1] != h.Before) {
		return false
	}
	end := at + len(h.OldLines)
	if h.HasNext && (end >= len(lines) || lines[end] != h.After) {
		return false
	}
	if !h.HasPrev && at != 0 {
		return false
	}
	return true
}

// Words diffs two lines word by word. Lone punctuation changes are not highlighted, and a
// line without any word in common gets no word highlights at all (the line colour says it).
func Words(oldLine, newLine string) ([]Seg, []Seg) {
	oldSegs, newSegs := words(oldLine, newLine)
	if !sharesWord(oldSegs) {
		return plain(oldLine), plain(newLine)
	}
	return calm(oldSegs), calm(newSegs)
}

// sharesWord reports whether an unchanged segment contains a letter or digit.
func sharesWord(segs []Seg) bool {
	for _, s := range segs {
		if s.Changed {
			continue
		}
		for _, r := range s.Text {
			if unicode.IsLetter(r) || unicode.IsDigit(r) {
				return true
			}
		}
	}
	return false
}

// calm unmarks changed segments that are only punctuation or space, then merges neighbours.
func calm(segs []Seg) []Seg {
	var out []Seg
	for _, s := range segs {
		if s.Changed && !hasWord(s.Text) {
			s.Changed = false
		}
		out = addSeg(out, s.Text, s.Changed)
	}
	return out
}

func hasWord(s string) bool {
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return true
		}
	}
	return false
}

func words(oldLine, newLine string) ([]Seg, []Seg) {
	a, b := tokens(oldLine), tokens(newLine)
	var oldSegs, newSegs []Seg
	for _, e := range myers(a, b) {
		switch e.kind {
		case opEqual:
			oldSegs = addSeg(oldSegs, a[e.a], false)
			newSegs = addSeg(newSegs, b[e.b], false)
		case opDelete:
			oldSegs = addSeg(oldSegs, a[e.a], true)
		case opInsert:
			newSegs = addSeg(newSegs, b[e.b], true)
		}
	}
	return oldSegs, newSegs
}

func addSeg(segs []Seg, text string, changed bool) []Seg {
	// Whitespace between two changed words counts as changed, so highlights read as one phrase.
	if n := len(segs); n > 0 && segs[n-1].Changed == changed {
		segs[n-1].Text += text
		return segs
	}
	return append(segs, Seg{Text: text, Changed: changed})
}

// tokens splits a line into words, whitespace runs and single punctuation characters.
func tokens(s string) []string {
	var out []string
	var cur []rune
	kind := 0
	flush := func() {
		if len(cur) > 0 {
			out = append(out, string(cur))
			cur = cur[:0]
		}
	}
	for _, r := range s {
		k := 3
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_':
			k = 1
		case unicode.IsSpace(r):
			k = 2
		}
		if k != kind || k == 3 {
			flush()
		}
		kind = k
		cur = append(cur, r)
	}
	flush()
	return out
}

func plain(s string) []Seg {
	if s == "" {
		return nil
	}
	return []Seg{{Text: s}}
}

// splitLines returns the lines and whether the text ended with a newline.
func splitLines(s string) ([]string, bool) {
	if s == "" {
		return nil, false
	}
	s = strings.ReplaceAll(s, "\r\n", "\n")
	nl := strings.HasSuffix(s, "\n")
	s = strings.TrimSuffix(s, "\n")
	return strings.Split(s, "\n"), nl
}

func joinLines(lines []string, nl bool) string {
	if len(lines) == 0 {
		return ""
	}
	out := strings.Join(lines, "\n")
	if nl {
		out += "\n"
	}
	return out
}

// frontEnd returns the index after the frontmatter block, 0 without one.
func frontEnd(lines []string) int {
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return 0
	}
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			return i + 1
		}
	}
	return 0
}

// FrontmatterHeading labels hunks inside the frontmatter block.
const FrontmatterHeading = "---"

// heading finds the Markdown heading above line i.
func heading(lines []string, i, front int) string {
	if i < front {
		return FrontmatterHeading
	}
	for j := min(i, len(lines)-1); j >= front; j-- {
		t := strings.TrimSpace(lines[j])
		if strings.HasPrefix(t, "#") && strings.Contains(t, " ") && strings.Trim(strings.SplitN(t, " ", 2)[0], "#") == "" {
			return t
		}
	}
	return ""
}
