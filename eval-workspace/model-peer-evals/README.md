# Model Peer Evals

This directory stores offline model-as-judge votes used to cold-start Elo scores before enough user votes exist.

## Files

- `generate_peer_votes.py`: samples answer pairs, calls remote judge models, and validates finished peer-vote JSONL files.
- `peer_vote_tasks.jsonl`: generated judging tasks. This file contains prompts and both answers, but no outcome.
- `peer_votes.jsonl`: finished static judging results imported by the backend on startup.
- `peer_votes.example.jsonl`: schema example.

The backend only imports `peer_votes.jsonl`. Missing files are treated as an empty cold-start set.

## Generate Tasks

From the repository root:

```bash
python3 eval-workspace/model-peer-evals/generate_peer_votes.py sample \
  --domains-root eval-workspace/domains \
  --output eval-workspace/model-peer-evals/peer_vote_tasks.jsonl \
  --pairs-per-judge 20 \
  --seed 20260512


python3 eval-workspace/model-peer-evals/generate_peer_votes.py sample 
```

The sampler uses the current four domains and the existing `responses/*.jsonl` files. Each enabled model becomes a judge for each domain, and each judge gets up to 20 random answer pairs. The judge model is excluded from the left/right candidates by default.

## Run Remote Judges

Use the peer-eval model gateway config in this directory:

```bash
python3 eval-workspace/model-peer-evals/generate_peer_votes.py judge \
  --input eval-workspace/model-peer-evals/peer_vote_tasks.jsonl \
  --output eval-workspace/model-peer-evals/peer_votes.jsonl \
  --errors-output eval-workspace/model-peer-evals/peer_vote_errors.jsonl \
  --models eval-workspace/model-peer-evals/models.local.json \
  --workers 7
```

The judge command reads each task's `judgeModel`, finds that model in `--models`, calls the configured remote endpoint, and writes only successful, importable rows to `peer_votes.jsonl`. Failed calls are written to `peer_vote_errors.jsonl`, so backend startup will not be blocked by malformed or failed judge records.

For newer OpenAI-compatible models that reject `max_tokens`, set `"maxTokensParam": "max_completion_tokens"` on that model config. The script also auto-detects common reasoning model ids such as `gpt-5*`, `o1*`, `o3*`, and `o4*`.

For endpoints that reject `temperature`, set `"temperature": null` on that model config. The script also retries once without `temperature` if the gateway returns a deprecation or unsupported-parameter error.

The judge model must return JSON like:

```json
{"outcome":"left","confidence":0.82}
```

## Fill Results Manually

After running judge calls, write `peer_votes.jsonl` with one JSON object per line:

```json
{"id":"mpe-20260512-000001","outcome":"left","score":1,"confidence":0.82}
```

`id` must match a row in `peer_vote_tasks.jsonl`. The backend uses that task row to recover `domain`, `queryId`, `judgeModel`, `leftModel`, and `rightModel`.

Allowed `outcome` values match the product voting flow:

- `left`
- `right`
- `both_good`
- `both_bad`

`score` and `confidence` are required in `peer_votes.jsonl`.

## Validate Results

```bash
python3 eval-workspace/model-peer-evals/generate_peer_votes.py validate \
  --domains-root eval-workspace/domains \
  --input eval-workspace/model-peer-evals/peer_votes.jsonl \
  --tasks eval-workspace/model-peer-evals/peer_vote_tasks.jsonl
```

Validation checks the compact result fields, then joins `peer_vote_tasks.jsonl` by `id` to verify domain slugs, query ids, model ids, distinct left/right models, and outcome values.
