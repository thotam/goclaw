package tools

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/nextlevelbuilder/goclaw/internal/providers"
	"github.com/nextlevelbuilder/goclaw/internal/store"
	"github.com/nextlevelbuilder/goclaw/internal/tokencount"
	usagecaps "github.com/nextlevelbuilder/goclaw/internal/usage/caps"
)

func agentBudgetFromContext(ctx context.Context) usagecaps.AgentBudget {
	return usagecaps.AgentBudget{
		ContextWindow: store.AgentContextWindowFromContext(ctx),
		MaxTokens:     store.AgentMaxTokensFromContext(ctx),
	}
}

// reserveToolLLMUsage guards and reserves a tool-internal model call whose full
// input already lives in the ChatRequest. The fixed local BudgetCounter counts
// the request itself.
func reserveToolLLMUsage(ctx context.Context, svc *usagecaps.Service, toolName, providerName, model string, req providers.ChatRequest) (*usagecaps.Reservation, error) {
	return reserveToolLLMUsageWithMedia(ctx, svc, toolName, providerName, model, req, 0)
}

// reserveToolLLMUsageWithMedia guards a native-media tool call whose payload is
// sent OUT-OF-BAND (native provider JSON body or File API upload) and is thus
// invisible to the ChatRequest the counter would otherwise see. Each payload is
// charged the flat tokencount.InlineMediaUnit the counter already applies to
// structured message media, so inline and out-of-band transports agree on one
// number and no path has to buffer bytes in order to be countable.
func reserveToolLLMUsageWithMedia(ctx context.Context, svc *usagecaps.Service, toolName, providerName, model string, req providers.ChatRequest, mediaItems int) (*usagecaps.Reservation, error) {
	mediaTokens := 0
	if mediaItems > 0 {
		mediaTokens = mediaItems * tokencount.InlineMediaUnit
	}
	budget := agentBudgetFromContext(ctx)
	req = clampToolRequestMaxTokens(req, budget.MaxTokens)
	if guardErr := usagecaps.GuardContextWindowWithMediaTokens(req, providerName, model, "tool:"+toolName, budget, mediaTokens); guardErr != nil {
		return nil, guardErr
	}
	if svc == nil {
		return nil, nil
	}
	return svc.Preflight(ctx, usagecaps.Request{
		TenantID:         store.TenantIDFromContext(ctx),
		AgentID:          store.AgentIDFromContext(ctx),
		ProviderName:     providerName,
		ModelID:          model,
		ReservationKey:   fmt.Sprintf("tool:%s:%s", toolName, uuid.NewString()),
		Messages:         req.Messages,
		MaxOutputTokens:  budget.MaxTokens,
		ExtraInputTokens: mediaTokens,
	})
}

// clampToolRequestMaxTokens enforces max_tokens <= agentMaxTokens for every
// agent-originated tool call. A request that does not declare max_tokens is set
// to the agent's max_tokens rather than left to a provider default, so the
// window invariant (completeInput + agentMaxTokens <= window) holds for the
// value actually sent.
func clampToolRequestMaxTokens(req providers.ChatRequest, agentMaxTokens int) providers.ChatRequest {
	if agentMaxTokens <= 0 {
		return req
	}
	if req.Options == nil {
		req.Options = map[string]any{}
	}
	current, ok := maxOutputTokensDeclared(req.Options)
	if !ok || current > agentMaxTokens {
		req.Options[providers.OptMaxTokens] = agentMaxTokens
	}
	return req
}

// maxOutputTokensDeclared reports the max_tokens declared in the request options
// and whether it was present at all. The bool distinguishes a genuinely missing
// option (ok == false) from an explicit zero, which the clamp needs so it can
// set the agent's max_tokens when the caller declared nothing.
func maxOutputTokensDeclared(options map[string]any) (int, bool) {
	if options == nil {
		return 0, false
	}
	v, ok := options[providers.OptMaxTokens]
	if !ok {
		return 0, false
	}
	switch n := v.(type) {
	case int:
		return n, true
	case int64:
		return int(n), true
	case int32:
		return int(n), true
	case float64:
		return int(n), true
	case float32:
		return int(n), true
	default:
		return 0, false
	}
}

func maxOutputTokensFromOptions(options map[string]any) int {
	maxTokens, _ := maxOutputTokensDeclared(options)
	return maxTokens
}
