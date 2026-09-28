package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"git.arianw.de/shrippen/hansei/core/config"
)

const (
	chatPath       = "/chat/completions"
	openAITimeout  = 15 * time.Minute
	finishToolCall = "tool_calls"
	finishLength   = "length"
	maxErrorBody   = 2048
)

// openAI speaks the OpenAI chat completions protocol, which Ollama, LM Studio and
// many gateways offer. It is used for non-Claude models only.
type openAI struct {
	http *http.Client
	cfg  config.Provider
	key  string
}

// NewOpenAI creates a provider for an OpenAI-compatible endpoint (BaseURL ends in /v1).
func NewOpenAI(p config.Provider, key string) Provider {
	return &openAI{http: &http.Client{Timeout: openAITimeout}, cfg: p, key: key}
}

func (o *openAI) Name() string  { return o.cfg.Name }
func (o *openAI) Model() string { return o.cfg.Model }

// Local is true when configured so or when the endpoint is on this machine.
func (o *openAI) Local() bool { return o.cfg.Local || LocalURL(o.cfg.BaseURL) }

// LocalURL reports whether an endpoint runs on this machine (localhost, loopback).
func LocalURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	h := u.Hostname()
	return h == "localhost" || h == "127.0.0.1" || h == "::1"
}

type oaMessage struct {
	Role       string       `json:"role"`
	Content    *string      `json:"content"`
	ToolCalls  []oaToolCall `json:"tool_calls,omitempty"`
	ToolCallID string       `json:"tool_call_id,omitempty"`
}

type oaToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type oaTool struct {
	Type     string `json:"type"`
	Function struct {
		Name        string         `json:"name"`
		Description string         `json:"description"`
		Parameters  map[string]any `json:"parameters"`
	} `json:"function"`
}

type oaResponse struct {
	Choices []struct {
		Message      oaMessage `json:"message"`
		FinishReason string    `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		Prompt     int `json:"prompt_tokens"`
		Completion int `json:"completion_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// Run drives the same tool loop as the Claude provider over chat completions.
func (o *openAI) Run(ctx context.Context, req Request) (Result, error) {
	system, prompt := req.System, req.Prompt
	msgs := []oaMessage{{Role: "system", Content: &system}, {Role: "user", Content: &prompt}}
	tools := o.tools(req.Tools)
	var res Result

	for res.Turns = 1; res.Turns <= req.MaxTurns; res.Turns++ {
		resp, err := o.post(ctx, map[string]any{"model": o.cfg.Model, "messages": msgs, "tools": tools})
		if err != nil {
			return res, err
		}
		res.Usage.In += resp.Usage.Prompt
		res.Usage.Out += resp.Usage.Completion
		emit(req, Event{Kind: EventUsage, Usage: res.Usage})
		if len(resp.Choices) == 0 {
			return res, fmt.Errorf("llm: empty answer from %s", o.cfg.Name)
		}

		choice := resp.Choices[0]
		msgs = append(msgs, choice.Message)
		if choice.Message.Content != nil {
			res.Text = *choice.Message.Content
			emit(req, Event{Kind: EventText, Text: res.Text})
		}
		if len(choice.Message.ToolCalls) == 0 {
			if choice.FinishReason == finishLength {
				return res, fmt.Errorf("llm: answer cut off by %s", o.cfg.Name)
			}
			return res, nil
		}

		for _, tc := range choice.Message.ToolCalls {
			raw := json.RawMessage(tc.Function.Arguments)
			emit(req, Event{Kind: EventTool, Tool: tc.Function.Name, Input: shortInput(raw)})
			out, _ := call(ctx, req.Tools, tc.Function.Name, raw)
			msgs = append(msgs, oaMessage{Role: "tool", Content: &out, ToolCallID: tc.ID})
		}
	}
	return res, ErrTurns
}

func (o *openAI) tools(in []Tool) []oaTool {
	out := make([]oaTool, 0, len(in))
	for _, t := range in {
		var ot oaTool
		ot.Type = "function"
		ot.Function.Name = t.Name
		ot.Function.Description = t.Description
		ot.Function.Parameters = map[string]any{"type": "object", "properties": t.Properties, "required": t.Required}
		out = append(out, ot)
	}
	return out
}

func (o *openAI) post(ctx context.Context, body map[string]any) (*oaResponse, error) {
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	endpoint := strings.TrimRight(o.cfg.BaseURL, "/") + chatPath
	hr, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	hr.Header.Set("Content-Type", "application/json")
	if o.key != "" {
		hr.Header.Set("Authorization", "Bearer "+o.key)
	}

	resp, err := o.http.Do(hr)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		if len(data) > maxErrorBody {
			data = data[:maxErrorBody]
		}
		return nil, fmt.Errorf("llm: %s answered %d: %s", o.cfg.Name, resp.StatusCode, strings.TrimSpace(string(data)))
	}

	var out oaResponse
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	if out.Error != nil {
		return nil, fmt.Errorf("llm: %s: %s", o.cfg.Name, out.Error.Message)
	}
	return &out, nil
}

// Models lists the models of the endpoint (GET /models).
func (o *openAI) Models(ctx context.Context) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(o.cfg.BaseURL, "/")+"/models", nil)
	if err != nil {
		return nil, err
	}
	if o.key != "" {
		req.Header.Set("Authorization", "Bearer "+o.key)
	}
	resp, err := o.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var body struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}
	out := make([]string, 0, len(body.Data))
	for _, m := range body.Data {
		out = append(out, m.ID)
	}
	return out, nil
}
