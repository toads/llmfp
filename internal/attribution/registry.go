package attribution

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"strings"
	"time"
)

const maxRegistryBytes = 2 << 20

type RegistryDefinition struct {
	Provider string
	Surface  string
	URL      string
	UAToken  string
	Format   string
}

const (
	registryFormatOpenAIJSON        = "openai-json"
	registryFormatAnthropicMarkdown = "anthropic-markdown"
)

var OfficialRegistries = []RegistryDefinition{
	{Provider: "openai", Surface: "chatgpt-user", URL: "https://openai.com/chatgpt-user.json", UAToken: "ChatGPT-User/", Format: registryFormatOpenAIJSON},
	{Provider: "openai", Surface: "searchbot", URL: "https://openai.com/searchbot.json", UAToken: "OAI-SearchBot/", Format: registryFormatOpenAIJSON},
	{Provider: "openai", Surface: "gptbot", URL: "https://openai.com/gptbot.json", UAToken: "GPTBot/", Format: registryFormatOpenAIJSON},
	{Provider: "anthropic", Surface: "outbound-tools", URL: "https://platform.claude.com/docs/en/api/ip-addresses.md", Format: registryFormatAnthropicMarkdown},
}

type RegistrySnapshot struct {
	Provider    string         `json:"provider"`
	Surface     string         `json:"surface"`
	URL         string         `json:"url"`
	PublishedAt string         `json:"published_at,omitempty"`
	FetchedAt   time.Time      `json:"fetched_at"`
	SHA256      string         `json:"sha256"`
	Prefixes    []netip.Prefix `json:"-"`
}

type registryDocument struct {
	CreationTime string `json:"creationTime"`
	Prefixes     []struct {
		IPv4 string `json:"ipv4Prefix"`
		IPv6 string `json:"ipv6Prefix"`
	} `json:"prefixes"`
}

type Matcher struct {
	Definitions []RegistryDefinition
	HTTPClient  *http.Client
}

func NewMatcher(timeout time.Duration) *Matcher {
	return &Matcher{
		Definitions: OfficialRegistries,
		HTTPClient:  &http.Client{Timeout: timeout},
	}
}

func (m *Matcher) Fetch(ctx context.Context) ([]RegistrySnapshot, []string) {
	var snapshots []RegistrySnapshot
	var warnings []string
	for _, definition := range m.Definitions {
		snapshot, err := m.fetchOne(ctx, definition)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("%s: %v", definition.URL, err))
			continue
		}
		snapshots = append(snapshots, snapshot)
	}
	return snapshots, warnings
}

func (m *Matcher) fetchOne(ctx context.Context, definition RegistryDefinition) (RegistrySnapshot, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, definition.URL, nil)
	if err != nil {
		return RegistrySnapshot{}, err
	}
	req.Header.Set("User-Agent", "llmfp/0.1")
	resp, err := m.HTTPClient.Do(req)
	if err != nil {
		return RegistrySnapshot{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return RegistrySnapshot{}, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxRegistryBytes+1))
	if err != nil {
		return RegistrySnapshot{}, err
	}
	if len(body) > maxRegistryBytes {
		return RegistrySnapshot{}, fmt.Errorf("registry exceeds 2 MiB")
	}
	digest := sha256.Sum256(body)
	snapshot := RegistrySnapshot{
		Provider:  definition.Provider,
		Surface:   definition.Surface,
		URL:       definition.URL,
		FetchedAt: time.Now().UTC(),
		SHA256:    hex.EncodeToString(digest[:]),
	}
	var parseErr error
	switch definition.Format {
	case registryFormatOpenAIJSON:
		parseErr = parseOpenAIJSON(body, &snapshot)
	case registryFormatAnthropicMarkdown:
		parseErr = parseAnthropicMarkdown(body, &snapshot)
	default:
		parseErr = fmt.Errorf("unsupported registry format %q", definition.Format)
	}
	if parseErr != nil {
		return RegistrySnapshot{}, parseErr
	}
	if len(snapshot.Prefixes) == 0 {
		return RegistrySnapshot{}, fmt.Errorf("registry contained no prefixes")
	}
	return snapshot, nil
}

func parseOpenAIJSON(body []byte, snapshot *RegistrySnapshot) error {
	var document registryDocument
	if err := json.Unmarshal(body, &document); err != nil {
		return fmt.Errorf("decode registry: %w", err)
	}
	snapshot.PublishedAt = document.CreationTime
	for _, item := range document.Prefixes {
		value := item.IPv4
		if value == "" {
			value = item.IPv6
		}
		prefix, err := netip.ParsePrefix(value)
		if err != nil {
			return fmt.Errorf("invalid prefix %q: %w", value, err)
		}
		snapshot.Prefixes = append(snapshot.Prefixes, prefix)
	}
	return nil
}

func parseAnthropicMarkdown(body []byte, snapshot *RegistrySnapshot) error {
	lines := strings.Split(string(body), "\n")
	inOutbound := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "## Outbound IP addresses" {
			inOutbound = true
			continue
		}
		if !inOutbound {
			continue
		}
		if strings.HasPrefix(trimmed, "## ") || strings.HasPrefix(trimmed, "### Phased out") {
			break
		}
		value := strings.Trim(trimmed, "`")
		if !strings.Contains(value, "/") {
			continue
		}
		prefix, err := netip.ParsePrefix(value)
		if err != nil {
			continue
		}
		snapshot.Prefixes = append(snapshot.Prefixes, prefix)
	}
	return nil
}

type Verdict struct {
	Status            string     `json:"status"`
	Provider          string     `json:"provider,omitempty"`
	Surface           string     `json:"surface,omitempty"`
	Confidence        string     `json:"confidence"`
	SourceIP          string     `json:"source_ip,omitempty"`
	UserAgent         string     `json:"user_agent,omitempty"`
	MatchedCIDR       string     `json:"matched_cidr,omitempty"`
	RegistryURL       string     `json:"registry_url,omitempty"`
	RegistryFetchedAt *time.Time `json:"registry_fetched_at,omitempty"`
	RegistrySHA256    string     `json:"registry_sha256,omitempty"`
	ExactPathFetched  bool       `json:"exact_path_fetched"`
	Reason            string     `json:"reason"`
}

type HTTPObservation struct {
	SourceIP  netip.Addr
	UserAgent string
	ExactPath bool
}

func Attribute(observation *HTTPObservation, hadDNS bool, snapshots []RegistrySnapshot) Verdict {
	if observation == nil {
		if hadDNS {
			return Verdict{Status: "dns_only", Confidence: "weak", Reason: "DNS resolution occurred, but no HTTP fetch was observed"}
		}
		return Verdict{Status: "no_callback", Confidence: "none", Reason: "No DNS or HTTP callback was observed before the timeout"}
	}

	verdict := Verdict{
		Status:           "unattributed",
		Confidence:       "none",
		SourceIP:         observation.SourceIP.String(),
		UserAgent:        observation.UserAgent,
		ExactPathFetched: observation.ExactPath,
		Reason:           "HTTP fetch observed, but the source IP did not match a downloaded official registry",
	}

	for _, snapshot := range snapshots {
		for _, prefix := range snapshot.Prefixes {
			if !prefix.Contains(observation.SourceIP) {
				continue
			}
			verdict.Status = "attributed"
			verdict.Provider = snapshot.Provider
			verdict.Surface = snapshot.Surface
			verdict.MatchedCIDR = prefix.String()
			verdict.RegistryURL = snapshot.URL
			fetchedAt := snapshot.FetchedAt
			verdict.RegistryFetchedAt = &fetchedAt
			verdict.RegistrySHA256 = snapshot.SHA256
			verdict.Confidence = "strong"
			verdict.Reason = "HTTP source IP matched a provider-published CIDR"
			if observation.ExactPath {
				verdict.Reason += " and fetched the unique probe path"
			}
			if uaTokenForSurface(snapshot.Surface) != "" && !strings.Contains(observation.UserAgent, uaTokenForSurface(snapshot.Surface)) {
				verdict.Confidence = "moderate"
				verdict.Reason += "; User-Agent did not match the published surface name"
			}
			return verdict
		}
	}

	for _, definition := range OfficialRegistries {
		if definition.UAToken != "" && strings.Contains(observation.UserAgent, definition.UAToken) {
			verdict.Status = "likely"
			verdict.Provider = definition.Provider
			verdict.Surface = definition.Surface
			verdict.Confidence = "weak"
			verdict.Reason = "User-Agent matched a provider token, but source IP did not match a downloaded official CIDR"
			return verdict
		}
	}
	return verdict
}

func uaTokenForSurface(surface string) string {
	for _, definition := range OfficialRegistries {
		if definition.Surface == surface {
			return definition.UAToken
		}
	}
	return ""
}
