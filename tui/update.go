package tui

import (
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"git.arianw.de/shrippen/hansei/core/service"
)

// Update routes messages and keys.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		m.task.SetWidth(max(20, m.w-8))
		return m, nil
	case tickMsg:
		return m, tickCmd()
	case statusMsg:
		m.status = service.StatusView(msg)
		if m.status.Style != "" && m.status.Style != m.style {
			m.style, m.st = m.status.Style, newStyles(m.status.Style)
		}
		return m, nil
	case homeMsg:
		m.home = service.HomeView(msg)
		return m, nil
	case batchesMsg:
		m.batches = msg
		return m, m.pickBatch()
	case batchMsg:
		b := service.BatchView(msg)
		m.batch = &b
		return m, m.pickFile()
	case fileMsg:
		f := service.FileView(msg)
		keep := m.file != nil && m.file.Path == f.Path && m.file.Batch == f.Batch
		m.file = &f
		if !keep {
			m.hunk, m.scroll = m.firstOpenHunk(), 0
		}
		m.hunk = clamp(m.hunk, 0, len(f.Hunks)-1)
		m.scrollToHunk()
		return m, nil
	case journalMsg:
		m.journal = msg
		m.ji = clamp(m.ji, 0, len(m.journal)-1)
		return m, nil
	case settingsMsg:
		m.settings = service.SettingsView(msg)
		return m, nil
	case estimateMsg:
		e := service.Estimate(msg)
		m.estimate = &e
		return m, nil
	case compareMsg:
		c := service.CompareView(msg)
		m.compare, m.compareFile, m.compareTitle = &c, nil, ""
		return m, nil
	case journalDiffMsg:
		c := msg.cmp
		m.compare, m.compareTitle = &c, msg.title
		m.compareFile = &service.FileView{Mode: defaultMode}
		return m, nil
	case findingsMsg:
		f := service.FindingsView(msg)
		m.findings = &f
		return m, nil
	case ruleMsg:
		r := service.RuleSectionView(msg)
		m.rule = &r
		return m, nil
	case decideMsg:
		return m, m.afterDecide(service.DecideResult(msg))
	case openMsg:
		m.screen, m.batchID, m.path, m.file = scrReview, msg.id, msg.path, nil
		return m, tea.Batch(m.loadBatches(), m.loadBatch(msg.id))
	case toastMsg:
		m.setToast(string(msg))
		return m, nil
	case errMsg:
		m.err = msg.err.Error()
		m.setToast("")
		return m, nil
	case eventMsg:
		return m, tea.Batch(m.onEvent(service.Event(msg)), m.waitEvent())
	case tea.MouseMsg:
		return m, m.onMouse(msg)
	case tea.KeyMsg:
		return m, m.onKey(msg)
	}
	return m, nil
}

// onEvent refreshes what an event touches.
func (m *Model) onEvent(e service.Event) tea.Cmd {
	switch e.Kind {
	case service.EventProgress:
		m.progress[e.Batch] = e.Text
		return nil
	case service.EventAI:
		s := m.aiText[e.Batch] + e.Text
		if r := []rune(s); len(r) > aiTextKeep {
			s = string(r[len(r)-aiTextKeep:])
		}
		m.aiText[e.Batch] = s
		return nil
	case service.EventToast:
		m.setToast(e.Text)
		return nil
	case service.EventUsage:
		m.liveUsage(e)
		return nil
	case service.EventSettings:
		return tea.Batch(m.loadStatus(), m.loadSettings(), m.loadHome())
	case service.EventBatch:
		delete(m.progress, e.Batch)
		delete(m.live, e.Batch)
		cmds := []tea.Cmd{m.loadBatches(), m.loadHome()}
		if e.Batch == m.batchID {
			delete(m.aiText, e.Batch)
			cmds = append(cmds, m.loadBatch(e.Batch))
		}
		if m.screen == scrJournal {
			cmds = append(cmds, m.loadJournal())
		}
		return tea.Batch(cmds...)
	}
	return nil
}

// reviewable are the batches the review screen walks through.
func (m *Model) reviewable() []service.BatchSummary {
	var out []service.BatchSummary
	for _, b := range m.batches {
		if b.Column == "review" || b.Column == "feedback" || b.ID == m.batchID {
			out = append(out, b)
		}
	}
	return out
}

// pickBatch keeps the current batch or selects the first waiting one.
func (m *Model) pickBatch() tea.Cmd {
	list := m.reviewable()
	for _, b := range list {
		if b.ID == m.batchID {
			return nil
		}
	}
	if len(list) == 0 {
		m.batchID, m.batch, m.path, m.file = "", nil, "", nil
		return nil
	}
	m.batchID, m.path, m.file = list[0].ID, "", nil
	return m.loadBatch(m.batchID)
}

// pickFile keeps the current file or selects the first open one.
func (m *Model) pickFile() tea.Cmd {
	if m.batch == nil || len(m.batch.Files) == 0 {
		m.path, m.file = "", nil
		return nil
	}
	for _, f := range m.batch.Files {
		if f.Path == m.path {
			return m.loadFile()
		}
	}
	m.path = m.batch.Files[0].Path
	for _, f := range m.batch.Files {
		if f.Status == "open" || f.Status == "stale" {
			m.path = f.Path
			break
		}
	}
	return m.loadFile()
}

func (m *Model) firstOpenHunk() int {
	if m.file == nil {
		return 0
	}
	for i, h := range m.file.Hunks {
		if h.State == "pending" {
			return i
		}
	}
	return 0
}

func (m *Model) afterDecide(r service.DecideResult) tea.Cmd {
	if r.Message != "" {
		m.setToast(r.Message)
	}
	if r.Done {
		m.done = &r
	}
	cmds := []tea.Cmd{m.loadBatch(m.batchID), m.loadBatches(), m.loadHome()}
	if r.Written || r.Skipped {
		// Move on to the next open file of the batch.
		if next := m.nextOpenFile(); next != "" {
			m.path = next
		}
		return tea.Batch(cmds...)
	}
	m.hunk = m.nextOpenHunk()
	return tea.Batch(append(cmds, m.loadFile())...)
}

func (m *Model) nextOpenHunk() int {
	if m.file == nil {
		return 0
	}
	n := len(m.file.Hunks)
	for i := 1; i <= n; i++ {
		j := (m.hunk + i) % n
		if m.file.Hunks[j].State == "pending" && j != m.hunk {
			return j
		}
	}
	return m.hunk
}

func (m *Model) nextOpenFile() string {
	if m.batch == nil {
		return ""
	}
	for _, f := range m.batch.Files {
		if f.Path != m.path && (f.Status == "open" || f.Status == "stale") {
			return f.Path
		}
	}
	return ""
}

func (m *Model) onMouse(msg tea.MouseMsg) tea.Cmd {
	if m.screen != scrReview {
		return nil
	}
	switch msg.Button {
	case tea.MouseButtonWheelUp:
		m.scroll = max(0, m.scroll-3)
	case tea.MouseButtonWheelDown:
		m.scroll += 3
	case tea.MouseButtonLeft:
		if msg.Action == tea.MouseActionPress {
			return m.onClick(msg.X, msg.Y)
		}
	}
	return nil
}

// onKey handles keys: global ones first, then the active input, then the screen.
func (m *Model) onKey(k tea.KeyMsg) tea.Cmd {
	key := k.String()
	if key == "ctrl+c" {
		return tea.Quit
	}
	m.err = ""

	if m.inKind != inputNone {
		return m.onInputKey(k)
	}
	if m.screen == scrTask {
		return m.onTaskKey(k)
	}
	if m.help {
		m.help = false
		return nil
	}
	if m.compare != nil {
		if key == "esc" || key == "q" || key == "V" || key == "enter" {
			m.compare = nil
		}
		return nil
	}
	if m.rule != nil {
		if key == "esc" || key == "q" || key == "i" {
			m.rule = nil
		}
		return nil
	}
	if m.done != nil {
		return m.onDoneKey(key)
	}

	switch key {
	case "q":
		return tea.Quit
	case "?":
		m.help = true
		return nil
	case "s":
		m.screen = scrStart
		return m.loadHome()
	case "w":
		if m.screen == scrBoard {
			m.screen = scrReview
			return m.pickBatch()
		}
		m.screen = scrBoard
		return tea.Batch(m.loadBatches(), m.loadHome())
	case "tab":
		if m.screen != scrReview {
			m.screen = scrReview
			return m.pickBatch()
		}
	case "o":
		m.screen = scrJournal
		return m.loadJournal()
	case ",":
		m.screen = scrSettings
		return m.loadSettings()
	case "n":
		return m.openTask()
	case "esc":
		if m.screen == scrJournal || m.screen == scrSettings {
			m.screen = scrStart
			return m.loadHome()
		}
	}

	switch m.screen {
	case scrReview:
		return m.onReviewKey(key)
	case scrBoard:
		return m.onBoardKey(key)
	case scrJournal:
		return m.onJournalKey(key)
	case scrSettings:
		return m.onSettingsKey(key)
	case scrStart:
		return m.onStartKey(key)
	}
	return nil
}

func (m *Model) onDoneKey(key string) tea.Cmd {
	switch key {
	case "enter":
		m.done = nil
		for _, b := range m.reviewable() {
			if b.ID != m.batchID && b.Column == "review" {
				m.batchID, m.path, m.file = b.ID, "", nil
				return m.loadBatch(b.ID)
			}
		}
		m.screen = scrStart
		return m.loadHome()
	case "u":
		id := m.batchID
		m.done = nil
		return func() tea.Msg {
			if _, err := m.api.UndoBatch(id); err != nil {
				return errMsg{err}
			}
			return nil
		}
	case "esc", "q":
		m.done = nil
	}
	return nil
}

func (m *Model) onStartKey(key string) tea.Cmd {
	switch key {
	case "j", "down":
		m.row = clamp(m.row+1, 0, len(m.home.Findings)-1)
	case "k", "up":
		m.row = clamp(m.row-1, 0, len(m.home.Findings)-1)
	case " ", "l", "right":
		// Show the findings of this check below its row.
		if m.row < len(m.home.Findings) {
			rule := string(m.home.Findings[m.row].Rule)
			if m.expanded == rule {
				m.expanded = ""
				return nil
			}
			m.expanded = rule
			return m.loadFindings()
		}
	case "enter", "b":
		if m.row < len(m.home.Findings) {
			rule := string(m.home.Findings[m.row].Rule)
			// A batch from this check is still open: go there instead of making a second one.
			if id := m.home.Open[rule]; id != "" {
				return func() tea.Msg { return openMsg{id: id} }
			}
			return m.fromFinding(rule)
		}
	}
	return nil
}

func (m *Model) fromFinding(rule string) tea.Cmd {
	return func() tea.Msg {
		sum, err := m.api.FromFinding(rule, "")
		if err != nil {
			return errMsg{err}
		}
		return openMsg{id: sum.ID}
	}
}

// Keys on the review screen.
func (m *Model) onReviewKey(key string) tea.Cmd {
	if m.batch == nil {
		if key == "enter" {
			return m.openTask()
		}
		return nil
	}
	f := m.file
	switch key {
	case "j", "down":
		if f != nil && m.hunk < len(f.Hunks)-1 {
			m.hunk++
			m.scrollToHunk()
		}
	case "k", "up":
		if m.hunk > 0 {
			m.hunk--
			m.scrollToHunk()
		}
	case "J":
		return m.stepFile(1)
	case "K":
		return m.stepFile(-1)
	case "]":
		return m.stepBatch(1)
	case "[":
		return m.stepBatch(-1)
	case "pgdown", "ctrl+d", " ":
		m.scroll += max(1, m.bodyHeight()-4)
	case "pgup", "ctrl+u":
		m.scroll = max(0, m.scroll-max(1, m.bodyHeight()-4))
	case "g", "home":
		m.scroll = 0
	case "G", "end":
		m.scroll = 1 << 30
	case "a":
		return m.decide("accepted", "")
	case "p":
		return m.decide("pending", "")
	case "r":
		if m.currentHunk() != nil {
			m.openInput(inputReason)
		}
	case "A":
		return m.decideFile("accepted")
	case "X":
		return m.decideFile("rejected")
	case "f":
		m.fbScope = 0
		if m.currentHunk() == nil {
			m.fbScope = 1
		}
		m.openInput(inputFeedback)
	case "F":
		m.fbScope = 2
		m.openInput(inputFeedback)
	case "e":
		return m.editExternal()
	case "R":
		return m.regenerate()
	case "v":
		return m.cycleVersion()
	case "V":
		return m.compareVersions()
	case "m":
		if m.mode == "split" {
			m.mode = "unified"
		} else {
			m.mode = "split"
		}
		return m.loadFile()
	case "z":
		m.reveal = !m.reveal
		return m.loadFile()
	case "u":
		return m.undoFile()
	case "y", "Y":
		return m.answerSuggestion(key == "y")
	case "c":
		id := m.batchID
		return func() tea.Msg { m.api.Cancel(id); return nil }
	case "i":
		return m.loadRule()
	case "O":
		id, p := m.batchID, m.path
		return func() tea.Msg {
			if _, err := m.api.Reopen(id, p); err != nil {
				return errMsg{err}
			}
			return nil
		}
	}
	return nil
}

func (m *Model) currentHunk() *service.HunkView {
	if m.file == nil || m.hunk < 0 || m.hunk >= len(m.file.Hunks) {
		return nil
	}
	return &m.file.Hunks[m.hunk]
}

func (m *Model) decide(decision, reason string) tea.Cmd {
	h := m.currentHunk()
	if h == nil {
		return nil
	}
	id, p, hid := m.batchID, m.path, h.ID
	return func() tea.Msg {
		r, err := m.api.Decide(id, p, hid, decision, reason)
		if err != nil {
			return errMsg{err}
		}
		return decideMsg(r)
	}
}

func (m *Model) decideFile(decision string) tea.Cmd {
	if m.file == nil {
		return nil
	}
	id, p := m.batchID, m.path
	return func() tea.Msg {
		r, err := m.api.DecideFile(id, p, decision)
		if err != nil {
			return errMsg{err}
		}
		return decideMsg(r)
	}
}

func (m *Model) stepFile(d int) tea.Cmd {
	if m.batch == nil || len(m.batch.Files) == 0 {
		return nil
	}
	i := 0
	for j, f := range m.batch.Files {
		if f.Path == m.path {
			i = j
		}
	}
	i += d
	if i < 0 || i >= len(m.batch.Files) {
		return m.stepBatch(d)
	}
	m.path, m.file = m.batch.Files[i].Path, nil
	return m.loadFile()
}

func (m *Model) stepBatch(d int) tea.Cmd {
	list := m.reviewable()
	if len(list) == 0 {
		return nil
	}
	i := 0
	for j, b := range list {
		if b.ID == m.batchID {
			i = j
		}
	}
	i = (i + d + len(list)) % len(list)
	m.batchID, m.path, m.file, m.batch = list[i].ID, "", nil, nil
	return m.loadBatch(m.batchID)
}

func (m *Model) regenerate() tea.Cmd {
	hunk := ""
	if h := m.currentHunk(); h != nil {
		hunk = h.ID
	}
	id, p := m.batchID, m.path
	return func() tea.Msg {
		if _, err := m.api.Regenerate(id, p, hunk); err != nil {
			return errMsg{err}
		}
		return nil
	}
}

func (m *Model) cycleVersion() tea.Cmd {
	if m.file == nil || len(m.file.Versions) < 2 {
		return nil
	}
	n := m.file.Current - 1
	if n < 1 {
		n = len(m.file.Versions)
	}
	id, p := m.batchID, m.path
	return func() tea.Msg {
		if _, err := m.api.SetVersion(id, p, n); err != nil {
			return errMsg{err}
		}
		return nil
	}
}

func (m *Model) compareVersions() tea.Cmd {
	if m.file == nil || m.file.Current < 2 {
		return nil
	}
	id, p, to := m.batchID, m.path, m.file.Current
	return func() tea.Msg {
		c, err := m.api.Compare(id, p, to-1, to, "split", 3)
		if err != nil {
			return errMsg{err}
		}
		return compareMsg(c)
	}
}

func (m *Model) undoFile() tea.Cmd {
	if m.file == nil || m.file.Status != "applied" {
		return nil
	}
	id, p := m.batchID, m.path
	return func() tea.Msg {
		if _, err := m.api.UndoFile(id, p); err != nil {
			return errMsg{err}
		}
		return nil
	}
}

func (m *Model) answerSuggestion(accept bool) tea.Cmd {
	if m.batch == nil {
		return nil
	}
	for _, s := range m.batch.Suggestions {
		if s.Status != "open" {
			continue
		}
		id, sid := m.batchID, s.ID
		return func() tea.Msg {
			if _, err := m.api.Suggestion(id, sid, accept); err != nil {
				return errMsg{err}
			}
			return nil
		}
	}
	return nil
}

// openInput shows the bottom input line.
func (m *Model) openInput(kind inputKind) {
	m.inKind, m.quick, m.remember, m.regen = kind, "", false, false
	m.input.EchoMode = textinput.EchoNormal
	m.input.SetValue("")
	m.input.Focus()
}

func (m *Model) closeInput() {
	m.inKind = inputNone
	m.input.Blur()
}

var quickCodes = map[string]string{"1": "wrong-fact", "2": "too-long", "3": "rulebook", "4": "question"}

func (m *Model) onInputKey(k tea.KeyMsg) tea.Cmd {
	key := k.String()
	switch key {
	case "esc":
		m.closeInput()
		return nil
	case "tab":
		switch m.inKind {
		case inputFeedback:
			m.fbScope = (m.fbScope + 1) % len(fbScopes)
		case inputReason:
			m.regen = !m.regen
		}
		return nil
	case "ctrl+r":
		m.remember = !m.remember
		return nil
	case "enter":
		return m.submitInput()
	}
	if code, ok := quickCodes[key]; ok && m.input.Value() == "" && m.inKind != inputKey && m.inKind != inputSetting {
		if m.quick == code {
			m.quick = ""
		} else {
			m.quick = code
		}
		return nil
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(k)
	return cmd
}

func (m *Model) submitInput() tea.Cmd {
	text := strings.TrimSpace(m.input.Value())
	kind := m.inKind
	m.closeInput()
	id, p := m.batchID, m.path
	quick, remember, regen := m.quick, m.remember, m.regen

	switch kind {
	case inputReason:
		reason := text
		if reason == "" && quick != "" {
			reason = m.t("quick." + quickIndex(quick))
		}
		h := m.currentHunk()
		if h == nil {
			return nil
		}
		hid := h.ID
		return func() tea.Msg {
			r, err := m.api.Decide(id, p, hid, "rejected", reason)
			if err != nil {
				return errMsg{err}
			}
			if regen {
				if _, err := m.api.Feedback(service.FeedbackInput{Batch: id, Scope: "hunk", Path: p, Hunk: hid, Text: firstNonEmpty(reason, m.t("regen")), Quick: quick}); err != nil {
					return errMsg{err}
				}
			}
			return decideMsg(r)
		}
	case inputFeedback:
		if text == "" && quick == "" {
			return nil
		}
		in := service.FeedbackInput{Batch: id, Scope: fbScopes[m.fbScope], Text: text, Quick: quick, Remember: remember}
		if in.Scope != "batch" {
			in.Path = p
		}
		if h := m.currentHunk(); h != nil && in.Scope == "hunk" {
			in.Hunk = h.ID
		}
		return func() tea.Msg {
			if _, err := m.api.Feedback(in); err != nil {
				return errMsg{err}
			}
			return nil
		}
	case inputSetting:
		return m.submitSetting(text)
	case inputKey:
		prov := m.keyFor
		return func() tea.Msg {
			if err := m.api.SetKey(prov, text); err != nil {
				return errMsg{err}
			}
			return toastMsg(prov + " ✓")
		}
	}
	return nil
}

func quickIndex(code string) string {
	for k, v := range quickCodes {
		if v == code {
			return k
		}
	}
	return "1"
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// Board keys.
func (m *Model) onBoardKey(key string) tea.Cmd {
	cols := m.boardColumns()
	switch key {
	case "h", "left":
		m.col = clamp(m.col-1, 0, len(cols)-1)
		m.row = 0
	case "l", "right":
		m.col = clamp(m.col+1, 0, len(cols)-1)
		m.row = 0
	case "j", "down":
		m.row = clamp(m.row+1, 0, len(cols[m.col].cards)-1)
	case "k", "up":
		m.row = clamp(m.row-1, 0, len(cols[m.col].cards)-1)
	case "enter", "b":
		c := m.selectedCard(cols)
		if c == nil {
			return nil
		}
		if c.rule != "" {
			return m.fromFinding(c.rule)
		}
		if key == "enter" {
			return func() tea.Msg { return openMsg{id: c.id} }
		}
	case "d":
		if c := m.selectedCard(cols); c != nil && c.id != "" {
			id := c.id
			return func() tea.Msg {
				if err := m.api.Discard(id); err != nil {
					return errMsg{err}
				}
				return nil
			}
		}
	case "c":
		if c := m.selectedCard(cols); c != nil && c.id != "" {
			id := c.id
			return func() tea.Msg { m.api.Cancel(id); return nil }
		}
	}
	return nil
}

func (m *Model) onJournalKey(key string) tea.Cmd {
	switch key {
	case "j", "down":
		m.ji = clamp(m.ji+1, 0, len(m.journal)-1)
	case "k", "up":
		m.ji = clamp(m.ji-1, 0, len(m.journal)-1)
	case "u":
		if m.ji >= len(m.journal) {
			return nil
		}
		e := m.journal[m.ji]
		return func() tea.Msg {
			if _, err := m.api.UndoFile(e.Batch, e.Path); err != nil {
				return errMsg{err}
			}
			return journalMsg(m.api.Journal(200))
		}
	case "U":
		if m.ji < len(m.journal) {
			return m.undoBatch(m.journal[m.ji].Batch)
		}
	case "enter", "d":
		// What this write changed, before you undo it.
		if m.ji < len(m.journal) {
			return m.journalDiff(m.journal[m.ji])
		}
	case "b":
		if m.ji < len(m.journal) {
			e := m.journal[m.ji]
			return func() tea.Msg { return openMsg{id: e.Batch, path: e.Path} }
		}
	}
	return nil
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func (m *Model) patch(p service.SettingsPatch) tea.Cmd {
	return func() tea.Msg {
		s, err := m.api.SetSettings(p)
		if err != nil {
			return errMsg{err}
		}
		return settingsMsg(s)
	}
}

func toggle(list []string, item string, on bool) []string {
	var out []string
	for _, x := range list {
		if x != item {
			out = append(out, x)
		}
	}
	if on {
		out = append(out, item)
	}
	return out
}

func ptr[T any](v T) *T { return &v }

// Task screen.
func (m *Model) openTask() tea.Cmd {
	m.back, m.screen = m.screen, scrTask
	m.taskField, m.estimate, m.findOnly = 0, nil, false
	m.scopeSel, m.scopeRow = map[string]bool{}, 0
	m.task.Reset()
	m.task.Focus()
	for i, p := range m.status.Providers {
		if p.Default {
			m.taskProv = i
		}
	}
	return nil
}

func (m *Model) taskInput() service.TaskInput {
	var scope []string
	for _, f := range m.status.Scopes {
		if m.scopeSel[f] {
			scope = append(scope, f)
		}
	}
	in := service.TaskInput{Instruction: m.task.Value(), Scope: scope, FindOnly: m.findOnly}
	if m.taskProv < len(m.status.Providers) {
		in.Provider = m.status.Providers[m.taskProv].Name
	}
	return in
}

func (m *Model) onTaskKey(k tea.KeyMsg) tea.Cmd {
	switch k.String() {
	case "esc":
		m.screen = m.back
		m.task.Blur()
		return nil
	case "tab":
		m.taskField = (m.taskField + 1) % 2
		if m.taskField == 0 {
			m.task.Focus()
		} else {
			m.task.Blur()
		}
		return m.estimateCmd()
	case "ctrl+f":
		m.findOnly = !m.findOnly
		return nil
	case "ctrl+t":
		m.nextTemplate()
		return m.estimateCmd()
	case "ctrl+p":
		if n := len(m.status.Providers); n > 0 {
			m.taskProv = (m.taskProv + 1) % n
		}
		return m.estimateCmd()
	case "ctrl+s":
		in := m.taskInput()
		if strings.TrimSpace(in.Instruction) == "" {
			return nil
		}
		m.screen = scrBoard
		m.task.Blur()
		return func() tea.Msg {
			if _, err := m.api.Task(in); err != nil {
				return errMsg{err}
			}
			return batchesMsg(m.api.Batches())
		}
	}
	// The folder list: j/k to move, space to pick; a folder covers its sub folders.
	if m.taskField == 1 {
		switch k.String() {
		case "j", "down":
			m.scopeRow = clamp(m.scopeRow+1, 0, len(m.status.Scopes)-1)
		case "k", "up":
			m.scopeRow = clamp(m.scopeRow-1, 0, len(m.status.Scopes)-1)
		case " ":
			if m.scopeRow < len(m.status.Scopes) {
				f := m.status.Scopes[m.scopeRow]
				if !m.scopeCovered(f) || m.scopeSel[f] {
					m.scopeSel[f] = !m.scopeSel[f]
					for g := range m.scopeSel {
						if strings.HasPrefix(g, f+"/") {
							delete(m.scopeSel, g)
						}
					}
				}
				return m.estimateCmd()
			}
		}
		return nil
	}
	var cmd tea.Cmd
	m.task, cmd = m.task.Update(k)
	return tea.Batch(cmd, m.estimateDebounced())
}

func (m *Model) estimateCmd() tea.Cmd {
	in := m.taskInput()
	return func() tea.Msg {
		e, err := m.api.Estimate(in)
		if err != nil {
			return errMsg{err}
		}
		return estimateMsg(e)
	}
}

func (m *Model) estimateDebounced() tea.Cmd {
	return tea.Tick(400*time.Millisecond, func(time.Time) tea.Msg { return m.estimateCmd()() })
}

func clamp(v, lo, hi int) int {
	if hi < lo {
		return lo
	}
	return min(max(v, lo), hi)
}

// scopeCovered: the folder or one of its parents is picked.
func (m *Model) scopeCovered(f string) bool {
	for g, on := range m.scopeSel {
		if on && (f == g || strings.HasPrefix(f, g+"/")) {
			return true
		}
	}
	return false
}
