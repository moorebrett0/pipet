package brain

import (
	"context"
	"encoding/json"
)

// Provider abstracts the AI API (Claude, Gemini, etc.).
type Provider interface {
	Send(ctx context.Context, systemPrompt string, history []Message) (*Response, error)
}

// Message is a provider-agnostic conversation turn.
type Message struct {
	Role        string       // "user", "assistant"
	Text        string       // text content (may be empty if only tool calls/results)
	ToolCalls   []ToolCall   // assistant → tool invocations
	ToolResults []ToolResult // user → tool outputs
}

// ToolCall is a request from the model to invoke a tool.
type ToolCall struct {
	ID    string          // provider-assigned ID (Gemini uses function name)
	Name  string          // tool/function name
	Input json.RawMessage // JSON arguments
}

// ToolResult is the output of a tool invocation sent back to the model.
type ToolResult struct {
	ID      string // matches ToolCall.ID
	Content string
	IsError bool
}

// Response is what a provider returns from a single Send() call.
type Response struct {
	Text      string     // text output (may be empty if tool calls)
	ToolCalls []ToolCall // non-empty means the model wants to use tools
	Done      bool       // true if the model is finished (no more tool calls)
}
