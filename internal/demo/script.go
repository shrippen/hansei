//go:build demo

package demo

import (
	"context"
	"regexp"
	"strings"
	"time"

	"git.arianw.de/shrippen/hansei/core/llm"
)

const stepDelay = 450 * time.Millisecond

var (
	secretLineRe = regexp.MustCompile(`(?m)^(\s*-\s*[^:\n]*(?:Passwort|Token|Key)[^:\n]*:\s*)⟦GEHEIM_\d+⟧`)
	noteLineRe   = regexp.MustCompile(`(?m)^(\S.*\.md) \(`)
	batchFileRe  = regexp.MustCompile(`<file path="([^"]+)"`)
)

// aiScript is the world's it_docs.ai_script: the demo AI's replies, topics
// and edits per kind of task.
type aiScript struct {
	Replies     map[string]text `json:"replies"`
	Topics      map[string]text `json:"topics"`
	SecretValue string          `json:"secret_value"`
	Switch      struct {
		From string `json:"from"`
		To   string `json:"to"`
	} `json:"switch"`
	Borg []struct {
		From string `json:"from"`
		To   string `json:"to"`
	} `json:"borg"`
}

// newScript is the demo AI: deterministic edits for the tasks the demo data invites.
func newScript(name, lang string, w world) llm.Provider {
	script, names := w.IT.Script, codeNames(w)
	say := func(key string) string { return script.Replies[key].in(lang) }
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
			next := edit(script, names, kind, content)
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
		call("finish", map[string]string{"title": short(title, topic(script, "", lang)), "topic": topic(script, kind, lang), "summary": say(kind), "reply": say(kind)})
		return say(kind), nil
	})
}

func edit(script aiScript, names map[string]string, kind, content string) string {
	switch kind {
	case "secrets":
		return secretLineRe.ReplaceAllString(content, "${1}"+script.SecretValue)
	case "names":
		for old, name := range names {
			content = strings.ReplaceAll(content, old, name)
		}
		return content
	case "switch":
		return strings.ReplaceAll(content, script.Switch.From, script.Switch.To)
	case "borg":
		// A line that starts with From becomes To (one pair per language).
		for _, r := range script.Borg {
			content = regexp.MustCompile(`(?m)^`+regexp.QuoteMeta(r.From)+`[^\n]*`).ReplaceAllLiteralString(content, r.To)
		}
	}
	return content
}

func topic(script aiScript, kind, lang string) string {
	if t, ok := script.Topics[kind]; ok {
		return t.in(lang)
	}
	return script.Topics["other"].in(lang)
}

func short(s, fallback string) string {
	if r := []rune(s); len(r) > 40 {
		return string(r[:40]) + "…"
	}
	if s == "" {
		return fallback
	}
	return s
}
