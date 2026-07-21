package apiclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestResponsesEndpoint(t *testing.T) {
	tests := []struct {
		name      string
		base      string
		allowHTTP bool
		want      string
		wantErr   bool
	}{
		{name: "origin root", base: "https://relay.example", want: "https://relay.example/v1/responses"},
		{name: "origin slash", base: "https://relay.example/", want: "https://relay.example/v1/responses"},
		{name: "base v1", base: "https://relay.example/v1", want: "https://relay.example/v1/responses"},
		{name: "full endpoint", base: "https://relay.example/v1/responses", want: "https://relay.example/v1/responses"},
		{name: "strip query", base: "https://relay.example/v1?x=1", want: "https://relay.example/v1/responses"},
		{name: "http rejected", base: "http://localhost:8080/v1", wantErr: true},
		{name: "http allowed", base: "http://localhost:8080/v1", allowHTTP: true, want: "http://localhost:8080/v1/responses"},
		{name: "embedded credentials", base: "https://user:pass@relay.example/v1", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResponsesEndpoint(tt.base, tt.allowHTTP)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ResponsesEndpoint() error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("ResponsesEndpoint() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestJSONContentType(t *testing.T) {
	for _, value := range []string{"application/json", "application/json; charset=utf-8", "application/problem+json"} {
		if !isJSONContentType(value) {
			t.Fatalf("isJSONContentType(%q) = false", value)
		}
	}
	for _, value := range []string{"", "text/html", "text/plain; charset=utf-8"} {
		if isJSONContentType(value) {
			t.Fatalf("isJSONContentType(%q) = true", value)
		}
	}
}

func TestCallResponsesRejectsHTMLSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" {
			t.Errorf("request path = %q, want /v1/responses", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte("<html>frontend fallback</html>"))
	}))
	defer server.Close()

	result := CallResponses(context.Background(), server.URL, "test-key", "test-model", "test", true, 5*time.Second)
	if result.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", result.StatusCode)
	}
	if !strings.Contains(result.Error, "unexpected Content-Type") {
		t.Fatalf("error = %q", result.Error)
	}
}
