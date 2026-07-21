package run

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"testing"
	"time"
)

var probeURLPattern = regexp.MustCompile(`https://[^\s]+/llmfp-[a-f0-9]+`)

func TestPublicOASTEndToEnd(t *testing.T) {
	if os.Getenv("LLMFP_INTEGRATION") != "1" {
		t.Skip("set LLMFP_INTEGRATION=1 to use the public oast.pro service")
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" {
			http.NotFound(w, r)
			return
		}
		var request struct {
			Input string `json:"input"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		probeURL := probeURLPattern.FindString(request.Input)
		if probeURL == "" {
			http.Error(w, "probe URL missing", http.StatusBadRequest)
			return
		}
		probeRequest, err := http.NewRequest(http.MethodGet, probeURL, nil)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		probeRequest.Header.Set("User-Agent", "llmfp-integration-test/1.0")
		response, err := http.DefaultClient.Do(probeRequest)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		defer response.Body.Close()
		body, err := io.ReadAll(response.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "resp_integration", "model": "mock-model", "output_text": string(body),
		})
	}))
	defer server.Close()

	report, err := Test(Config{
		Adapter: "openai-responses", BaseURL: server.URL + "/v1", APIKey: "integration-only", Model: "mock-model",
		ProbeServer: "oast.pro", CallbackWait: 15 * time.Second,
		RequestTimeout: 20 * time.Second, RegistryTimeout: 10 * time.Second, AllowHTTP: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.API.StatusCode != http.StatusOK {
		t.Fatalf("API status = %d, error = %s", report.API.StatusCode, report.API.Error)
	}
	if !report.API.ChallengeReturned {
		t.Fatalf("dynamic Interactsh response was not found in mock API response: %+v", report.API)
	}
	if !report.Attribution.ExactPathFetched {
		t.Fatalf("exact probe path was not observed: %+v", report.Events)
	}
}
