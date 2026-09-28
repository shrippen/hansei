package llm

import (
	"context"
	"encoding/json"
)

// CallFunc lets a script call a tool the way a model would.
type CallFunc func(name string, input any) (string, bool)

// ScriptFunc plays the model: it may call tools and returns the final text.
type ScriptFunc func(ctx context.Context, req Request, call CallFunc) (string, error)

// script is a provider without a model, for tests and offline runs.
type script struct {
	name string
	fn   ScriptFunc
}

// NewScript returns a provider that runs fn instead of a model.
func NewScript(name string, fn ScriptFunc) Provider { return &script{name: name, fn: fn} }

func (s *script) Name() string  { return s.name }
func (s *script) Model() string { return "script" }
func (s *script) Local() bool   { return true }

func (s *script) Run(ctx context.Context, req Request) (Result, error) {
	turns := 1
	callTool := func(name string, input any) (string, bool) {
		raw, err := json.Marshal(input)
		if err != nil {
			return err.Error(), true
		}
		turns++
		emit(req, Event{Kind: EventTool, Tool: name, Input: shortInput(raw)})
		return call(ctx, req.Tools, name, raw)
	}
	text, err := s.fn(ctx, req, callTool)
	return Result{Text: text, Turns: turns}, err
}
