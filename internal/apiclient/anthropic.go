package apiclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const anthropicVersion = "2023-06-01"

type anthropicRequest struct {
	Model     string             `json:"model"`
	MaxTokens int                `json:"max_tokens"`
	Messages  []anthropicMessage `json:"messages"`
	Tools     []anthropicTool    `json:"tools"`
}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type anthropicTool struct {
	Type    string `json:"type"`
	Name    string `json:"name"`
	MaxUses int    `json:"max_uses"`
}

func CallAnthropic(ctx context.Context, baseURL, apiKey, model, prompt string, allowHTTP bool, timeout time.Duration) Result {
	endpoint, err := MessagesEndpoint(baseURL, allowHTTP)
	if err != nil {
		return Result{Adapter: AdapterAnthropicMessages, Error: err.Error()}
	}
	result := Result{Adapter: AdapterAnthropicMessages, Endpoint: endpoint}
	payload, err := json.Marshal(anthropicRequest{
		Model:     model,
		MaxTokens: 1024,
		Messages:  []anthropicMessage{{Role: "user", Content: prompt}},
		Tools:     []anthropicTool{{Type: "web_fetch_20250910", Name: "web_fetch", MaxUses: 1}},
	})
	if err != nil {
		result.Error = err.Error()
		return result
	}

	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		result.Error = err.Error()
		return result
	}
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", anthropicVersion)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "llmfp/0.1")

	client := &http.Client{
		Timeout: timeout,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return errors.New("redirects are disabled for API requests")
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	defer resp.Body.Close()
	result.StatusCode = resp.StatusCode
	result.ContentType = resp.Header.Get("Content-Type")
	result.RawBody, err = io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		result.Error = fmt.Sprintf("read API response: %v", err)
		return result
	}
	if len(result.RawBody) > maxResponseBytes {
		result.RawBody = result.RawBody[:maxResponseBytes]
		result.Error = "API response exceeded 4 MiB and was truncated"
	}
	var envelope struct {
		ID    string `json:"id"`
		Model string `json:"model"`
	}
	if json.Unmarshal(result.RawBody, &envelope) == nil {
		result.ResponseID = envelope.ID
		result.ReportedModel = envelope.Model
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		result.Error = fmt.Sprintf("API returned HTTP %d", resp.StatusCode)
	} else if !isJSONContentType(result.ContentType) {
		result.Error = fmt.Sprintf("API returned unexpected Content-Type %q; check that --base-url points to the Anthropic API base", result.ContentType)
	}
	return result
}

func MessagesEndpoint(baseURL string, allowHTTP bool) (string, error) {
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return "", fmt.Errorf("parse --base-url: %w", err)
	}
	if parsed.Scheme != "https" && !(allowHTTP && parsed.Scheme == "http") {
		return "", errors.New("--base-url must use https:// (or pass --allow-http)")
	}
	if parsed.Host == "" || parsed.User != nil {
		return "", errors.New("--base-url must contain a host and no embedded credentials")
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	path := strings.TrimRight(parsed.Path, "/")
	switch {
	case strings.HasSuffix(path, "/messages"):
	case path == "":
		path = "/v1/messages"
	default:
		path += "/messages"
	}
	parsed.Path = path
	return parsed.String(), nil
}
