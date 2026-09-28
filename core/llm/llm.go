// Package llm runs a tool-using conversation against a configured provider.
//
// Each provider owns its own loop so it can keep its native message history
// (Claude needs its thinking blocks echoed back unchanged).
//
//	agent ──Run(Request)──▶ Provider ──▶ model ──tool call──▶ Tool.Run ──▶ result ──▶ model … ──▶ Result
package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// Tool is a function the model may call.
type Tool struct {
	Name        string
	Description string
	Properties  map[string]any // JSON Schema properties
	Required    []string
	Run         func(ctx context.Context, input json.RawMessage) (string, error)
}

// EventKind classifies progress events.
type EventKind string

const (
	EventText  EventKind = "text"  // streamed answer text
	EventTool  EventKind = "tool"  // a tool call started
	EventUsage EventKind = "usage" // tokens so far in this run, after each turn
)

// Event reports progress to the UI.
type Event struct {
	Kind  EventKind `json:"kind"`
	Text  string    `json:"text,omitempty"`
	Tool  string    `json:"tool,omitempty"`
	Input string    `json:"input,omitempty"`
	Usage Usage     `json:"usage,omitzero"`
}

// Request is one task for the model.
type Request struct {
	System   string
	Prompt   string
	Tools    []Tool
	MaxTurns int
	OnEvent  func(Event)
}

// Usage counts tokens.
type Usage struct {
	In     int `json:"in"`
	Out    int `json:"out"`
	Cached int `json:"cached"`
}

// Result is the final answer.
type Result struct {
	Text  string
	Usage Usage
	Turns int
}

// Lister is implemented by providers that can list their models.
type Lister interface {
	Models(ctx context.Context) ([]string, error)
}

// Provider is one AI backend.
type Provider interface {
	Name() string
	Model() string
	Local() bool
	Run(ctx context.Context, req Request) (Result, error)
}

var (
	// ErrRefused means the model declined the request.
	ErrRefused = errors.New("llm: the model declined the request")
	// ErrTurns means the model did not finish within the turn limit.
	ErrTurns = errors.New("llm: too many tool rounds")
	// ErrNoKey means no API key was found.
	ErrNoKey = errors.New("llm: no API key")
)

// maxToolResult caps a single tool result sent back to the model.
const maxToolResult = 200_000

// call runs one tool and returns the text for the model; errors are returned as text
// flagged isError so the model can correct itself.
func call(ctx context.Context, tools []Tool, name string, input json.RawMessage) (string, bool) {
	for _, t := range tools {
		if t.Name != name {
			continue
		}
		if !json.Valid(input) {
			return "INVALID_JSON: the tool input was not valid JSON, send it again", true
		}
		out, err := t.Run(ctx, input)
		if err != nil {
			return "error: " + err.Error(), true
		}
		if len(out) > maxToolResult {
			out = out[:maxToolResult] + "\n… (truncated)"
		}
		return out, false
	}
	return fmt.Sprintf("error: unknown tool %q", name), true
}

func emit(req Request, e Event) {
	if req.OnEvent != nil {
		req.OnEvent(e)
	}
}

// shortInput shortens a tool input for progress display.
func shortInput(raw []byte) string {
	s := strings.Join(strings.Fields(string(raw)), " ")
	if r := []rune(s); len(r) > 120 {
		return string(r[:120]) + "…"
	}
	return s
}
