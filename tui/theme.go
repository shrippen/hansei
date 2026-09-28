package tui

import (
	_ "embed"
	"encoding/json"

	"github.com/charmbracelet/lipgloss"
)

//go:embed kante/palette.json
var paletteJSON []byte

// Style names match the config (system is the default, Kante is opt-in).
const (
	styleSystem     = "system"
	styleKanteLight = "kante-light"
	styleKante      = "kante"
)

// palette is the role set every style fills.
type palette struct {
	ground, panel, field, border  lipgloss.TerminalColor
	text, strong, muted, dim      lipgloss.TerminalColor
	accent, accentFg              lipgloss.TerminalColor
	add, del, addBg, delBg        lipgloss.TerminalColor
	info, ok, warn, hl, tag, fail lipgloss.TerminalColor
}

// kantePalette mirrors kante/tokens/palette.json (synced by scripts/sync-kante.sh).
type kantePalette struct {
	Backgrounds map[string]string `json:"backgrounds"`
	Foregrounds map[string]string `json:"foregrounds"`
	Accent      string            `json:"accent"`
	Semantic    map[string]struct {
		Bright  string `json:"bright"`
		Neutral string `json:"neutral"`
		Faded   string `json:"faded"`
	} `json:"semantic"`
	Light struct {
		Backgrounds map[string]string `json:"backgrounds"`
		Foregrounds map[string]string `json:"foregrounds"`
		Accent      string            `json:"accent"`
		Semantic    map[string]string `json:"semantic"`
	} `json:"light"`
}

func loadKante() kantePalette {
	var k kantePalette
	_ = json.Unmarshal(paletteJSON, &k)
	return k
}

// themeFor builds the palette of a style. Kante uses the dark tokens, Kante Light the
// "Leinen" tokens; System uses the terminal's own ANSI colours so it follows its scheme.
func themeFor(style string) palette {
	k := loadKante()
	c := func(hex string) lipgloss.TerminalColor { return lipgloss.Color(hex) }
	switch style {
	case styleKante:
		return palette{
			ground: c(k.Backgrounds["bg-hard"]), panel: c(k.Backgrounds["bg0"]), field: c(k.Backgrounds["bg1"]), border: c(k.Backgrounds["bg2"]),
			text: c(k.Foregrounds["fg1"]), strong: c(k.Foregrounds["fg0"]), muted: c(k.Foregrounds["fg2"]), dim: c(k.Foregrounds["fg3"]),
			accent: c(k.Semantic["yellow"].Bright), accentFg: c(k.Backgrounds["bg-hard"]),
			add: c(k.Semantic["green"].Bright), del: c(k.Semantic["red"].Bright),
			addBg: c(k.Semantic["green"].Faded), delBg: c(k.Semantic["red"].Faded),
			info: c(k.Semantic["blue"].Bright), ok: c(k.Semantic["aqua"].Bright), warn: c(k.Semantic["yellow"].Bright),
			hl: c(k.Semantic["orange"].Bright), tag: c(k.Semantic["purple"].Bright), fail: c(k.Semantic["red"].Bright),
		}
	case styleKanteLight:
		l := k.Light
		return palette{
			ground: c(l.Backgrounds["bg-void"]), panel: c(l.Backgrounds["bg-panel"]), field: c(l.Backgrounds["bg1"]), border: c(l.Backgrounds["bg2"]),
			text: c(l.Foregrounds["fg1"]), strong: c(l.Foregrounds["fg0"]), muted: c(l.Foregrounds["fg2"]), dim: c(l.Foregrounds["fg3"]),
			accent: c(l.Semantic["yellow"]), accentFg: c(l.Backgrounds["bg-void"]),
			add: c(l.Semantic["green"]), del: c(l.Semantic["red"]),
			addBg: c(l.Backgrounds["bg-hard"]), delBg: c(l.Backgrounds["bg-hard"]),
			info: c(l.Semantic["blue"]), ok: c(l.Semantic["aqua"]), warn: c(l.Semantic["yellow"]),
			hl: c(l.Semantic["orange"]), tag: c(l.Semantic["purple"]), fail: c(l.Semantic["red"]),
		}
	}
	// System: ANSI 0–15, whatever the terminal scheme makes of them.
	a := func(n string) lipgloss.TerminalColor { return lipgloss.Color(n) }
	return palette{
		ground: lipgloss.NoColor{}, panel: lipgloss.NoColor{}, field: a("8"), border: a("8"),
		text: lipgloss.NoColor{}, strong: a("15"), muted: a("7"), dim: a("8"),
		accent: a("3"), accentFg: a("0"),
		add: a("2"), del: a("1"), addBg: a("22"), delBg: a("52"),
		info: a("4"), ok: a("6"), warn: a("3"), hl: a("5"), tag: a("5"), fail: a("1"),
	}
}

// styles are the lipgloss styles built from a palette.
type styles struct {
	p                                   palette
	base, strong, muted, dim            lipgloss.Style
	title, label, key                   lipgloss.Style
	add, del, addWord, delWord, filler  lipgloss.Style
	chipOK, chipInfo, chipWarn, chipTag lipgloss.Style
	chipFail, chipDim, chipHl, sel, bar lipgloss.Style
	border                              lipgloss.Style
}

func newStyles(style string) styles {
	p := themeFor(style)
	s := styles{p: p}
	s.base = lipgloss.NewStyle().Foreground(p.text)
	s.strong = lipgloss.NewStyle().Foreground(p.strong).Bold(true)
	s.muted = lipgloss.NewStyle().Foreground(p.muted)
	s.dim = lipgloss.NewStyle().Foreground(p.dim)
	s.title = lipgloss.NewStyle().Foreground(p.strong).Bold(true)
	s.label = lipgloss.NewStyle().Foreground(p.dim)
	s.key = lipgloss.NewStyle().Foreground(p.accent).Bold(true)
	s.add = lipgloss.NewStyle().Foreground(p.add)
	s.del = lipgloss.NewStyle().Foreground(p.del)
	s.addWord = lipgloss.NewStyle().Foreground(p.strong).Background(p.addBg).Bold(true)
	s.delWord = lipgloss.NewStyle().Foreground(p.strong).Background(p.delBg).Strikethrough(true)
	s.filler = lipgloss.NewStyle().Foreground(p.border)
	s.chipOK = lipgloss.NewStyle().Foreground(p.ok)
	s.chipInfo = lipgloss.NewStyle().Foreground(p.info)
	s.chipWarn = lipgloss.NewStyle().Foreground(p.warn)
	s.chipTag = lipgloss.NewStyle().Foreground(p.tag)
	s.chipFail = lipgloss.NewStyle().Foreground(p.fail)
	s.chipDim = lipgloss.NewStyle().Foreground(p.dim)
	s.chipHl = lipgloss.NewStyle().Foreground(p.hl)
	s.sel = lipgloss.NewStyle().Foreground(p.strong).Background(p.field)
	s.bar = lipgloss.NewStyle().Foreground(p.accentFg).Background(p.accent).Bold(true)
	s.border = lipgloss.NewStyle().Foreground(p.border)
	return s
}
