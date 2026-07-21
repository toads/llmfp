package attribution

import (
	"context"
	"net/netip"
	"os"
	"testing"
	"time"
)

func TestAttributeStrongOfficialCIDR(t *testing.T) {
	verdict := Attribute(&HTTPObservation{
		SourceIP:  netip.MustParseAddr("52.231.49.54"),
		UserAgent: "Mozilla/5.0 compatible; ChatGPT-User/1.0",
		ExactPath: true,
	}, true, []RegistrySnapshot{{
		Provider: "openai", Surface: "chatgpt-user", URL: "https://openai.com/chatgpt-user.json",
		FetchedAt: time.Now(), SHA256: "abc", Prefixes: []netip.Prefix{netip.MustParsePrefix("52.231.49.48/28")},
	}})
	if verdict.Status != "attributed" || verdict.Confidence != "strong" {
		t.Fatalf("unexpected verdict: %+v", verdict)
	}
	if verdict.MatchedCIDR != "52.231.49.48/28" || !verdict.ExactPathFetched {
		t.Fatalf("missing evidence: %+v", verdict)
	}
}

func TestAttributeDNSOnly(t *testing.T) {
	verdict := Attribute(nil, true, nil)
	if verdict.Status != "dns_only" {
		t.Fatalf("status = %q, want dns_only", verdict.Status)
	}
}

func TestParseAnthropicMarkdown(t *testing.T) {
	body := []byte("# IP addresses\n\n## Outbound IP addresses\n\n### IPv4\n\n`160.79.104.0/21`\n\n### Phased out IP addresses\n\n`34.162.46.92/32`\n")
	var snapshot RegistrySnapshot
	if err := parseAnthropicMarkdown(body, &snapshot); err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Prefixes) != 1 || snapshot.Prefixes[0].String() != "160.79.104.0/21" {
		t.Fatalf("prefixes = %v", snapshot.Prefixes)
	}
	verdict := Attribute(&HTTPObservation{
		SourceIP: netip.MustParseAddr("160.79.104.42"), ExactPath: true,
	}, true, []RegistrySnapshot{{
		Provider: "anthropic", Surface: "outbound-tools", URL: "https://platform.claude.com/docs/en/api/ip-addresses.md",
		FetchedAt: time.Now(), SHA256: "abc", Prefixes: snapshot.Prefixes,
	}})
	if verdict.Provider != "anthropic" || verdict.Confidence != "strong" {
		t.Fatalf("verdict = %+v", verdict)
	}
}

func TestAttributeDoesNotTrustUserAgentAlone(t *testing.T) {
	verdict := Attribute(&HTTPObservation{
		SourceIP:  netip.MustParseAddr("203.0.113.9"),
		UserAgent: "compatible; ChatGPT-User/1.0",
		ExactPath: true,
	}, true, nil)
	if verdict.Status != "likely" || verdict.Confidence != "weak" {
		t.Fatalf("unexpected verdict: %+v", verdict)
	}
}

func TestProviderRegistriesLive(t *testing.T) {
	if os.Getenv("LLMFP_INTEGRATION") != "1" {
		t.Skip("set LLMFP_INTEGRATION=1 to download the official OpenAI registries")
	}
	matcher := NewMatcher(20 * time.Second)
	snapshots, warnings := matcher.Fetch(context.Background())
	if len(warnings) != 0 {
		t.Fatalf("registry warnings: %v", warnings)
	}
	if len(snapshots) != len(OfficialRegistries) {
		t.Fatalf("got %d snapshots, want %d", len(snapshots), len(OfficialRegistries))
	}
	verdict := Attribute(&HTTPObservation{
		SourceIP:  netip.MustParseAddr("52.231.49.54"),
		UserAgent: "compatible; ChatGPT-User/1.0",
		ExactPath: true,
	}, true, snapshots)
	if verdict.Provider != "openai" || verdict.Surface != "chatgpt-user" || verdict.Confidence != "strong" {
		t.Fatalf("unexpected live registry verdict: %+v", verdict)
	}
}
