# Security

`llmfp` is an independent project and is not affiliated with OpenAI,
Anthropic, or ProjectDiscovery.

## Reporting a vulnerability

Use GitHub private vulnerability reporting when it is enabled for the eventual
public repository. Do not open a public issue containing API keys, callback
URLs that are still active, private endpoints, or raw customer traffic.

## API key handling

`llmfp` intentionally has no `--api-key` flag. Use a hidden terminal prompt,
`--api-key-stdin`, or `--api-key-env`. Prefer a temporary, restricted, low-limit
key when testing an untrusted relay.

The CLI sends the selected key to the API host supplied with `--base-url`.
Review the normalized endpoint printed by the CLI before using a valuable key.
API keys are not included in reports, callback URLs, or prompts.

## Public probe service

The default `oast.pro` collector is operated by a third party. Its operators
can observe callback metadata such as source addresses, timestamps, headers,
and probe paths. Do not embed secrets or private data in a probe. For sensitive
work, use a reviewed self-hosted Interactsh server with `--probe-server`.

## Evidence boundary

A provider-published CIDR match can attribute the infrastructure that executed
a web fetch. It does not prove which model generated the final answer. Security
reports should preserve this distinction.
