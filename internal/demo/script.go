//go:build demo

package demo

import (
	"context"
	"path"
	"regexp"
	"strings"
	"time"

	"git.arianw.de/shrippen/hansei/core/llm"
)

const stepDelay = 450 * time.Millisecond

var (
	secretLineRe = regexp.MustCompile(`(?m)^(\s*-\s*[^:\n]*(?:Passwort|Token|Key)[^:\n]*:\s*)⟦GEHEIM_\d+⟧`)
	codeNames    = map[string]string{"SW-NAS01": "Nebelhorn", "SW-VPS": "Feuerschiff", "SW-PI": "Boje"}
	noteLineRe   = regexp.MustCompile(`(?m)^(\S.*\.md) \(`)
	batchFileRe  = regexp.MustCompile(`<file path="([^"]+)"`)
)

var replies = map[string][2]string{
	"secrets":  {"Klartext-Werte durch Vaultwarden-Verweise ersetzt.", "Replaced plain-text values with Vaultwarden references."},
	"names":    {"Alte Codenamen durch die Seezeichen-Namen ersetzt.", "Replaced old code names with the sea mark names."},
	"switch":   {"Switch in der Netzwerkübersicht aktualisiert.", "Updated the switch in the network overview."},
	"borg":     {"Verstanden. Restic durch Borg auf [[Nebelhorn]] ersetzt.", "Understood. Replaced Restic with Borg to [[Nebelhorn]]."},
	"generic":  {"Verstanden, in der Demo antwortet eine vorbereitete KI ohne echtes Modell.", "Understood; in the demo a prepared AI answers without a real model."},
	"question": {"Welche Notizen genau meinst du? Die Demo-KI kennt Secrets, Codenamen und den Switch.", "Which notes exactly? The demo AI knows secrets, code names and the switch."},
}

// newScript is the demo AI: deterministic edits for the tasks the demo data invites.
func newScript(name, lang string) llm.Provider {
	say := func(key string) string {
		if lang == "en" {
			return replies[key][1]
		}
		return replies[key][0]
	}
	return llm.NewScript(name, func(ctx context.Context, req llm.Request, call llm.CallFunc) (string, error) {
		step := func() {
			select {
			case <-ctx.Done():
			case <-time.After(stepDelay):
			}
		}
		task := strings.ToLower(req.Prompt)
		revise := strings.Contains(req.Prompt, "New feedback")
		if revise {
			// Only the new feedback decides; the history above also mentions older topics.
			task = strings.ToLower(req.Prompt[strings.LastIndex(req.Prompt, "New feedback"):])
		}

		kind := "generic"
		switch {
		case revise && strings.Contains(task, "borg"):
			kind = "borg"
		case !revise && (strings.Contains(task, "secret") || strings.Contains(task, "passw")):
			kind = "secrets"
		case !revise && (strings.Contains(task, "codenam") || strings.Contains(task, "code name")):
			kind = "names"
		case !revise && strings.Contains(task, "switch"):
			kind = "switch"
		}

		// Revisions start from the current proposals of the batch, new tasks from the notes.
		var paths []string
		read := "read_note"
		if revise {
			read = "read_proposal"
			for _, m := range batchFileRe.FindAllStringSubmatch(req.Prompt, -1) {
				paths = append(paths, m[1])
			}
		} else {
			list, _ := call("list_notes", map[string]string{"folder": ""})
			for _, m := range noteLineRe.FindAllStringSubmatch(list, -1) {
				paths = append(paths, m[1])
			}
		}
		step()
		changed := 0
		for _, p := range paths {
			if ctx.Err() != nil {
				return "", ctx.Err()
			}
			// The demo AI leaves the rulebooks and history alone, like the real one is told to.
			if strings.HasSuffix(p, "Design.md") || strings.HasSuffix(p, "agent.md") || strings.Contains(p, "/deprecated/") {
				continue
			}
			content, bad := call(read, map[string]string{"path": p})
			if bad {
				continue
			}
			next := edit(kind, content)
			if next == content {
				continue
			}
			step()
			if _, bad := call("propose_edit", map[string]any{"path": p, "content": next, "summary": say(kind),
				"changes": []map[string]string{{"reason": say(kind), "rule": "Design.md"}}}); !bad {
				changed++
			}
		}

		if changed == 0 && !revise {
			call("ask_user", map[string]string{"question": say("question")})
		}
		title := strings.TrimSpace(strings.SplitN(req.Prompt[strings.LastIndex(req.Prompt, "\n")+1:], ".", 2)[0])
		call("finish", map[string]string{"title": short(title), "topic": topic(kind, lang), "summary": say(kind), "reply": say(kind)})
		return say(kind), nil
	})
}

func edit(kind, content string) string {
	switch kind {
	case "secrets":
		return secretLineRe.ReplaceAllString(content, "${1}siehe Vaultwarden: Admin")
	case "names":
		for old, name := range codeNames {
			content = strings.ReplaceAll(content, old, name)
		}
		return content
	case "switch":
		return strings.ReplaceAll(content, "Knotenwerk KS-8, 8 Ports, alle belegt.", "Knotenwerk KS-24, 24 Ports.")
	case "borg":
		return regexp.MustCompile(`(?m)^Nächtlich mit Restic[^\n]*`).ReplaceAllString(
			regexp.MustCompile(`(?m)^Nightly with Restic[^\n]*`).ReplaceAllString(content, "Nightly with Borg to [[Nebelhorn]]."),
			"Nächtlich mit Borg auf [[Nebelhorn]].")
	}
	return content
}

func topic(kind, lang string) string {
	t := map[string][2]string{"secrets": {"Secrets", "Secrets"}, "names": {"Namen", "Names"}, "switch": {"Netzwerk", "Network"}, "borg": {"Backup", "Backup"}}[kind]
	if t[0] == "" {
		return "Demo"
	}
	if lang == "en" {
		return t[1]
	}
	return t[0]
}

func short(s string) string {
	if r := []rune(s); len(r) > 40 {
		return string(r[:40]) + "…"
	}
	if s == "" {
		return path.Base("Demo")
	}
	return s
}
