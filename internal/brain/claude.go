package brain

import (
	"context"
	"encoding/json"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// runShellTool is the Claude tool definition for executing shell commands.
var runShellTool anthropic.ToolUnionParam

func init() {
	tool := anthropic.ToolUnionParamOfTool(
		anthropic.ToolInputSchemaParam{
			Type: "object",
			Properties: map[string]any{
				"command": map[string]any{
					"type":        "string",
					"description": "The shell command to execute",
				},
			},
			Required: []string{"command"},
		},
		"run_shell",
	)
	tool.OfTool.Description = anthropic.String("Execute a shell command on the Raspberry Pi host. Use this to check system status, manage services, or investigate issues. Commands have a timeout and blocked patterns for safety. Output is truncated to 10KB.")
	runShellTool = tool
}

// claudeProvider implements Provider using the Anthropic Claude API.
type claudeProvider struct {
	client    *anthropic.Client
	model     anthropic.Model
	maxTokens int64
}

func newClaudeProvider(apiKey, model string, maxTokens int64) *claudeProvider {
	client := anthropic.NewClient(option.WithAPIKey(apiKey))
	return &claudeProvider{
		client:    &client,
		model:     anthropic.Model(model),
		maxTokens: maxTokens,
	}
}

func (c *claudeProvider) Send(ctx context.Context, systemPrompt string, history []Message) (*Response, error) {
	// Convert agnostic messages to anthropic params
	var msgs []anthropic.MessageParam
	for _, m := range history {
		switch m.Role {
		case "user":
			if len(m.ToolResults) > 0 {
				var blocks []anthropic.ContentBlockParamUnion
				for _, tr := range m.ToolResults {
					blocks = append(blocks, anthropic.NewToolResultBlock(tr.ID, tr.Content, tr.IsError))
				}
				msgs = append(msgs, anthropic.MessageParam{
					Role:    anthropic.MessageParamRoleUser,
					Content: blocks,
				})
			} else {
				msgs = append(msgs, anthropic.MessageParam{
					Role: anthropic.MessageParamRoleUser,
					Content: []anthropic.ContentBlockParamUnion{
						anthropic.NewTextBlock(m.Text),
					},
				})
			}
		case "assistant":
			var blocks []anthropic.ContentBlockParamUnion
			if m.Text != "" {
				blocks = append(blocks, anthropic.NewTextBlock(m.Text))
			}
			for _, tc := range m.ToolCalls {
				blocks = append(blocks, anthropic.NewToolUseBlock(tc.ID, tc.Input, tc.Name))
			}
			msgs = append(msgs, anthropic.NewAssistantMessage(blocks...))
		}
	}

	resp, err := c.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     c.model,
		MaxTokens: c.maxTokens,
		System:    []anthropic.TextBlockParam{{Text: systemPrompt}},
		Messages:  msgs,
		Tools:     []anthropic.ToolUnionParam{runShellTool},
	})
	if err != nil {
		return nil, err
	}

	// Convert response
	out := &Response{Done: resp.StopReason != anthropic.StopReasonToolUse}

	for _, block := range resp.Content {
		switch block.Type {
		case "text":
			out.Text = block.AsText().Text
		case "tool_use":
			tu := block.AsToolUse()
			raw, _ := json.Marshal(tu.Input)
			out.ToolCalls = append(out.ToolCalls, ToolCall{
				ID:    tu.ID,
				Name:  tu.Name,
				Input: raw,
			})
		}
	}

	return out, nil
}
