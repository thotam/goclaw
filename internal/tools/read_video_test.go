package tools

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nextlevelbuilder/goclaw/internal/security"
	"github.com/nextlevelbuilder/goclaw/internal/store"
)

type mockCredentialProvider struct {
	apiKey  string
	apiBase string
}

func (m *mockCredentialProvider) APIKey() string  { return m.apiKey }
func (m *mockCredentialProvider) APIBase() string { return m.apiBase }

func TestReadVideo_BothMediaIdAndUrl_Error(t *testing.T) {
	tool := NewReadVideoTool(nil, nil)

	res := tool.Execute(context.Background(), map[string]any{
		"prompt":   "describe this video",
		"media_id": "video-123",
		"url":      "https://example.com/video.mp4",
	})

	if !res.IsError {
		t.Fatalf("expected error when both media_id and url are provided")
	}

	if !strings.Contains(res.ForLLM, "Both 'media_id' and 'url' parameters cannot be specified") {
		t.Errorf("unexpected error message: %s", res.ForLLM)
	}
}

func TestReadVideo_PrivateURL_Error(t *testing.T) {
	tool := NewReadVideoTool(nil, nil)

	res := tool.Execute(context.Background(), map[string]any{
		"prompt": "describe this video",
		"url":    "http://127.0.0.1/video.mp4",
	})

	if !res.IsError {
		t.Fatalf("expected error for private video URL")
	}
	if !strings.Contains(res.ForLLM, "Invalid video URL") {
		t.Errorf("unexpected error message: %s", res.ForLLM)
	}
}

// TestReadVideo_GeminiURL_ReachesTransportUnderAgentBudget locks the contract for
// a streamed video URL: the reservation charges one flat media unit rather than
// trying to count bytes it does not hold, so the call proceeds to transport
// instead of refusing. The server answers 500 so the flow stops at the status
// check, well before any provider call.
func TestReadVideo_GeminiURL_ReachesTransportUnderAgentBudget(t *testing.T) {
	security.SetAllowLoopbackForTest(true)
	defer security.SetAllowLoopbackForTest(false)

	hits := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	tool := NewReadVideoTool(nil, nil)
	cp := &mockCredentialProvider{apiKey: "test-key"}

	ctx := store.WithAgentContextWindow(context.Background(), 200_000)
	ctx = store.WithAgentMaxTokens(ctx, 32_000)

	params := map[string]any{
		"prompt":         "describe this video",
		"url":            ts.URL,
		"_provider_type": "gemini",
	}

	_, _, err := tool.callProvider(ctx, cp, "gemini", "gemini-2.5-flash", params)
	if err == nil {
		t.Fatal("expected the 500 response to surface as an error")
	}
	if strings.Contains(err.Error(), "cannot verify streamed native media") {
		t.Fatalf("the reservation must no longer fail closed on a streamed URL: %v", err)
	}
	if !strings.Contains(err.Error(), "status code 500") {
		t.Fatalf("expected the transport status check to reject, got: %v", err)
	}
	if hits != 1 {
		t.Fatalf("the URL was contacted %d time(s), want 1", hits)
	}
}

// TestReadVideo_GeminiURL_RejectsMissingContentLength covers the Content-Length
// check in read_video_resolve.go, which was dead code while the fail-closed
// reservation refused every streamed URL before transport.
func TestReadVideo_GeminiURL_RejectsMissingContentLength(t *testing.T) {
	security.SetAllowLoopbackForTest(true)
	defer security.SetAllowLoopbackForTest(false)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK) // empty body: Go sends Content-Length: 0
	}))
	defer ts.Close()

	tool := NewReadVideoTool(nil, nil)
	cp := &mockCredentialProvider{apiKey: "test-key"}
	ctx := store.WithAgentContextWindow(context.Background(), 200_000)
	ctx = store.WithAgentMaxTokens(ctx, 32_000)

	_, _, err := tool.callProvider(ctx, cp, "gemini", "gemini-2.5-flash", map[string]any{
		"prompt": "describe this video", "url": ts.URL, "_provider_type": "gemini",
	})
	if err == nil || !strings.Contains(err.Error(), "does not support static streaming") {
		t.Fatalf("expected the Content-Length check to reject, got: %v", err)
	}
}

// TestReadVideo_GeminiURL_RejectsOversizedStream covers the 2 GB stream ceiling
// in read_video_resolve.go, which was dead code while the fail-closed
// reservation refused every streamed URL before transport.
func TestReadVideo_GeminiURL_RejectsOversizedStream(t *testing.T) {
	security.SetAllowLoopbackForTest(true)
	defer security.SetAllowLoopbackForTest(false)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "3221225472") // 3 GB, over the 2 GB ceiling
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	tool := NewReadVideoTool(nil, nil)
	cp := &mockCredentialProvider{apiKey: "test-key"}
	ctx := store.WithAgentContextWindow(context.Background(), 200_000)
	ctx = store.WithAgentMaxTokens(ctx, 32_000)

	_, _, err := tool.callProvider(ctx, cp, "gemini", "gemini-2.5-flash", map[string]any{
		"prompt": "describe this video", "url": ts.URL, "_provider_type": "gemini",
	})
	if err == nil || !strings.Contains(err.Error(), "exceeds the maximum limit of 2 GB") {
		t.Fatalf("expected the 2 GB ceiling to reject, got: %v", err)
	}
}
