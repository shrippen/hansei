package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/anthropics/anthropic-sdk-go/shared/constant"

	"git.arianw.de/shrippen/hansei/core/config"
)

const (
	// claudeMaxTokens is generous because the model writes whole notes into tool calls;
	// streaming keeps long requests clear of HTTP timeouts.
	claudeMaxTokens = 64000
	// fallbackDefault lets the API pick the fallback model after a refusal.
	fallbackDefault = "default"
)

// claude talks to the Claude API through the official SDK.
type claude struct {
	client anthropic.Client
	cfg    config.Provider
}

// NewClaude creates the Claude provider. Without key the SDK resolves credentials itself
// (ANTHROPIC_API_KEY, ANTHROPIC_AUTH_TOKEN, an `ant auth login` profile).
func NewClaude(p config.Provider, key string) Provider {
	var opts []option.RequestOption
	if key != "" {
		opts = append(opts, option.WithAPIKey(key))
	}
	if p.BaseURL != "" {
		opts = append(opts, option.WithBaseURL(p.BaseURL))
	}
	return &claude{client: anthropic.NewClient(opts...), cfg: p}
}

func (c *claude) Name() string  { return c.cfg.Name }
func (c *claude) Model() string { return c.cfg.Model }
func (c *claude) Local() bool   { return c.cfg.Local }

// Run drives the tool loop: stream a turn, run the requested tools, send all results back
// in one user message, repeat until the model stops asking for tools.
func (c *claude) Run(ctx context.Context, req Request) (Result, error) {
	params := c.params(req)
	var res Result

	for res.Turns = 1; res.Turns <= req.MaxTurns; res.Turns++ {
		msg, err := c.turn(ctx, params, req)
		if err != nil {
			return res, err
		}
		res.Usage.In += int(msg.Usage.InputTokens) + int(msg.Usage.CacheCreationInputTokens) + int(msg.Usage.CacheReadInputTokens)
		res.Usage.Cached += int(msg.Usage.CacheReadInputTokens)
		res.Usage.Out += int(msg.Usage.OutputTokens)
		emit(req, Event{Kind: EventUsage, Usage: res.Usage})
		if msg.StopReason == anthropic.BetaStopReasonRefusal {
			return res, fmt.Errorf("%w: %s", ErrRefused, msg.StopDetails.Explanation)
		}

		// Keep the full assistant turn (incl. thinking blocks) in the history.
		params.Messages = append(params.Messages, msg.ToParam())

		var text strings.Builder
		var results []anthropic.BetaContentBlockParamUnion
		for _, block := range msg.Content {
			switch b := block.AsAny().(type) {
			case anthropic.BetaTextBlock:
				text.WriteString(b.Text)
			case anthropic.BetaToolUseBlock:
				raw := json.RawMessage(b.JSON.Input.Raw())
				emit(req, Event{Kind: EventTool, Tool: b.Name, Input: shortInput(raw)})
				out, isErr := call(ctx, req.Tools, b.Name, raw)
				results = append(results, anthropic.NewBetaToolResultBlock(b.ID, out, isErr))
			}
		}
		res.Text = text.String()

		if msg.StopReason == anthropic.BetaStopReasonMaxTokens && len(results) == 0 {
			return res, fmt.Errorf("llm: answer cut off at %d tokens", claudeMaxTokens)
		}
		if len(results) == 0 {
			return res, nil
		}
		params.Messages = append(params.Messages, anthropic.NewBetaUserMessage(results...))
	}
	return res, ErrTurns
}

// turn streams one model turn and returns the accumulated message.
func (c *claude) turn(ctx context.Context, params anthropic.BetaMessageNewParams, req Request) (anthropic.BetaMessage, error) {
	stream := c.client.Beta.Messages.NewStreaming(ctx, params)
	msg := anthropic.BetaMessage{}
	for stream.Next() {
		ev := stream.Current()
		if err := msg.Accumulate(ev); err != nil {
			return msg, err
		}
		delta, ok := ev.AsAny().(anthropic.BetaRawContentBlockDeltaEvent)
		if !ok {
			continue
		}
		if t, ok := delta.Delta.AsAny().(anthropic.BetaTextDelta); ok {
			emit(req, Event{Kind: EventText, Text: t.Text})
		}
	}
	return msg, stream.Err()
}

// params builds the request: cached system prompt, tools, first user message, fallbacks.
func (c *claude) params(req Request) anthropic.BetaMessageNewParams {
	tools := make([]anthropic.BetaToolUnionParam, 0, len(req.Tools))
	for _, t := range req.Tools {
		tp := anthropic.BetaToolParam{
			Name:        t.Name,
			Description: anthropic.String(t.Description),
			InputSchema: anthropic.BetaToolInputSchemaParam{Properties: t.Properties, Required: t.Required},
			// Whole notes travel as tool input; stream them instead of one late burst.
			// call() validates the JSON before a tool runs.
			EagerInputStreaming: anthropic.Bool(true),
		}
		tools = append(tools, anthropic.BetaToolUnionParam{OfTool: &tp})
	}

	p := anthropic.BetaMessageNewParams{
		Model:     anthropic.Model(c.cfg.Model),
		MaxTokens: claudeMaxTokens,
		System:    []anthropic.BetaTextBlockParam{{Text: req.System, CacheControl: anthropic.NewBetaCacheControlEphemeralParam()}},
		Tools:     tools,
		Messages:  []anthropic.BetaMessageParam{anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock(req.Prompt))},
	}

	// Server-side fallback: a refused request is served again by another model in the same call.
	switch c.cfg.Fallback {
	case "":
	case fallbackDefault:
		p.Fallbacks = anthropic.BetaFallbacksParamUnion{OfDefault: constant.ValueOf[constant.Default]()}
		p.Betas = []anthropic.AnthropicBeta{anthropic.AnthropicBetaServerSideFallback2026_07_01}
	default:
		p.Fallbacks = anthropic.BetaFallbacksParamUnion{OfBetaFallbackArray: []anthropic.BetaFallbackParam{{Model: anthropic.Model(c.cfg.Fallback)}}}
		p.Betas = []anthropic.AnthropicBeta{anthropic.AnthropicBetaServerSideFallback2026_06_01}
	}
	return p
}

// Models lists the model IDs the key can use.
func (c *claude) Models(ctx context.Context) ([]string, error) {
	var out []string
	iter := c.client.Models.ListAutoPaging(ctx, anthropic.ModelListParams{})
	for iter.Next() {
		out = append(out, iter.Current().ID)
	}
	return out, iter.Err()
}
