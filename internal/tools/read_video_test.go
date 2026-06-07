package tools

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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

func TestReadVideo_GeminiURL_Validation(t *testing.T) {
	// 1. Trường hợp không có Content-Length (ContentLength <= 0)
	ts1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Transfer-Encoding", "chunked")
		w.Write([]byte("chunked data mock video"))
	}))
	defer ts1.Close()

	tool := NewReadVideoTool(nil, nil)
	cp := &mockCredentialProvider{apiKey: "test-key"}

	params1 := map[string]any{
		"prompt":         "describe this video",
		"url":            ts1.URL,
		"_provider_type": "gemini",
	}

	_, _, err := tool.callProvider(context.Background(), cp, "gemini", "gemini-2.5-flash", params1)
	if err == nil {
		t.Fatalf("expected error for missing Content-Length")
	}
	if !strings.Contains(err.Error(), "URL does not support static streaming") {
		t.Errorf("unexpected error for missing Content-Length: %v", err)
	}

	// 2. Trường hợp Content-Length vượt quá 2 GB
	ts2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "2147483649") // 2GB + 1 byte
		w.WriteHeader(http.StatusOK)
	}))
	defer ts2.Close()

	params2 := map[string]any{
		"prompt":         "describe this video",
		"url":            ts2.URL,
		"_provider_type": "gemini",
	}

	_, _, err = tool.callProvider(context.Background(), cp, "gemini", "gemini-2.5-flash", params2)
	if err == nil {
		t.Fatalf("expected error for Content-Length exceeding 2GB")
	}
	if !strings.Contains(err.Error(), "exceeds the maximum limit of 2 GB") {
		t.Errorf("unexpected error for limit exceed: %v", err)
	}

	// 3. Trường hợp HTTP status code lỗi (ví dụ 404)
	ts3 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts3.Close()

	params3 := map[string]any{
		"prompt":         "describe this video",
		"url":            ts3.URL,
		"_provider_type": "gemini",
	}

	_, _, err = tool.callProvider(context.Background(), cp, "gemini", "gemini-2.5-flash", params3)
	if err == nil {
		t.Fatalf("expected error for HTTP 404 status code")
	}
	if !strings.Contains(err.Error(), "video URL returned status code 404") {
		t.Errorf("unexpected error for HTTP status code: %v", err)
	}
}
