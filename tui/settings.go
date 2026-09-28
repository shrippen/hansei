package tui

import (
	"fmt"

	"sort"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"git.arianw.de/shrippen/hansei/core/service"
)

// Settings like the app's settings window: folders, checks, providers, style.

const reviewStep = 30 // days per +/- on the review date check

// settingsRow is one selectable line of the settings screen.
type settingsRow struct {
	kind string // folder, codename, required, days, provider
	key  string // folder path, code name, provider name
	idx  int
}

func (m *Model) settingsRows() []settingsRow {
	s := m.settings
	var rows []settingsRow
	for i, f := range s.Folders {
		rows = append(rows, settingsRow{kind: "folder", key: f.Path, idx: i})
	}
	for _, k := range sortedStrings(s.Codenames) {
		rows = append(rows, settingsRow{kind: "codename", key: k})
	}
	for _, k := range sortedStrings(s.Required) {
		rows = append(rows, settingsRow{kind: "required", key: k})
	}
	rows = append(rows, settingsRow{kind: "days"})
	for i, p := range s.Providers {
		rows = append(rows, settingsRow{kind: "provider", key: p.Name, idx: i})
	}
	return rows
}

func sortedStrings[V any](mp map[string]V) []string {
	keys := make([]string, 0, len(mp))
	for k := range mp {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func (m *Model) viewSettings() string {
	s := m.settings
	rows := m.settingsRows()
	m.si = clamp(m.si, 0, len(rows)-1)
	sel := func(i int, line string) string { return m.selRow(i == m.si, line) }

	// The vault path shortened to its last two folders.
	vault := s.Vault
	if parts := strings.Split(vault, "/"); len(parts) > 3 {
		vault = "…/" + strings.Join(parts[len(parts)-2:], "/")
	}
	lines := []string{m.st.label.Render(strings.ToUpper(m.t("folders"))) + m.st.dim.Render("  "+vault)}
	i := 0
	for _, f := range s.Folders {
		state := m.st.dim.Render("·")
		switch {
		case f.Blocked:
			state = m.st.chipFail.Render("■ " + m.t("blocked"))
		case f.Allowed:
			state = m.st.chipOK.Render("✓ " + m.t("allowed"))
		}
		if f.LocalOnly {
			state += m.st.chipWarn.Render(" · " + m.t("localOnly"))
		}
		// The rulebook only where the folder is allowed on its own.
		rule := ""
		if contains(s.Allow, f.Path) && len(f.Rulebook) > 0 {
			rule = m.st.dim.Render("  " + m.t("rulebook") + ": " + strings.Join(unique(f.Rulebook), ", "))
		}
		indent := strings.Repeat("  ", f.Depth)
		lines = append(lines, sel(i, fmt.Sprintf("%-32s ", trunc(indent+f.Name+"/", 32))+state+rule))
		i++
	}

	lines = append(lines, "", m.st.label.Render(strings.ToUpper(m.t("checks"))))
	for _, k := range sortedStrings(s.Codenames) {
		to := s.Codenames[k]
		if to == "" {
			to = m.st.dim.Render(m.t("onlyReport"))
		}
		lines = append(lines, sel(i, fmt.Sprintf("%-14s %-22s → ", m.t("codenames"), trunc(k, 22))+to))
		i++
	}
	for _, k := range sortedStrings(s.Required) {
		lines = append(lines, sel(i, fmt.Sprintf("%-14s %-22s   ", m.t("required"), trunc(k+"/", 22))+strings.Join(s.Required[k], ", ")))
		i++
	}
	days := m.t("reviewOff")
	if s.ReviewDays > 0 {
		days = m.t("reviewDays", s.ReviewDays)
	}
	lines = append(lines, sel(i, days))
	i++

	lines = append(lines, "", m.st.label.Render(strings.ToUpper(m.t("providers"))))
	for _, p := range s.Providers {
		key := ""
		switch {
		case p.HasKey:
			key = m.st.chipOK.Render(m.t("hasKey"))
		case !p.Local && p.Kind != "demo":
			key = m.st.chipFail.Render(m.t("noKey"))
		}
		flags := []string{}
		if p.Default {
			flags = append(flags, m.st.key.Render(m.t("default")))
		}
		if p.Local {
			flags = append(flags, m.st.chipWarn.Render(m.t("localOnly")))
		}
		price := m.st.dim.Render(m.t("noPrice"))
		if p.PriceIn > 0 || p.PriceOut > 0 {
			price = m.st.dim.Render(m.t("price", strconv.FormatFloat(p.PriceIn, 'f', -1, 64), strconv.FormatFloat(p.PriceOut, 'f', -1, 64)))
		}
		row := fmt.Sprintf("%-12s %-22s ", trunc(p.Name, 12), trunc(p.Model, 22)) + strings.Join(nonEmpty(key, strings.Join(flags, " · "), price), "  ")
		lines = append(lines, sel(i, row))
		if p.BaseURL != "" {
			lines = append(lines, "    "+m.st.dim.Render(trunc(p.BaseURL, m.w-8)))
		}
		i++
	}

	lines = append(lines, "", m.st.label.Render(strings.ToUpper(m.t("style")))+"  "+m.styleChoice(s.Style), m.st.dim.Render(m.t("styleNote")))
	lines = append(lines, "", m.st.dim.Render(trunc(s.Path, m.w-2)))
	return strings.Join(lines, "\n")
}

func unique(list []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, x := range list {
		if !seen[x] {
			seen[x] = true
			out = append(out, x)
		}
	}
	return out
}

func (m *Model) onSettingsKey(key string) tea.Cmd {
	rows := m.settingsRows()
	switch key {
	case "j", "down":
		m.si = clamp(m.si+1, 0, len(rows)-1)
		return nil
	case "k", "up":
		m.si = clamp(m.si-1, 0, len(rows)-1)
		return nil
	case "t":
		next := map[string]string{styleSystem: styleKanteLight, styleKanteLight: styleKante, styleKante: styleSystem}[m.settings.Style]
		return m.patch(service.SettingsPatch{Style: &next})
	case "a":
		m.edit("newcodename", "")
		return nil
	}
	if m.si >= len(rows) {
		return nil
	}
	r := rows[m.si]
	s := m.settings
	switch r.kind {
	case "folder":
		switch key {
		case " ":
			return m.patch(service.SettingsPatch{Allow: ptr(toggle(s.Allow, r.key, !contains(s.Allow, r.key)))})
		case "b":
			return m.patch(service.SettingsPatch{Block: ptr(toggle(s.Block, r.key, !contains(s.Block, r.key)))})
		case "l":
			return m.patch(service.SettingsPatch{LocalOnly: ptr(toggle(s.LocalOnly, r.key, !contains(s.LocalOnly, r.key)))})
		}
	case "codename":
		switch key {
		case "enter":
			m.edit("codename:"+r.key, s.Codenames[r.key])
		case "x":
			c := copyMap(s.Codenames)
			delete(c, r.key)
			return m.patch(service.SettingsPatch{Codenames: &c})
		}
	case "required":
		switch key {
		case "enter":
			m.edit("required:"+r.key, strings.Join(s.Required[r.key], ", "))
		case "x":
			c := copyMap(s.Required)
			delete(c, r.key)
			return m.patch(service.SettingsPatch{Required: &c})
		}
	case "days":
		days := s.ReviewDays
		switch key {
		case "+", "l", "right":
			days += reviewStep
		case "-", "h", "left":
			days = max(0, days-reviewStep)
		default:
			return nil
		}
		return m.patch(service.SettingsPatch{ReviewDays: &days})
	case "provider":
		p := s.Providers[r.idx]
		switch key {
		case "enter":
			return m.patch(service.SettingsPatch{Provider: &p.Name})
		case "K":
			m.keyFor = p.Name
			m.openInput(inputKey)
			m.input.EchoMode = textinput.EchoPassword
		case "M":
			m.edit(fmt.Sprintf("model:%d", r.idx), p.Model)
		case "P":
			m.edit(fmt.Sprintf("price:%d", r.idx), fmt.Sprintf("%g %g", p.PriceIn, p.PriceOut))
		case "c":
			m.setToast(m.t("testing"))
			return m.checkProvider(p.Name)
		}
	}
	return nil
}

func copyMap[V any](mp map[string]V) map[string]V {
	out := make(map[string]V, len(mp))
	for k, v := range mp {
		out[k] = v
	}
	return out
}

// edit opens the input line for a settings value.
func (m *Model) edit(what, value string) {
	m.editing = what
	m.openInput(inputSetting)
	m.input.SetValue(value)
	m.input.CursorEnd()
}

func (m *Model) settingPrompt() string {
	kind, arg, _ := strings.Cut(m.editing, ":")
	switch kind {
	case "codename":
		return m.t("prompt.codename", arg)
	case "newcodename":
		return m.t("prompt.newCodename")
	case "required":
		return m.t("required") + " " + arg
	case "model", "price":
		i, _ := strconv.Atoi(arg)
		name := ""
		if i < len(m.settings.Providers) {
			name = m.settings.Providers[i].Name
		}
		return m.t("prompt."+kind, name)
	}
	return ""
}

// submitSetting saves what the input line collected.
func (m *Model) submitSetting(text string) tea.Cmd {
	s := m.settings
	kind, arg, _ := strings.Cut(m.editing, ":")
	switch kind {
	case "codename":
		c := copyMap(s.Codenames)
		c[arg] = text
		return m.patch(service.SettingsPatch{Codenames: &c})
	case "newcodename":
		from, to, ok := strings.Cut(text, "=")
		if !ok || strings.TrimSpace(from) == "" {
			return nil
		}
		c := copyMap(s.Codenames)
		c[strings.TrimSpace(from)] = strings.TrimSpace(to)
		return m.patch(service.SettingsPatch{Codenames: &c})
	case "required":
		c := copyMap(s.Required)
		var fields []string
		for _, f := range strings.Split(text, ",") {
			if f = strings.TrimSpace(f); f != "" {
				fields = append(fields, f)
			}
		}
		c[arg] = fields
		return m.patch(service.SettingsPatch{Required: &c})
	case "model", "price":
		i, err := strconv.Atoi(arg)
		if err != nil || i >= len(s.Providers) {
			return nil
		}
		list := append([]service.ProviderView(nil), s.Providers...)
		if kind == "model" {
			list[i].Model = text
		} else {
			f := strings.Fields(strings.ReplaceAll(text, ",", "."))
			if len(f) != 2 {
				return nil
			}
			list[i].PriceIn, _ = strconv.ParseFloat(f[0], 64)
			list[i].PriceOut, _ = strconv.ParseFloat(f[1], 64)
		}
		return m.patch(service.SettingsPatch{Providers: &list})
	}
	return nil
}
