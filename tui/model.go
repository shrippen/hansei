// Package tui is Hansei in the terminal. It uses service.API, so it runs the core in-process
// or talks to a running daemon; every feature of the KDE app is here too.
package tui

import (
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"git.arianw.de/shrippen/hansei/core/batch"
	"git.arianw.de/shrippen/hansei/core/service"
)

type screen int

const (
	scrStart screen = iota
	scrReview
	scrBoard
	scrJournal
	scrSettings
	scrTask
)

// inputKind is what the bottom input line is collecting.
type inputKind int

const (
	inputNone inputKind = iota
	inputFeedback
	inputReason
	inputKey
)

const (
	toastTime   = 4 * time.Second
	aiTextKeep  = 400
	defaultMode = "split"
)

var fbScopes = []string{"hunk", "file", "batch"}

// Model is the whole TUI state.
type Model struct {
	api   service.API
	lang  string
	style string
	st    styles
	w, h  int

	screen screen
	back   screen
	help   bool

	status  service.StatusView
	home    service.HomeView
	batches []service.BatchSummary

	// Review
	batchID string
	batch   *service.BatchView
	path    string
	file    *service.FileView
	hunk    int
	mode    string
	reveal  bool
	scroll  int
	compare *service.CompareView
	done    *service.DecideResult
	// Overlays and extras shared with the KDE app.
	compareFile  *service.FileView // what the compare overlay renders against (journal diffs)
	compareTitle string
	rule         *service.RuleSectionView
	findings     *service.FindingsView
	expanded     string // start page: rule whose findings are open
	findOnly     bool   // new task: only find notes
	live         map[string]batch.Usage
	hits         []hit // clickable areas of the last frame

	// Board
	col, row int

	// Journal
	journal []service.JournalView
	ji      int

	// Settings
	settings service.SettingsView
	si       int

	// New task
	task      textarea.Model
	taskScope textinput.Model
	taskField int
	taskProv  int
	estimate  *service.Estimate

	// Bottom input
	input    textinput.Model
	inKind   inputKind
	fbScope  int
	quick    string
	remember bool
	regen    bool
	keyFor   string

	toast      string
	toastUntil time.Time
	err        string
	aiText     map[string]string
	progress   map[string]string
	events     <-chan service.Event
	stopEvents func()
}

// New builds the model for an API.
func New(api service.API, lang, style string) *Model {
	in := textinput.New()
	in.Prompt = "› "
	in.CharLimit = 4000

	ta := textarea.New()
	ta.Placeholder = ""
	ta.ShowLineNumbers = false
	ta.SetHeight(4)
	ta.CharLimit = 4000

	sc := textinput.New()
	sc.Prompt = ""

	m := &Model{api: api, lang: lang, style: style, st: newStyles(style), mode: defaultMode, input: in, task: ta, taskScope: sc,
		aiText: map[string]string{}, progress: map[string]string{}, live: map[string]batch.Usage{}}
	m.task.Placeholder = m.t("taskHint")
	m.taskScope.Placeholder = m.t("scopeHint")
	return m
}

// Run starts the program in the alternate screen.
func Run(api service.API, lang, style string) error {
	m := New(api, lang, style)
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
	_, err := p.Run()
	if m.stopEvents != nil {
		m.stopEvents()
	}
	return err
}

// Init loads everything and starts listening for events.
func (m *Model) Init() tea.Cmd {
	m.events, m.stopEvents = m.api.Subscribe()
	return tea.Batch(m.loadStatus(), m.loadHome(), m.loadBatches(), m.waitEvent(), tickCmd())
}

// Messages from background loads.
type (
	statusMsg      service.StatusView
	homeMsg        service.HomeView
	batchesMsg     []service.BatchSummary
	batchMsg       service.BatchView
	fileMsg        service.FileView
	journalMsg     []service.JournalView
	settingsMsg    service.SettingsView
	estimateMsg    service.Estimate
	compareMsg     service.CompareView
	decideMsg      service.DecideResult
	eventMsg       service.Event
	errMsg         struct{ err error }
	toastMsg       string
	tickMsg        time.Time
	openMsg        struct{ id, path string }
	findingsMsg    service.FindingsView
	ruleMsg        service.RuleSectionView
	journalDiffMsg struct {
		title string
		cmp   service.CompareView
	}
)

func (m *Model) waitEvent() tea.Cmd {
	ch := m.events
	return func() tea.Msg {
		e, ok := <-ch
		if !ok {
			return nil
		}
		return eventMsg(e)
	}
}

func tickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m *Model) loadStatus() tea.Cmd {
	return func() tea.Msg { return statusMsg(m.api.Status()) }
}

func (m *Model) loadHome() tea.Cmd {
	return func() tea.Msg { return homeMsg(m.api.Home()) }
}

func (m *Model) loadBatches() tea.Cmd {
	return func() tea.Msg { return batchesMsg(m.api.Batches()) }
}

func (m *Model) loadBatch(id string) tea.Cmd {
	if id == "" {
		return nil
	}
	return func() tea.Msg {
		b, err := m.api.Batch(id)
		if err != nil {
			return errMsg{err}
		}
		return batchMsg(b)
	}
}

func (m *Model) loadFile() tea.Cmd {
	id, p, mode, reveal := m.batchID, m.path, m.mode, m.reveal
	if id == "" || p == "" {
		return nil
	}
	return func() tea.Msg {
		f, err := m.api.File(id, p, mode, 3, reveal)
		if err != nil {
			return errMsg{err}
		}
		return fileMsg(f)
	}
}

func (m *Model) loadJournal() tea.Cmd {
	return func() tea.Msg { return journalMsg(m.api.Journal(200)) }
}

func (m *Model) loadSettings() tea.Cmd {
	return func() tea.Msg { return settingsMsg(m.api.Settings()) }
}

func (m *Model) fail(err error) tea.Msg { return errMsg{err} }

func (m *Model) setToast(s string) {
	m.toast, m.toastUntil = s, time.Now().Add(toastTime)
}
