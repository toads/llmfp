package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"runtime"
	"strings"
	"time"

	"golang.org/x/term"

	"github.com/toads/llmfp/internal/attribution"
	"github.com/toads/llmfp/internal/probe"
	"github.com/toads/llmfp/internal/run"
)

var (
	version   = "0.1.0-dev"
	commit    = "unknown"
	buildDate = "unknown"
)

func main() {
	if err := execute(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func execute(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		printUsage(stderr)
		return errors.New("missing command")
	}

	switch args[0] {
	case "test":
		return runTest(args[1:], stdout, stderr)
	case "doctor":
		return runDoctor(args[1:], stdout, stderr)
	case "version", "--version", "-version":
		fmt.Fprintf(stdout, "llmfp %s\ncommit: %s\nbuilt: %s\ngo: %s\nplatform: %s/%s\n", version, commit, buildDate, runtime.Version(), runtime.GOOS, runtime.GOARCH)
		return nil
	case "help", "--help", "-h":
		printUsage(stdout)
		return nil
	default:
		printUsage(stderr)
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, `llmfp attributes the web tool used by an OpenAI Responses-compatible API.

Usage:
  llmfp test --base-url URL --model MODEL [options]
  llmfp doctor [options]
  llmfp version

Environment:
  LLMFP_BASE_URL   default API base URL
  LLMFP_MODEL      default model name
  LLMFP_API_KEY    preferred automatic API key source
  RELAY_API_KEY    fallback automatic API key source
  ANTHROPIC_API_KEY used only for api.anthropic.com

Run "llmfp <command> -h" for command options.`)
}

func runTest(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	fs.SetOutput(stderr)

	var cfg run.Config
	var apiKeyEnv string
	var apiKeyStdin bool
	var quiet bool
	fs.StringVar(&cfg.Adapter, "adapter", "auto", "API adapter: auto, openai-responses, or anthropic-messages")
	fs.StringVar(&cfg.BaseURL, "base-url", os.Getenv("LLMFP_BASE_URL"), "API origin, /v1 base, or complete adapter endpoint")
	fs.StringVar(&cfg.Model, "model", os.Getenv("LLMFP_MODEL"), "model name passed to the API")
	fs.StringVar(&apiKeyEnv, "api-key-env", "", "read the API key from this environment variable")
	fs.BoolVar(&apiKeyStdin, "api-key-stdin", false, "read one API key line from stdin")
	fs.DurationVar(&cfg.CallbackWait, "wait", 45*time.Second, "maximum time to wait for a callback after the API responds")
	fs.DurationVar(&cfg.RequestTimeout, "request-timeout", 90*time.Second, "timeout for the API request")
	fs.DurationVar(&cfg.RegistryTimeout, "registry-timeout", 10*time.Second, "timeout for official IP registry downloads")
	fs.BoolVar(&cfg.JSON, "json", false, "write the report as JSON")
	fs.BoolVar(&quiet, "quiet", false, "suppress progress messages on stderr")
	fs.BoolVar(&cfg.AllowHTTP, "allow-http", false, "allow an http:// API base URL")
	fs.StringVar(&cfg.ProbeServer, "probe-server", "oast.pro", "Interactsh server domain")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if cfg.BaseURL == "" {
		return errors.New("--base-url is required")
	}
	if cfg.Model == "" {
		return errors.New("--model is required")
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}

	if apiKeyEnv == "" && !apiKeyStdin {
		apiKeyEnv = automaticAPIKeyEnv(cfg.BaseURL)
	}
	apiKey, err := readAPIKey(apiKeyEnv, apiKeyStdin, stderr)
	if err != nil {
		return err
	}
	cfg.APIKey = apiKey
	defer func() { cfg.APIKey = "" }()
	if !quiet {
		cfg.Progress = func(message string) {
			fmt.Fprintf(stderr, "[%s] %s\n", time.Now().Format("15:04:05"), message)
		}
	}

	report, err := run.Test(cfg)
	if err != nil {
		return err
	}
	if cfg.JSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(report)
	}
	writeTextReport(stdout, report)
	return nil
}

func automaticAPIKeyEnv(baseURL string) string {
	for _, name := range []string{"LLMFP_API_KEY", "RELAY_API_KEY"} {
		if os.Getenv(name) != "" {
			return name
		}
	}
	parsed, err := url.Parse(baseURL)
	if err == nil && strings.EqualFold(parsed.Hostname(), "api.openai.com") && os.Getenv("OPENAI_API_KEY") != "" {
		return "OPENAI_API_KEY"
	}
	if err == nil && strings.EqualFold(parsed.Hostname(), "api.anthropic.com") && os.Getenv("ANTHROPIC_API_KEY") != "" {
		return "ANTHROPIC_API_KEY"
	}
	return ""
}

type doctorReport struct {
	SchemaVersion string               `json:"schema_version"`
	OK            bool                 `json:"ok"`
	ProbeServer   string               `json:"probe_server"`
	ProbeHost     string               `json:"probe_host,omitempty"`
	Registries    []run.RegistryReport `json:"registries"`
	Warnings      []string             `json:"warnings,omitempty"`
}

func runDoctor(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var probeServer string
	var registryTimeout time.Duration
	var jsonOutput bool
	fs.StringVar(&probeServer, "probe-server", "oast.pro", "Interactsh server domain")
	fs.DurationVar(&registryTimeout, "registry-timeout", 10*time.Second, "timeout for official IP registry downloads")
	fs.BoolVar(&jsonOutput, "json", false, "write diagnostics as JSON")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}

	report := doctorReport{SchemaVersion: run.SchemaVersion, ProbeServer: probeServer, Registries: []run.RegistryReport{}}
	probeClient, err := probe.New(probeServer)
	if err != nil {
		report.Warnings = append(report.Warnings, err.Error())
	} else {
		report.ProbeHost = probeClient.Host()
		if closeErr := probeClient.Close(); closeErr != nil {
			report.Warnings = append(report.Warnings, closeErr.Error())
		}
	}

	matcher := attribution.NewMatcher(registryTimeout)
	ctx, cancel := context.WithTimeout(context.Background(), registryTimeout)
	snapshots, warnings := matcher.Fetch(ctx)
	cancel()
	report.Warnings = append(report.Warnings, warnings...)
	for _, snapshot := range snapshots {
		report.Registries = append(report.Registries, run.RegistryReport{
			Provider: snapshot.Provider, Surface: snapshot.Surface, URL: snapshot.URL,
			PublishedAt: snapshot.PublishedAt, FetchedAt: snapshot.FetchedAt,
			SHA256: snapshot.SHA256, Prefixes: len(snapshot.Prefixes),
		})
	}
	report.OK = report.ProbeHost != "" && len(report.Registries) == len(attribution.OfficialRegistries) && len(report.Warnings) == 0

	if jsonOutput {
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(report); err != nil {
			return err
		}
	} else {
		fmt.Fprintf(stdout, "Probe server: %s\n", probeServer)
		fmt.Fprintf(stdout, "Probe check:  %s\n", statusWord(report.ProbeHost != ""))
		fmt.Fprintf(stdout, "Registries:   %d/%d available\n", len(report.Registries), len(attribution.OfficialRegistries))
		for _, registry := range report.Registries {
			fmt.Fprintf(stdout, "  %-14s %4d prefixes  %s\n", registry.Surface, registry.Prefixes, registry.SHA256[:12])
		}
		for _, warning := range report.Warnings {
			fmt.Fprintf(stdout, "Warning: %s\n", warning)
		}
	}
	if !report.OK {
		return errors.New("one or more diagnostics failed")
	}
	return nil
}

func statusWord(ok bool) string {
	if ok {
		return "ok"
	}
	return "failed"
}

func readAPIKey(envName string, fromStdin bool, stderr io.Writer) (string, error) {
	if envName != "" && fromStdin {
		return "", errors.New("use only one of --api-key-env and --api-key-stdin")
	}
	if envName != "" {
		value := os.Getenv(envName)
		if value == "" {
			return "", fmt.Errorf("environment variable %s is empty", envName)
		}
		return value, nil
	}
	if fromStdin {
		value, err := bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return "", fmt.Errorf("read API key: %w", err)
		}
		value = strings.TrimSpace(value)
		if value == "" {
			return "", errors.New("API key from stdin is empty")
		}
		return value, nil
	}

	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return "", errors.New("no API key source; use --api-key-env or --api-key-stdin")
	}
	fmt.Fprint(stderr, "API Key: ")
	value, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(stderr)
	if err != nil {
		return "", fmt.Errorf("read hidden API key: %w", err)
	}
	if len(value) == 0 {
		return "", errors.New("API key is empty")
	}
	return string(value), nil
}

func writeTextReport(w io.Writer, report run.Report) {
	fmt.Fprintf(w, "Probe URL:       %s\n", report.Probe.URL)
	fmt.Fprintf(w, "API adapter:     %s\n", report.API.Adapter)
	fmt.Fprintf(w, "API endpoint:    %s\n", report.API.Endpoint)
	fmt.Fprintf(w, "API status:      %d\n", report.API.StatusCode)
	fmt.Fprintf(w, "Content-Type:    %s\n", valueOrUnknown(report.API.ContentType))
	fmt.Fprintf(w, "Claimed model:   %s (unverified)\n", valueOrUnknown(report.API.ReportedModel))
	fmt.Fprintf(w, "Callback status: %s\n", report.Attribution.Status)
	fmt.Fprintf(w, "Tool provider:   %s\n", valueOrUnknown(report.Attribution.Provider))
	fmt.Fprintf(w, "Tool surface:    %s\n", valueOrUnknown(report.Attribution.Surface))
	fmt.Fprintf(w, "Confidence:      %s\n", report.Attribution.Confidence)
	fmt.Fprintf(w, "Source IP:       %s\n", valueOrUnknown(report.Attribution.SourceIP))
	fmt.Fprintf(w, "Matched CIDR:    %s\n", valueOrUnknown(report.Attribution.MatchedCIDR))
	fmt.Fprintf(w, "User-Agent:      %s\n", valueOrUnknown(report.Attribution.UserAgent))
	fmt.Fprintf(w, "Exact path hit:  %t\n", report.Attribution.ExactPathFetched)
	fmt.Fprintf(w, "Body returned:   %t\n", report.API.ChallengeReturned)
	fmt.Fprintf(w, "Model provider:  unknown (tool attribution does not prove model identity)\n")
	fmt.Fprintf(w, "Elapsed:         %s\n", report.CompletedAt.Sub(report.StartedAt).Round(time.Millisecond))
	if report.Attribution.Reason != "" {
		fmt.Fprintf(w, "Reason:           %s\n", report.Attribution.Reason)
	}
	if report.API.Error != "" {
		fmt.Fprintf(w, "API error:        %s\n", report.API.Error)
	}
	for _, registryErr := range report.RegistryErrors {
		fmt.Fprintf(w, "Registry warning: %s\n", registryErr)
	}
	if report.Attribution.Status == "no_callback" {
		fmt.Fprintln(w, "Hint:             The API may ignore tools, lack hosted web search, or block the probe domain.")
	}
}

func valueOrUnknown(value string) string {
	if value == "" {
		return "unknown"
	}
	return value
}
