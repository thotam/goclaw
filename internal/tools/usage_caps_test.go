package tools

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/nextlevelbuilder/goclaw/internal/providers"
	"github.com/nextlevelbuilder/goclaw/internal/store"
	usagecaps "github.com/nextlevelbuilder/goclaw/internal/usage/caps"
)

// bigContent builds a user message string that exceeds `targetChars` words under
// the caps guard's fixed BudgetCounter, so the guard math is predictable here.
func bigContent(targetChars int) string {
	return strings.Repeat("word ", targetChars)
}

// withToolAgentBudget mirrors what the agent loop's injectContext does before a
// tool runs: it propagates the CALLING agent's context window and max_tokens.
func withToolAgentBudget(window, maxTokens int) context.Context {
	ctx := store.WithAgentContextWindow(context.Background(), window)
	return store.WithAgentMaxTokens(ctx, maxTokens)
}

// TestReserveToolLLMUsage_HonorsAgentWindowFromContext is the production-path
// regression guard: reserveToolLLMUsage reads the CALLING agent's budget from
// ctx (set by injectContext via store.WithAgentContextWindow /
// store.WithAgentMaxTokens) and enforces completeInput + max_tokens <= window.
// Model/provider are never budget authorities.
func TestReserveToolLLMUsage_HonorsAgentWindowFromContext(t *testing.T) {
	model := "claude-sonnet-4-5-20250929" // 200k model window — must NOT matter
	req := providers.ChatRequest{
		Model:    model,
		Messages: []providers.Message{{Role: "user", Content: bigContent(40_000)}},
		Options:  map[string]any{providers.OptMaxTokens: 4096},
	}

	// A 200k agent window admits ~40k tokens of prompt.
	ctxBig := withToolAgentBudget(200_000, 8_192)
	if _, err := reserveToolLLMUsage(ctxBig, nil, "read_document", "anthropic", model, req); err != nil {
		t.Fatalf("expected allow under 200k agent window, got %v", err)
	}

	// A 20k agent window must block the SAME request before transport.
	ctxSmall := withToolAgentBudget(20_000, 8_192)
	_, err := reserveToolLLMUsage(ctxSmall, nil, "read_document", "anthropic", model, req)
	if err == nil {
		t.Fatal("expected abort when agent window (20k) is below the request size")
	}
	var ctxErr *usagecaps.ContextWindowExceededError
	if !errors.As(err, &ctxErr) {
		t.Fatalf("expected *ContextWindowExceededError, got %T: %v", err, err)
	}
	if ctxErr.ContextWindow != 20_000 {
		t.Fatalf("guard used window %d, want 20000 (agent cap, not model window)", ctxErr.ContextWindow)
	}
}

// TestReserveToolLLMUsage_FailsClosedWithoutAgentBudget proves an agent-scoped
// tool call reaching the model gate WITHOUT a propagated budget fails closed
// with a wiring error before any transport — it must never silently guess a
// model window.
func TestReserveToolLLMUsage_FailsClosedWithoutAgentBudget(t *testing.T) {
	req := providers.ChatRequest{
		Model:    "gpt-4o",
		Messages: []providers.Message{{Role: "user", Content: "tiny prompt"}},
		Options:  map[string]any{providers.OptMaxTokens: 4096},
	}
	_, err := reserveToolLLMUsage(context.Background(), nil, "read_document", "openai", "gpt-4o", req)
	if err == nil {
		t.Fatal("expected wiring error without a propagated agent budget")
	}
	var wiringErr *usagecaps.AgentBudgetWiringError
	if !errors.As(err, &wiringErr) {
		t.Fatalf("expected *AgentBudgetWiringError, got %T: %v", err, err)
	}
}

// TestReserveToolLLMUsageWithMedia_ChargesFlatUnitPerItem proves an out-of-band
// payload is part of completeInput at a flat per-item cost, independent of its
// byte size. Size independence is the point: counting bytes made every
// real-world video exceed every window.
func TestReserveToolLLMUsageWithMedia_ChargesFlatUnitPerItem(t *testing.T) {
	model := "gpt-4o"
	req := func() providers.ChatRequest {
		return providers.ChatRequest{
			Model:    model,
			Messages: []providers.Message{{Role: "user", Content: "Transcribe this."}},
			Options:  map[string]any{providers.OptMaxTokens: 4096},
		}
	}

	// One media item fits a large window, whatever the payload behind it.
	ctxBig := withToolAgentBudget(128_000, 8_192)
	if _, err := reserveToolLLMUsageWithMedia(ctxBig, nil, "read_document", "openai", model, req(), 1); err != nil {
		t.Fatalf("one media item must fit a 128k window, got %v", err)
	}

	// The charge is real: enough items against a small window abort before
	// transport.
	ctxSmall := withToolAgentBudget(20_000, 8_192)
	_, err := reserveToolLLMUsageWithMedia(ctxSmall, nil, "read_document", "openai", model, req(), 8)
	if err == nil {
		t.Fatal("expected abort: 8 media items plus an 8192 output reserve exceed a 20k window")
	}
	var ctxErr *usagecaps.ContextWindowExceededError
	if !errors.As(err, &ctxErr) {
		t.Fatalf("expected *ContextWindowExceededError, got %T: %v", err, err)
	}
	if ctxErr.ContextWindow != 20_000 {
		t.Fatalf("guard used window %d, want 20000 (agent cap)", ctxErr.ContextWindow)
	}
}
