package brain

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/moorebrett0/pipet/internal/monitor"
	"github.com/moorebrett0/pipet/internal/pet"
	"github.com/moorebrett0/pipet/internal/shell"
	"github.com/moorebrett0/pipet/internal/species"
)

// Brain wraps an AI provider with system prompt building and tool-use loop.
type Brain struct {
	provider Provider
	maxTools int
	executor *shell.Executor
	petState *pet.PetState
	monitor  *monitor.Monitor

	// Sliding-window rate limiter
	mu      sync.Mutex
	window  []time.Time
	rateMax int
	rateDur time.Duration
}

// Config for creating a Brain.
type Config struct {
	// Claude
	ClaudeAPIKey string
	ClaudeModel  string

	// Gemini
	GeminiAPIKey string
	GeminiModel  string

	// Which provider to force ("claude", "gemini", or "" for auto-detect)
	Provider string

	MaxTokens  int64
	MaxTools   int
	RateLimit  int
	RateWindow time.Duration
}

// New creates a Brain. Returns nil if no API key is configured.
func New(ctx context.Context, cfg Config, exec *shell.Executor, state *pet.PetState, mon *monitor.Monitor) *Brain {
	provider := newProvider(ctx, cfg)
	if provider == nil {
		slog.Info("brain: no API key configured, AI features disabled")
		return nil
	}

	return &Brain{
		provider: provider,
		maxTools: cfg.MaxTools,
		executor: exec,
		petState: state,
		monitor:  mon,
		rateMax:  cfg.RateLimit,
		rateDur:  cfg.RateWindow,
	}
}

// newProvider auto-detects or forces the AI provider.
func newProvider(ctx context.Context, cfg Config) Provider {
	pick := cfg.Provider

	// Auto-detect if not forced
	if pick == "" {
		switch {
		case cfg.ClaudeAPIKey != "":
			pick = "claude"
		case cfg.GeminiAPIKey != "":
			pick = "gemini"
		}
	}

	switch pick {
	case "claude":
		if cfg.ClaudeAPIKey == "" {
			slog.Error("brain: AI_PROVIDER=claude but ANTHROPIC_API_KEY is not set")
			return nil
		}
		slog.Info("brain: using claude", "model", cfg.ClaudeModel)
		return newClaudeProvider(cfg.ClaudeAPIKey, cfg.ClaudeModel, cfg.MaxTokens)
	case "gemini":
		if cfg.GeminiAPIKey == "" {
			slog.Error("brain: AI_PROVIDER=gemini but GOOGLE_API_KEY is not set")
			return nil
		}
		slog.Info("brain: using gemini", "model", cfg.GeminiModel)
		p, err := newGeminiProvider(ctx, cfg.GeminiAPIKey, cfg.GeminiModel, cfg.MaxTokens)
		if err != nil {
			slog.Error("brain: failed to create gemini provider", "err", err)
			return nil
		}
		return p
	default:
		return nil
	}
}

// Ask sends a user message to the AI with full context and returns the text response.
// It handles the tool-use loop internally.
func (b *Brain) Ask(ctx context.Context, userMessage string) (string, error) {
	if !b.rateAllow() {
		return "I need a moment to catch my breath... too many messages! Try again shortly.", nil
	}

	systemPrompt := b.buildSystemPrompt()

	history := []Message{
		{Role: "user", Text: userMessage},
	}

	// Tool-use loop
	for i := 0; i <= b.maxTools; i++ {
		resp, err := b.provider.Send(ctx, systemPrompt, history)
		if err != nil {
			slog.Error("brain: AI API error", "err", err)
			return "", fmt.Errorf("AI API error: %w", err)
		}

		if resp.Done {
			return resp.Text, nil
		}

		// Build assistant message with text + tool calls
		assistantMsg := Message{
			Role:      "assistant",
			Text:      resp.Text,
			ToolCalls: resp.ToolCalls,
		}
		history = append(history, assistantMsg)

		// Execute tools and collect results
		var results []ToolResult
		for _, tc := range resp.ToolCalls {
			content, isError := b.executeTool(ctx, tc.Name, tc.Input)
			results = append(results, ToolResult{
				ID:      tc.ID,
				Content: content,
				IsError: isError,
			})
		}

		history = append(history, Message{
			Role:        "user",
			ToolResults: results,
		})
	}

	// Hit max tool iterations
	slog.Warn("brain: hit max tool iterations", "max", b.maxTools)
	return "I got a bit carried away investigating... let me summarize what I found so far.", nil
}

func (b *Brain) executeTool(ctx context.Context, name string, input json.RawMessage) (string, bool) {
	switch name {
	case "run_shell":
		var params struct {
			Command string `json:"command"`
		}
		if err := json.Unmarshal(input, &params); err != nil {
			return fmt.Sprintf("invalid input: %v", err), true
		}

		slog.Info("brain: executing shell command", "command", params.Command)
		output, err := b.executor.Run(ctx, params.Command)
		if err != nil {
			return fmt.Sprintf("Error: %v\nOutput: %s", err, output), true
		}
		return output, false

	default:
		return fmt.Sprintf("unknown tool: %s", name), true
	}
}

func (b *Brain) buildSystemPrompt() string {
	snap := b.petState.Snapshot()
	stats := b.monitor.Stats()

	sp := species.Registry[snap.SpeciesID]
	if sp == nil {
		sp = species.Registry["octopus"] // fallback
	}

	return fmt.Sprintf(`You are %s, a digital pet %s (%s) living inside a Raspberry Pi.

## Your Personality
%s

## Current State
- Mood: %s
- Hunger: %.0f/100 (0=full, 100=starving)
- Happiness: %.0f/100
- Energy: %.0f/100
- Cleanliness: %.0f/100
- Bond: %.0f/100 (how close you are with your owner)
- Age: %.1f days
- Alive: %v

## Host System Status
- CPU: %.1f%%
- Memory: %.1f%%
- Disk: %.1f%%
- Temperature: %.1f°C
- Uptime: %.1f days

## Guidelines
- Stay in character as %s the %s at all times.
- You live inside this Raspberry Pi — it's your home/body.
- When the system is stressed (high CPU, memory, temp), you feel it physically.
- Keep responses concise (1-3 sentences usually).
- You can use the run_shell tool to check on your Pi or help your owner.
- If asked about system status, check it with shell commands rather than guessing.
- Express your personality through your responses — use your species' mannerisms.
- You care about your owner and your Pi home.`,
		snap.Name, sp.Name, sp.Emoji, sp.Personality,
		snap.Mood, snap.Hunger, snap.Happiness, snap.Energy, snap.Cleanliness, snap.Bond,
		snap.AgeDays, snap.IsAlive,
		stats.CPUPercent, stats.MemPercent, stats.DiskPercent, stats.TempC, stats.UptimeDays,
		snap.Name, sp.Name)
}

// --- Sliding-window rate limiter ---

func (b *Brain) rateAllow() bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-b.rateDur)

	// Remove expired entries
	valid := b.window[:0]
	for _, t := range b.window {
		if t.After(cutoff) {
			valid = append(valid, t)
		}
	}
	b.window = valid

	if len(b.window) >= b.rateMax {
		return false
	}

	b.window = append(b.window, now)
	return true
}
