package apiclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestMessagesEndpoint(t *testing.T) {
	tests := []struct {
		base    string
		want    string
		wantErr bool
	}{
		{base: "https://api.anthropic.com", want: "https://api.anthropic.com/v1/messages"},
		{base: "https://relay.example/v1", want: "https://relay.example/v1/messages"},
		{base: "https://relay.example/v1/messages", want: "https://relay.example/v1/messages"},
		{base: "http://localhost:8080/v1", wantErr: true},
	}
	for _, tt := range tests {
		got, err := MessagesEndpoint(tt.base, false)
		if (err != nil) != tt.wantErr {
			t.Fatalf("MessagesEndpoint(%q) error = %v", tt.base, err)
		}
		if got != tt.want {
			t.Fatalf("MessagesEndpoint(%q) = %q, want %q", tt.base, got, tt.want)
		}
	}
}

func TestCallAnthropicRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			t.Errorf("path = %q", r.URL.Path)
		}
		if got := r.Header.Get("x-api-key"); got != "test-key" {
			t.Errorf("x-api-key = %q", got)
		}
		if got := r.Header.Get("anthropic-version"); got != anthropicVersion {
			t.Errorf("anthropic-version = %q", got)
		}
		var body anthropicRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if len(body.Tools) != 1 || body.Tools[0].Type != "web_fetch_20250910" || body.Tools[0].Name != "web_fetch" {
			t.Errorf("tools = %+v", body.Tools)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"msg_test","model":"claude-test","content":[]}`))
	}))
	defer server.Close()

	result := CallAnthropic(context.Background(), server.URL, "test-key", "claude-test", "fetch", true, 5*time.Second)
	if result.Error != "" || result.StatusCode != http.StatusOK {
		t.Fatalf("result = %+v", result)
	}
	if result.ReportedModel != "claude-test" || result.ResponseID != "msg_test" {
		t.Fatalf("metadata = %+v", result)
	}
}

func TestResolveAdapter(t *testing.T) {
	for _, tt := range []struct {
		requested string
		baseURL   string
		model     string
		want      string
	}{
		{requested: "auto", baseURL: "https://api.anthropic.com", model: "anything", want: AdapterAnthropicMessages},
		{requested: "auto", baseURL: "https://relay.example", model: "claude-sonnet-4", want: AdapterAnthropicMessages},
		{requested: "auto", baseURL: "https://relay.example", model: "gpt-5", want: AdapterOpenAIResponses},
		{requested: "openai-responses", baseURL: "https://relay.example", model: "claude-alias", want: AdapterOpenAIResponses},
	} {
		got, err := ResolveAdapter(tt.requested, tt.baseURL, tt.model)
		if err != nil || got != tt.want {
			t.Fatalf("ResolveAdapter(%q, %q, %q) = %q, %v; want %q", tt.requested, tt.baseURL, tt.model, got, err, tt.want)
		}
	}
}
