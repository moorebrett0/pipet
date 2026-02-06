package brain

import (
	"context"
	"encoding/json"

	"google.golang.org/genai"
)

// runShellDecl is the Gemini function declaration for executing shell commands.
var runShellDecl = &genai.FunctionDeclaration{
	Name:        "run_shell",
	Description: "Execute a shell command on the Raspberry Pi host. Use this to check system status, manage services, or investigate issues. Commands have a timeout and blocked patterns for safety. Output is truncated to 10KB.",
	Parameters: &genai.Schema{
		Type: genai.TypeObject,
		Properties: map[string]*genai.Schema{
			"command": {
				Type:        genai.TypeString,
				Description: "The shell command to execute",
			},
		},
		Required: []string{"command"},
	},
}

// geminiProvider implements Provider using the Google Gemini API.
type geminiProvider struct {
	client    *genai.Client
	model     string
	maxTokens int32
}

func newGeminiProvider(ctx context.Context, apiKey, model string, maxTokens int64) (*geminiProvider, error) {
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  apiKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		return nil, err
	}
	return &geminiProvider{
		client:    client,
		model:     model,
		maxTokens: int32(maxTokens),
	}, nil
}

func (g *geminiProvider) Send(ctx context.Context, systemPrompt string, history []Message) (*Response, error) {
	// Build contents from history
	var contents []*genai.Content
	for _, m := range history {
		role := m.Role
		if role == "assistant" {
			role = "model"
		}

		if len(m.ToolResults) > 0 {
			var parts []*genai.Part
			for _, tr := range m.ToolResults {
				resp := map[string]any{"output": tr.Content}
				if tr.IsError {
					resp["error"] = true
				}
				parts = append(parts, genai.NewPartFromFunctionResponse(tr.ID, resp))
			}
			contents = append(contents, &genai.Content{
				Role:  role,
				Parts: parts,
			})
			continue
		}

		if len(m.ToolCalls) > 0 {
			var parts []*genai.Part
			if m.Text != "" {
				parts = append(parts, genai.NewPartFromText(m.Text))
			}
			for _, tc := range m.ToolCalls {
				var args map[string]any
				_ = json.Unmarshal(tc.Input, &args)
				parts = append(parts, genai.NewPartFromFunctionCall(tc.Name, args))
			}
			contents = append(contents, &genai.Content{
				Role:  role,
				Parts: parts,
			})
			continue
		}

		contents = append(contents, genai.NewContentFromText(m.Text, genai.Role(role)))
	}

	config := &genai.GenerateContentConfig{
		SystemInstruction: genai.NewContentFromText(systemPrompt, ""),
		MaxOutputTokens:   g.maxTokens,
		Tools: []*genai.Tool{
			{FunctionDeclarations: []*genai.FunctionDeclaration{runShellDecl}},
		},
	}

	resp, err := g.client.Models.GenerateContent(ctx, g.model, contents, config)
	if err != nil {
		return nil, err
	}

	// Extract function calls
	calls := resp.FunctionCalls()
	if len(calls) > 0 {
		out := &Response{Done: false}
		// Also grab any text from the response
		out.Text = resp.Text()
		for _, fc := range calls {
			raw, _ := json.Marshal(fc.Args)
			id := fc.ID
			if id == "" {
				id = fc.Name // fallback: use name as ID
			}
			out.ToolCalls = append(out.ToolCalls, ToolCall{
				ID:    id,
				Name:  fc.Name,
				Input: raw,
			})
		}
		return out, nil
	}

	return &Response{
		Text: resp.Text(),
		Done: true,
	}, nil
}
