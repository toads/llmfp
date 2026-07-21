package run

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/toads/llmfp/internal/probe"
)

func TestReportSchemaVersion(t *testing.T) {
	encoded, err := json.Marshal(Report{SchemaVersion: SchemaVersion})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(encoded), `{"schema_version":"1"`) {
		t.Fatalf("report JSON does not start with schema version: %s", encoded)
	}
}

func TestSummarizeHTTPEvent(t *testing.T) {
	event := &probe.Interaction{
		Protocol:      "http",
		RemoteAddress: "52.231.49.54",
		RawRequest:    "GET /llmfp-abc HTTP/2.0\r\nHost: example.test\r\nUser-Agent: ChatGPT-User/1.0\r\n\r\n",
	}
	report, observation := summarizeEvent(event, "/llmfp-abc")
	if observation == nil {
		t.Fatal("observation is nil")
	}
	if !report.ExactPath || report.Path != "/llmfp-abc" {
		t.Fatalf("unexpected report: %+v", report)
	}
	if observation.SourceIP.String() != "52.231.49.54" || observation.UserAgent != "ChatGPT-User/1.0" {
		t.Fatalf("unexpected observation: %+v", observation)
	}
}
