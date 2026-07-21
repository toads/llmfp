package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestAutomaticAPIKeyEnv(t *testing.T) {
	t.Setenv("LLMFP_API_KEY", "")
	t.Setenv("RELAY_API_KEY", "relay-secret")
	t.Setenv("OPENAI_API_KEY", "openai-secret")
	t.Setenv("ANTHROPIC_API_KEY", "anthropic-secret")
	if got := automaticAPIKeyEnv("https://relay.example/v1"); got != "RELAY_API_KEY" {
		t.Fatalf("automaticAPIKeyEnv() = %q, want RELAY_API_KEY", got)
	}

	t.Setenv("RELAY_API_KEY", "")
	if got := automaticAPIKeyEnv("https://api.openai.com/v1"); got != "OPENAI_API_KEY" {
		t.Fatalf("automaticAPIKeyEnv() = %q, want OPENAI_API_KEY", got)
	}
	if got := automaticAPIKeyEnv("https://relay.example/v1"); got != "" {
		t.Fatalf("automaticAPIKeyEnv() sent OPENAI_API_KEY to relay: %q", got)
	}
	if got := automaticAPIKeyEnv("https://api.anthropic.com"); got != "ANTHROPIC_API_KEY" {
		t.Fatalf("automaticAPIKeyEnv() = %q, want ANTHROPIC_API_KEY", got)
	}
}

func TestExplicitEnvironmentAPIKeyIsNotPrinted(t *testing.T) {
	t.Setenv("TEST_LLMFP_KEY", "secret-value")
	var stderr bytes.Buffer
	key, err := readAPIKey("TEST_LLMFP_KEY", false, &stderr)
	if err != nil {
		t.Fatal(err)
	}
	if key != "secret-value" {
		t.Fatalf("key = %q", key)
	}
	if strings.Contains(stderr.String(), key) {
		t.Fatal("API key was printed")
	}
}

func TestHelp(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := execute([]string{"help"}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "llmfp doctor") {
		t.Fatalf("help missing doctor command: %s", stdout.String())
	}
}
