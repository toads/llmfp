package apiclient

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"
)

const (
	AdapterAuto              = "auto"
	AdapterOpenAIResponses   = "openai-responses"
	AdapterAnthropicMessages = "anthropic-messages"
)

type Result struct {
	Adapter       string
	Endpoint      string
	StatusCode    int
	ContentType   string
	ResponseID    string
	ReportedModel string
	RawBody       []byte
	Error         string
}

func ResolveAdapter(requested, baseURL, model string) (string, error) {
	switch requested {
	case "", AdapterAuto:
		parsed, _ := url.Parse(baseURL)
		if strings.EqualFold(parsed.Hostname(), "api.anthropic.com") || strings.HasPrefix(strings.ToLower(model), "claude-") {
			return AdapterAnthropicMessages, nil
		}
		return AdapterOpenAIResponses, nil
	case AdapterOpenAIResponses, "openai":
		return AdapterOpenAIResponses, nil
	case AdapterAnthropicMessages, "anthropic":
		return AdapterAnthropicMessages, nil
	default:
		return "", fmt.Errorf("unsupported adapter %q; use auto, openai-responses, or anthropic-messages", requested)
	}
}

func Endpoint(adapter, baseURL string, allowHTTP bool) (string, error) {
	switch adapter {
	case AdapterOpenAIResponses:
		return ResponsesEndpoint(baseURL, allowHTTP)
	case AdapterAnthropicMessages:
		return MessagesEndpoint(baseURL, allowHTTP)
	default:
		return "", fmt.Errorf("unsupported adapter %q", adapter)
	}
}

func Call(ctx context.Context, adapter, baseURL, apiKey, model, prompt string, allowHTTP bool, timeout time.Duration) Result {
	switch adapter {
	case AdapterOpenAIResponses:
		return CallResponses(ctx, baseURL, apiKey, model, prompt, allowHTTP, timeout)
	case AdapterAnthropicMessages:
		return CallAnthropic(ctx, baseURL, apiKey, model, prompt, allowHTTP, timeout)
	default:
		return Result{Adapter: adapter, Error: fmt.Sprintf("unsupported adapter %q", adapter)}
	}
}
