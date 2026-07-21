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

const maxResponseBytes = 4 << 20

type responsesRequest struct {
	Model string `json:"model"`
	Tools []tool `json:"tools"`
	Input string `json:"input"`
}

type tool struct {
	Type string `json:"type"`
}

func CallResponses(ctx context.Context, baseURL, apiKey, model, prompt string, allowHTTP bool, timeout time.Duration) Result {
	endpoint, err := ResponsesEndpoint(baseURL, allowHTTP)
	if err != nil {
		return Result{Error: err.Error()}
	}
	result := Result{Adapter: AdapterOpenAIResponses, Endpoint: endpoint}

	payload, err := json.Marshal(responsesRequest{
		Model: model,
		Tools: []tool{{Type: "web_search"}},
		Input: prompt,
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
	req.Header.Set("Authorization", "Bearer "+apiKey)
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
		Error any    `json:"error"`
	}
	if json.Unmarshal(result.RawBody, &envelope) == nil {
		result.ResponseID = envelope.ID
		result.ReportedModel = envelope.Model
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		result.Error = fmt.Sprintf("API returned HTTP %d", resp.StatusCode)
	} else if !isJSONContentType(result.ContentType) {
		result.Error = fmt.Sprintf("API returned unexpected Content-Type %q; check that --base-url points to the API base, normally ending in /v1", result.ContentType)
	}
	return result
}

func ResponsesEndpoint(baseURL string, allowHTTP bool) (string, error) {
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
	case strings.HasSuffix(path, "/responses"):
	case path == "":
		path = "/v1/responses"
	default:
		path += "/responses"
	}
	parsed.Path = path
	return parsed.String(), nil
}

func isJSONContentType(value string) bool {
	mediaType := strings.ToLower(strings.TrimSpace(strings.SplitN(value, ";", 2)[0]))
	return mediaType == "application/json" || strings.HasSuffix(mediaType, "+json")
}
