# Roadmap and TODO

The project welcomes additional, reproducible fingerprinting methods. The goal
is a collection of independent evidence layers, not a single opaque "real or
fake" score.

## Next research integration: single-token behavioral fingerprints

The next planned evidence layer is based on Tomáš Bruckner's dataset
[Single-token output distributions as behavioral fingerprints of large
language models](https://doi.org/10.5281/zenodo.21278557).

Important source facts:

- Zenodo record: `21278557`, DOI `10.5281/zenodo.21278557`
- Dataset license: CC-BY-4.0
- Published archive: `pamela-publish-data.zip`, 52,366,366 bytes
- Zenodo checksum: `md5:f2ce3fba3081f73e9908179fb2f061b6`
- The archive contains raw/normalized response records, per-cell output
  distributions, Jensen-Shannon divergence results, clustering,
  classification, and verification outputs.
- The published `main-02` manifest describes 15 task IDs, English/Russian/
  Chinese/Arabic prompts, 30 repetitions at temperature 1, and three at
  temperature 0.
- The artifact's `results/verification.json` reports full-battery AUC `0.971342`
  and EER `0.07282`. These are dataset results, not accuracy guarantees for
  arbitrary relays or future models.

The data archive does not appear to contain the complete prompt-source files or
the analysis programs referenced by its internal README. Exact reproduction of
the battery and normalization rules is therefore a blocking prerequisite, not
something llmfp will guess from task names.

### Integration work packages

- [ ] Locate a citable companion paper/repository or obtain the exact prompt
  battery and normalization code from the dataset author.
- [ ] Write a reproducibility note mapping every implemented prompt, language,
  sampling parameter, normalization rule, and score to the source artifact.
- [ ] Add an explicit, opt-in artifact downloader with DOI, byte-size, checksum,
  cache, and CC-BY-4.0 attribution validation. Do not vendor or silently
  download the dataset.
- [ ] Define a versioned reference-data format that can be regenerated from the
  pinned artifact rather than committing the 52 MB archive to this repository.
- [ ] Implement parsing tests for `normalized.jsonl`, `distributions.json`, and
  the reference model/provider metadata.
- [ ] Add a behavioral collection command that freezes prompt version,
  temperature, repetition count, adapter, provider route, and timestamps.
- [ ] Implement the documented distribution and Jensen-Shannon calculations
  with frozen fixtures and cross-check them against the published results.
- [ ] Calibrate nearest-neighbor/family scoring, explicit unknown-model
  rejection, minimum valid-response coverage, and confidence intervals.
- [ ] Measure sensitivity to provider routing, system prompts, model revisions,
  quantization, language, sampling implementation, and safety layers.
- [ ] Keep `tool_attribution` and future `behavioral_attribution` separate in
  output. Introduce a new JSON schema version before exposing the latter.
- [ ] Add budget controls and a dry-run cost estimate: repeated behavioral
  sampling can make many paid API calls.
- [ ] Publish a validation report covering known-model identification,
  out-of-reference rejection, drift, false attribution, cost, and latency.

### Acceptance gates

The integration is not complete until:

1. The exact experiment is reproducible from licensed, pinned inputs.
2. Independent fixtures reproduce the selected reference statistics within a
   documented tolerance.
3. Unknown and drifted models can produce `unknown` rather than a forced label.
4. Provider and model-family claims have separately reported calibration.
5. Reports disclose query count, cost-relevant parameters, reference version,
   and all material caveats.

## Other fingerprint contributions welcome

Useful proposals may target tool/network infrastructure, behavioral output
distributions, tokenizer or protocol behavior, model capability probes, or
other measurable signals. Open a
[fingerprint method proposal](https://github.com/toads/llmfp/issues/new?template=fingerprint-method.yml)
before a large implementation.

Every proposed detector should include:

- the evidence layer and exact claim it supports;
- a threat model explaining how a relay could imitate or bypass the signal;
- reproducible fixtures and an evaluation dataset;
- false-positive, unknown-model, and drift behavior;
- query cost, latency, privacy, and external-service requirements;
- source/data licenses and required attribution;
- a versioned machine-output contract.

Small, independently useful improvements are also welcome: new API adapters,
provider registries, self-hosted OAST support, offline report verification,
and anonymized integration fixtures.
