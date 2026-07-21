# Contributing

Thanks for helping improve `llmfp`.

## Development setup

Install Go 1.25.12 or newer, then run:

```bash
go mod download
make check
```

Keep changes focused and add tests for behavior changes. The default test suite
must not require API keys or external services.

## New fingerprint methods

New evidence layers are welcome. Start with the
[fingerprint method proposal](https://github.com/toads/llmfp/issues/new?template=fingerprint-method.yml)
and follow [TODO.md](TODO.md). A proposal must state the narrow supported claim,
threat model, reference-data license, reproducible evaluation, false-positive
and unknown behavior, drift risks, cost, and privacy impact.

Do not merge independent signals into an undocumented authenticity score.
Tool-layer, behavioral, protocol, and other results must remain distinguishable
in both human and JSON reports.

## Integration tests

Integration tests contact the public `oast.pro` service and the provider
registry URLs documented in the project. Run them explicitly:

```bash
make integration
```

Do not put API keys, customer endpoints, private prompts, or captured callback
data in tests, fixtures, issues, or pull requests.

## Pull requests

Before opening a pull request:

```bash
make check
```

Describe the user-visible behavior, adapter behavior, evidence-boundary changes,
and validation performed. Attribution changes should include cases for official
CIDR matches, forgeable-header-only matches, DNS-only observations, and missing
callbacks.

By submitting a contribution, you agree that it is licensed under the
Apache-2.0 license used by this repository.
