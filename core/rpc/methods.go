package rpc

import (
	"encoding/json"

	"git.arianw.de/shrippen/hansei/core/service"
)

// handler decodes params and calls the API.
type handler func(api service.API, params json.RawMessage) (any, error)

// with decodes params into P before calling fn.
func with[P any](fn func(api service.API, p P) (any, error)) handler {
	return func(api service.API, raw json.RawMessage) (any, error) {
		var p P
		if len(raw) > 0 && string(raw) != "null" {
			if err := json.Unmarshal(raw, &p); err != nil {
				return nil, &rpcError{Code: codeParams, Message: err.Error()}
			}
		}
		return fn(api, p)
	}
}

type idParams struct {
	ID string `json:"id"`
}

type fileParams struct {
	ID       string `json:"id"`
	Path     string `json:"path"`
	Mode     string `json:"mode"`
	Context  int    `json:"context"`
	Reveal   bool   `json:"reveal"`
	Hunk     string `json:"hunk"`
	Decision string `json:"decision"`
	Reason   string `json:"reason"`
	Text     string `json:"text"`
	Content  string `json:"content"`
	N        int    `json:"n"`
	From     int    `json:"from"`
	To       int    `json:"to"`
}

type ok struct {
	OK bool `json:"ok"`
}

// Methods maps method names to the API; the KDE app uses these names.
var Methods = map[string]handler{
	"hello": func(service.API, json.RawMessage) (any, error) {
		return map[string]any{"protocol": Version, "name": "hansei"}, nil
	},
	"status": func(a service.API, _ json.RawMessage) (any, error) { return a.Status(), nil },
	"home":   func(a service.API, _ json.RawMessage) (any, error) { return a.Home(), nil },
	"findings": with(func(a service.API, p struct {
		Refresh bool `json:"refresh"`
	}) (any, error) {
		return a.Findings(p.Refresh), nil
	}),
	"batches": func(a service.API, _ json.RawMessage) (any, error) { return a.Batches(), nil },
	"batch":   with(func(a service.API, p idParams) (any, error) { return a.Batch(p.ID) }),
	"file": with(func(a service.API, p fileParams) (any, error) {
		return a.File(p.ID, p.Path, p.Mode, p.Context, p.Reveal)
	}),
	"compare": with(func(a service.API, p fileParams) (any, error) {
		return a.Compare(p.ID, p.Path, p.From, p.To, p.Mode, p.Context)
	}),
	"decide": with(func(a service.API, p fileParams) (any, error) {
		return a.Decide(p.ID, p.Path, p.Hunk, p.Decision, p.Reason)
	}),
	"decideFile": with(func(a service.API, p fileParams) (any, error) { return a.DecideFile(p.ID, p.Path, p.Decision) }),
	"editHunk":   with(func(a service.API, p fileParams) (any, error) { return a.EditHunk(p.ID, p.Path, p.Hunk, p.Text) }),
	"editFile":   with(func(a service.API, p fileParams) (any, error) { return a.EditFile(p.ID, p.Path, p.Content) }),
	"setVersion": with(func(a service.API, p fileParams) (any, error) { return a.SetVersion(p.ID, p.Path, p.N) }),
	"reopen":     with(func(a service.API, p fileParams) (any, error) { return a.Reopen(p.ID, p.Path) }),
	"undoFile":   with(func(a service.API, p fileParams) (any, error) { return a.UndoFile(p.ID, p.Path) }),
	"undoBatch":  with(func(a service.API, p idParams) (any, error) { return a.UndoBatch(p.ID) }),
	"journal": with(func(a service.API, p struct {
		Limit int `json:"limit"`
	}) (any, error) {
		return a.Journal(p.Limit), nil
	}),
	"discard": with(func(a service.API, p idParams) (any, error) { return ok{true}, a.Discard(p.ID) }),
	"cancel": with(func(a service.API, p idParams) (any, error) {
		a.Cancel(p.ID)
		return ok{true}, nil
	}),
	"task":       with(func(a service.API, p service.TaskInput) (any, error) { return a.Task(p) }),
	"estimate":   with(func(a service.API, p service.TaskInput) (any, error) { return a.Estimate(p) }),
	"feedback":   with(func(a service.API, p service.FeedbackInput) (any, error) { return a.Feedback(p) }),
	"regenerate": with(func(a service.API, p fileParams) (any, error) { return a.Regenerate(p.ID, p.Path, p.Hunk) }),
	"suggestion": with(func(a service.API, p struct {
		ID     string `json:"id"`
		SID    string `json:"sid"`
		Accept bool   `json:"accept"`
	}) (any, error) {
		return a.Suggestion(p.ID, p.SID, p.Accept)
	}),
	"fromFinding": with(func(a service.API, p struct {
		Rule     string `json:"rule"`
		Provider string `json:"provider"`
	}) (any, error) {
		return a.FromFinding(p.Rule, p.Provider)
	}),
	"import": func(a service.API, _ json.RawMessage) (any, error) {
		n, err := a.Import()
		return map[string]int{"imported": n}, err
	},
	"settings": func(a service.API, _ json.RawMessage) (any, error) { return a.Settings(), nil },
	"checkProvider": with(func(a service.API, p struct {
		Name string `json:"name"`
	}) (any, error) {
		return a.CheckProvider(p.Name), nil
	}),
	"ruleSection": with(func(a service.API, p struct {
		Ref string `json:"ref"`
	}) (any, error) {
		return a.RuleSection(p.Ref)
	}),
	"journalDiff": with(func(a service.API, p idParams) (any, error) { return a.JournalDiff(p.ID) }),
	"setSettings": with(func(a service.API, p service.SettingsPatch) (any, error) { return a.SetSettings(p) }),
	"setKey": with(func(a service.API, p struct {
		Provider string `json:"provider"`
		Key      string `json:"key"`
	}) (any, error) {
		return ok{true}, a.SetKey(p.Provider, p.Key)
	}),
}
