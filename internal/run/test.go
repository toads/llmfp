package run

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/netip"
	"regexp"
	"strings"
	"time"

	"github.com/toads/llmfp/internal/apiclient"
	"github.com/toads/llmfp/internal/attribution"
	"github.com/toads/llmfp/internal/probe"
)

type Config struct {
	Adapter         string
	BaseURL         string
	APIKey          string
	Model           string
	ProbeServer     string
	CallbackWait    time.Duration
	RequestTimeout  time.Duration
	RegistryTimeout time.Duration
	AllowHTTP       bool
	JSON            bool
	Progress        func(string)
}

type ProbeReport struct {
	URL  string `json:"url"`
	Host string `json:"host"`
	Path string `json:"path"`
}

type APIReport struct {
	Adapter           string `json:"adapter"`
	Endpoint          string `json:"endpoint"`
	StatusCode        int    `json:"status_code"`
	ContentType       string `json:"content_type,omitempty"`
	ResponseID        string `json:"response_id,omitempty"`
	ReportedModel     string `json:"reported_model,omitempty"`
	Error             string `json:"error,omitempty"`
	ChallengeReturned bool   `json:"challenge_returned"`
}

type EventReport struct {
	Protocol      string    `json:"protocol"`
	RemoteAddress string    `json:"remote_address"`
	Timestamp     time.Time `json:"timestamp"`
	QType         string    `json:"q_type,omitempty"`
	Path          string    `json:"path,omitempty"`
	UserAgent     string    `json:"user_agent,omitempty"`
	ExactPath     bool      `json:"exact_path"`
}

type Report struct {
	SchemaVersion    string              `json:"schema_version"`
	StartedAt        time.Time           `json:"started_at"`
	CompletedAt      time.Time           `json:"completed_at"`
	Probe            ProbeReport         `json:"probe"`
	API              APIReport           `json:"api"`
	Events           []EventReport       `json:"events"`
	Attribution      attribution.Verdict `json:"tool_attribution"`
	ModelAttribution string              `json:"model_attribution"`
	Registries       []RegistryReport    `json:"registries"`
	RegistryErrors   []string            `json:"registry_errors,omitempty"`
}

const SchemaVersion = "1"

type RegistryReport struct {
	Provider    string    `json:"provider"`
	Surface     string    `json:"surface"`
	URL         string    `json:"url"`
	PublishedAt string    `json:"published_at,omitempty"`
	FetchedAt   time.Time `json:"fetched_at"`
	SHA256      string    `json:"sha256"`
	Prefixes    int       `json:"prefix_count"`
}

var bodyPattern = regexp.MustCompile(`(?is)<body>([^<]{1,512})</body>`)

func Test(cfg Config) (Report, error) {
	if cfg.ProbeServer == "" {
		cfg.ProbeServer = "oast.pro"
	}
	if cfg.CallbackWait <= 0 {
		return Report{}, fmt.Errorf("--wait must be positive")
	}
	if cfg.RequestTimeout <= 0 {
		return Report{}, fmt.Errorf("--request-timeout must be positive")
	}
	if cfg.RegistryTimeout <= 0 {
		return Report{}, fmt.Errorf("--registry-timeout must be positive")
	}
	adapter, err := apiclient.ResolveAdapter(cfg.Adapter, cfg.BaseURL, cfg.Model)
	if err != nil {
		return Report{}, err
	}
	if _, err := apiclient.Endpoint(adapter, cfg.BaseURL, cfg.AllowHTTP); err != nil {
		return Report{}, err
	}

	report := Report{
		SchemaVersion:    SchemaVersion,
		StartedAt:        time.Now().UTC(),
		Events:           []EventReport{},
		Registries:       []RegistryReport{},
		ModelAttribution: "unknown: tool attribution does not prove generation model identity",
	}
	notify(cfg, "Registering one-time probe with "+cfg.ProbeServer)
	probeClient, err := probe.New(cfg.ProbeServer)
	if err != nil {
		return Report{}, err
	}
	defer probeClient.Close()

	path := "/llmfp-" + randomHex(8)
	host := probeClient.Host()
	probeURL := "https://" + host + path
	report.Probe = ProbeReport{URL: probeURL, Host: host, Path: path}
	notify(cfg, "Probe ready: "+probeURL)

	prompt := fmt.Sprintf("Use the web search or web fetch tool to directly open this exact URL. Do not use cached search results. Return only the text inside the HTML body: %s", probeURL)
	notify(cfg, "Calling API with "+adapter)
	apiResult := apiclient.Call(context.Background(), adapter, cfg.BaseURL, cfg.APIKey, cfg.Model, prompt, cfg.AllowHTTP, cfg.RequestTimeout)
	report.API = APIReport{
		Adapter:       apiResult.Adapter,
		Endpoint:      apiResult.Endpoint,
		StatusCode:    apiResult.StatusCode,
		ContentType:   apiResult.ContentType,
		ResponseID:    apiResult.ResponseID,
		ReportedModel: apiResult.ReportedModel,
		Error:         apiResult.Error,
	}

	callbackWait := cfg.CallbackWait
	if apiResult.Error != "" || apiResult.StatusCode < 200 || apiResult.StatusCode >= 300 {
		callbackWait = minDuration(callbackWait, 3*time.Second)
	}
	if apiResult.StatusCode != 0 {
		notify(cfg, fmt.Sprintf("API responded with HTTP %d", apiResult.StatusCode))
	} else {
		notify(cfg, "API request failed before an HTTP response")
	}
	notify(cfg, "Waiting for DNS/HTTP callback")
	events := collectEvents(probeClient.Events(), callbackWait)
	notify(cfg, fmt.Sprintf("Collected %d callback event(s)", len(events)))
	var observation *attribution.HTTPObservation
	var hadDNS bool
	var challenge string
	for _, event := range events {
		eventReport, parsedObservation := summarizeEvent(event, path)
		report.Events = append(report.Events, eventReport)
		if strings.EqualFold(event.Protocol, "dns") {
			hadDNS = true
		}
		if parsedObservation != nil && (observation == nil || parsedObservation.ExactPath) {
			observation = parsedObservation
		}
		if challenge == "" && strings.EqualFold(event.Protocol, "http") {
			if match := bodyPattern.FindStringSubmatch(event.RawResponse); len(match) == 2 {
				challenge = strings.TrimSpace(match[1])
			}
		}
	}
	if challenge != "" {
		report.API.ChallengeReturned = strings.Contains(string(apiResult.RawBody), challenge)
	}

	var snapshots []attribution.RegistrySnapshot
	if observation != nil {
		notify(cfg, "Downloading official provider IP registries")
		matcher := attribution.NewMatcher(cfg.RegistryTimeout)
		registryCtx, cancel := context.WithTimeout(context.Background(), cfg.RegistryTimeout)
		var warnings []string
		snapshots, warnings = matcher.Fetch(registryCtx)
		cancel()
		report.RegistryErrors = warnings
		for _, snapshot := range snapshots {
			report.Registries = append(report.Registries, RegistryReport{
				Provider: snapshot.Provider, Surface: snapshot.Surface, URL: snapshot.URL,
				PublishedAt: snapshot.PublishedAt, FetchedAt: snapshot.FetchedAt,
				SHA256: snapshot.SHA256, Prefixes: len(snapshot.Prefixes),
			})
		}
	}
	report.Attribution = attribution.Attribute(observation, hadDNS, snapshots)
	report.CompletedAt = time.Now().UTC()
	notify(cfg, "Attribution complete: "+report.Attribution.Status)
	return report, nil
}

func notify(cfg Config, message string) {
	if cfg.Progress != nil {
		cfg.Progress(message)
	}
}

func minDuration(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}

func collectEvents(events <-chan *probe.Interaction, wait time.Duration) []*probe.Interaction {
	deadline := time.NewTimer(wait)
	defer deadline.Stop()
	var collected []*probe.Interaction
	var settle *time.Timer
	var settleC <-chan time.Time
	for {
		select {
		case event := <-events:
			collected = append(collected, event)
			if strings.EqualFold(event.Protocol, "http") && settle == nil {
				settle = time.NewTimer(1500 * time.Millisecond)
				settleC = settle.C
			}
		case <-settleC:
			return collected
		case <-deadline.C:
			if settle != nil {
				settle.Stop()
			}
			return collected
		}
	}
}

func summarizeEvent(event *probe.Interaction, expectedPath string) (EventReport, *attribution.HTTPObservation) {
	report := EventReport{
		Protocol: event.Protocol, RemoteAddress: event.RemoteAddress,
		Timestamp: event.Timestamp, QType: event.QType,
	}
	if !strings.EqualFold(event.Protocol, "http") {
		return report, nil
	}
	path, userAgent := parseRawRequest(event.RawRequest)
	report.Path = path
	report.UserAgent = userAgent
	report.ExactPath = path == expectedPath
	address, err := parseRemoteIP(event.RemoteAddress)
	if err != nil {
		return report, nil
	}
	return report, &attribution.HTTPObservation{SourceIP: address, UserAgent: userAgent, ExactPath: report.ExactPath}
}

func parseRawRequest(raw string) (string, string) {
	lines := strings.Split(strings.ReplaceAll(raw, "\r", ""), "\n")
	var path, userAgent string
	if len(lines) > 0 {
		parts := strings.Fields(lines[0])
		if len(parts) >= 2 {
			path = parts[1]
		}
	}
	for _, line := range lines[1:] {
		name, value, ok := strings.Cut(line, ":")
		if ok && strings.EqualFold(strings.TrimSpace(name), "User-Agent") {
			userAgent = strings.TrimSpace(value)
			break
		}
	}
	return path, userAgent
}

func parseRemoteIP(value string) (netip.Addr, error) {
	if address, err := netip.ParseAddr(value); err == nil {
		return address.Unmap(), nil
	}
	if addressPort, err := netip.ParseAddrPort(value); err == nil {
		return addressPort.Addr().Unmap(), nil
	}
	return netip.Addr{}, fmt.Errorf("invalid remote address %q", value)
}

func randomHex(bytesCount int) string {
	buffer := make([]byte, bytesCount)
	if _, err := rand.Read(buffer); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buffer)
}
