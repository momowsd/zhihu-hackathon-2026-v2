#!/usr/bin/env python3
from __future__ import annotations

import argparse
import concurrent.futures
import datetime as dt
import itertools
import json
import os
import random
import re
import threading
import time
from dataclasses import dataclass
from pathlib import Path
from typing import Any

DEFAULT_DOMAINS = ["ruozhi-eval", "novel-writing-eval", "movie-script-eval", "emotion-eval"]
ALLOWED_OUTCOMES = {"left", "right", "both_good", "both_bad"}


@dataclass(frozen=True)
class Query:
    domain: str
    query_id: str
    query: str


@dataclass(frozen=True)
class Response:
    domain: str
    query_id: str
    model_id: str
    answer: str
    response_file: str


def read_jsonl(path: Path) -> list[dict[str, Any]]:
    rows: list[dict[str, Any]] = []
    if not path.exists():
        return rows
    with path.open("r", encoding="utf-8") as fh:
        for line_no, line in enumerate(fh, start=1):
            line = line.strip()
            if not line:
                continue
            try:
                rows.append(json.loads(line))
            except json.JSONDecodeError as exc:
                raise ValueError(f"{path}:{line_no}: invalid JSON: {exc}") from exc
    return rows


def write_jsonl(path: Path, rows: list[dict[str, Any]]) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    with path.open("w", encoding="utf-8") as fh:
        for row in rows:
            fh.write(json.dumps(row, ensure_ascii=False) + "\n")


def append_jsonl(path: Path, row: dict[str, Any], lock: threading.Lock | None = None) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    if lock:
        lock.acquire()
    try:
        with path.open("a", encoding="utf-8") as fh:
            fh.write(json.dumps(row, ensure_ascii=False) + "\n")
            fh.flush()
    finally:
        if lock:
            lock.release()


def load_queries(domains_root: Path, domain: str) -> dict[str, Query]:
    out: dict[str, Query] = {}
    for row in read_jsonl(domains_root / domain / "raw_queries.jsonl"):
        query_id = str(row.get("id") or "").strip()
        query = str(row.get("query") or "").strip()
        if query_id and query:
            out[query_id] = Query(domain=domain, query_id=query_id, query=query)
    return out


def load_responses(domains_root: Path, domain: str) -> dict[str, dict[str, Response]]:
    by_query: dict[str, dict[str, Response]] = {}
    responses_dir = domains_root / domain / "responses"
    for path in sorted(responses_dir.glob("*.jsonl")):
        default_model_id = path.stem
        for row in read_jsonl(path):
            if row.get("error"):
                continue
            answer = str(row.get("answer") or "").strip()
            query_id = str(row.get("queryId") or "").strip()
            model_id = str(row.get("modelId") or row.get("model") or default_model_id).strip()
            if not query_id or not model_id or not answer:
                continue
            by_query.setdefault(query_id, {})[model_id] = Response(
                domain=domain,
                query_id=query_id,
                model_id=model_id,
                answer=answer,
                response_file=str(path.relative_to(domains_root / domain)),
            )
    return by_query


def model_ids_for_domain(responses: dict[str, dict[str, Response]]) -> list[str]:
    model_ids = {model_id for per_query in responses.values() for model_id in per_query}
    return sorted(model_ids)


def build_judge_prompt(query: str, left_model: str, left_answer: str, right_model: str, right_answer: str) -> str:
    return (
        "你是一名严格但公平的模型盲评裁判。请只根据题目和两侧回答质量判断，不要因为模型名称产生偏见。\n\n"
        "请在四档中选择一个 outcome：left、right、both_good、both_bad。\n"
        "如果一侧明显更符合题意、质量更高，选择 left 或 right；如果两侧都好且难分高下，选择 both_good；"
        "如果两侧都明显不合格，选择 both_bad。\n\n"
        f"题目：\n{query}\n\n"
        f"模型 A（{left_model}）回答：\n{left_answer}\n\n"
        f"模型 B（{right_model}）回答：\n{right_answer}\n\n"
        "请只输出 JSON：{\"outcome\":\"left|right|both_good|both_bad\",\"confidence\":0.0-1.0}"
    )


def merge_defaults(config: dict[str, Any]) -> list[dict[str, Any]]:
    defaults = config.get("defaults", {})
    return [{**defaults, **model} for model in config.get("models", [])]


def load_judge_models(path: Path, only_judge: list[str]) -> dict[str, dict[str, Any]]:
    config = json.loads(path.read_text(encoding="utf-8"))
    wanted = set(only_judge)
    out: dict[str, dict[str, Any]] = {}
    for model in merge_defaults(config):
        if not model.get("enabled"):
            continue
        keys = {str(model.get("id")), str(model.get("name")), str(model.get("model"))}
        if wanted and not keys.intersection(wanted):
            continue
        for key in keys:
            if key:
                out[key] = model
    return out


def build_messages(judge_model: dict[str, Any], prompt: str) -> list[dict[str, Any]]:
    messages: list[dict[str, Any]] = []
    if judge_model.get("includeSystemPrompt", True):
        messages.append(
            {
                "role": "system",
                "content": "你是模型盲评裁判。必须只输出合法 JSON，不要输出 Markdown 或额外解释。",
            }
        )
    messages.append({"role": "user", "content": [{"type": "text", "text": prompt}]})
    return messages


def completion_token_param(judge_model: dict[str, Any]) -> str:
    explicit = str(judge_model.get("maxTokensParam") or "").strip()
    if explicit in {"max_tokens", "max_completion_tokens"}:
        return explicit
    model_name = str(judge_model.get("model") or judge_model.get("id") or "")
    if model_name.startswith(("gpt-5", "o1", "o3", "o4")):
        return "max_completion_tokens"
    return "max_tokens"


def call_judge_model(
    judge_model: dict[str, Any],
    prompt: str,
    max_tokens: int | None,
    timeout_seconds: float | None,
) -> Any:
    try:
        from openai import OpenAI  # type: ignore[reportMissingImports]
    except ImportError as exc:
        raise RuntimeError("Missing Python package: openai. Install it before running the judge command.") from exc

    api_key = os.getenv(str(judge_model["apiKeyEnv"]))
    if not api_key:
        raise RuntimeError(f"Missing env var: {judge_model['apiKeyEnv']}")
    effective_model = dict(judge_model)
    if max_tokens is not None:
        effective_model["maxTokens"] = max_tokens
    if timeout_seconds is not None:
        effective_model["timeoutSeconds"] = timeout_seconds
    client = OpenAI(
        base_url=str(effective_model["baseUrl"]),
        api_key=api_key,
        timeout=float(effective_model.get("timeoutSeconds", 60 * 20)),
    )
    request_body: dict[str, Any] = {
        "model": effective_model["model"],
        "messages": build_messages(effective_model, prompt),
    }
    if effective_model.get("temperature") is not None:
        request_body["temperature"] = effective_model["temperature"]
    if "maxTokens" in effective_model:
        request_body[completion_token_param(effective_model)] = effective_model["maxTokens"]
    try:
        return client.chat.completions.create(**request_body)
    except Exception as exc:
        message = str(exc)
        if "max_tokens" in request_body and "max_completion_tokens" in message:
            request_body["max_completion_tokens"] = request_body.pop("max_tokens")
            return client.chat.completions.create(**request_body)
        if "temperature" in request_body and ("temperature` is deprecated" in message or "temperature" in message and "unsupported" in message.lower()):
            request_body.pop("temperature", None)
            return client.chat.completions.create(**request_body)
        raise


def extract_content(payload: Any) -> str:
    choices = getattr(payload, "choices", None) or []
    if not choices:
        return ""
    message = getattr(choices[0], "message", None)
    content = getattr(message, "content", None)
    return content if isinstance(content, str) else ""


def parse_judge_json(content: str) -> dict[str, Any]:
    text = content.strip()
    if text.startswith("```"):
        text = re.sub(r"^```(?:json)?\s*", "", text)
        text = re.sub(r"\s*```$", "", text)
    try:
        data = json.loads(text)
    except json.JSONDecodeError:
        match = re.search(r"\{.*\}", text, flags=re.S)
        if not match:
            raise
        data = json.loads(match.group(0))
    outcome = str(data.get("outcome") or "").strip()
    if outcome not in ALLOWED_OUTCOMES:
        raise ValueError(f"invalid judge outcome: {outcome}")
    confidence = data.get("confidence")
    try:
        confidence_value = float(confidence)
    except (TypeError, ValueError):
        confidence_value = 0.0
    confidence_value = max(0.0, min(1.0, confidence_value))
    return {
        "outcome": outcome,
        "confidence": confidence_value,
    }


def sample_tasks(args: argparse.Namespace) -> None:
    domains_root = Path(args.domains_root)
    rng = random.Random(args.seed)
    rows: list[dict[str, Any]] = []
    counter = 1
    domains = args.domain or DEFAULT_DOMAINS

    for domain in domains:
        queries = load_queries(domains_root, domain)
        responses = load_responses(domains_root, domain)
        model_ids = model_ids_for_domain(responses)
        for judge_model in model_ids:
            candidates: list[tuple[str, str, str]] = []
            for query_id, per_model in responses.items():
                pair_models = [model_id for model_id in per_model if args.include_self or model_id != judge_model]
                for left_model, right_model in itertools.combinations(sorted(pair_models), 2):
                    candidates.append((query_id, left_model, right_model))
            rng.shuffle(candidates)
            selected = candidates[: args.pairs_per_judge]
            for query_id, left_model, right_model in selected:
                if rng.random() < 0.5:
                    left_model, right_model = right_model, left_model
                query = queries.get(query_id)
                left = responses[query_id][left_model]
                right = responses[query_id][right_model]
                row_id = f"mpe-{args.seed}-{counter:06d}"
                counter += 1
                rows.append(
                    {
                        "id": row_id,
                        "schemaVersion": 1,
                        "runId": args.run_id,
                        "domain": domain,
                        "queryId": query_id,
                        "query": query.query if query else "",
                        "judgeModel": judge_model,
                        "leftModel": left_model,
                        "rightModel": right_model,
                        "left": {
                            "modelId": left_model,
                            "responseFile": left.response_file,
                            "answer": left.answer,
                        },
                        "right": {
                            "modelId": right_model,
                            "responseFile": right.response_file,
                            "answer": right.answer,
                        },
                        "judgePrompt": build_judge_prompt(
                            query.query if query else "",
                            left_model,
                            left.answer,
                            right_model,
                            right.answer,
                        ),
                        "seed": args.seed,
                    }
                )
    write_jsonl(Path(args.output), rows)
    print(f"wrote {len(rows)} task(s) to {args.output}")


def completed_ids(path: Path) -> set[str]:
    done: set[str] = set()
    for row in read_jsonl(path):
        row_id = str(row.get("id") or "").strip()
        if row_id:
            done.add(row_id)
    return done


def task_lookup(path: Path) -> dict[str, dict[str, Any]]:
    tasks: dict[str, dict[str, Any]] = {}
    for task in read_jsonl(path):
        task_id = str(task.get("id") or "").strip()
        if task_id:
            tasks[task_id] = task
    return tasks


def run_judge_task(
    task: dict[str, Any],
    judge_models: dict[str, dict[str, Any]],
    max_tokens: int | None,
    timeout_seconds: float | None,
) -> tuple[dict[str, Any] | None, dict[str, Any] | None]:
    started_at = time.monotonic()
    task_id = str(task.get("id") or "").strip()
    judge_model_id = str(task.get("judgeModel") or "").strip()
    judge_model = judge_models.get(judge_model_id)
    if not judge_model:
        return None, {**task, "error": f"judge model not enabled or not found: {judge_model_id}"}
    try:
        raw = call_judge_model(judge_model, str(task.get("judgePrompt") or ""), max_tokens, timeout_seconds)
        content = extract_content(raw)
        parsed = parse_judge_json(content)
        outcome = parsed["outcome"]
        record = {
            "id": task_id,
            "outcome": outcome,
            "score": {"left": 1, "right": -1, "both_good": 0.35, "both_bad": 0}[outcome],
            "confidence": parsed["confidence"],
        }
        return record, None
    except Exception as exc:
        return None, {
            **task,
            "error": str(exc),
            "elapsedSeconds": round(time.monotonic() - started_at, 3),
            "failedAt": dt.datetime.now(dt.timezone.utc).isoformat(),
        }


def judge_tasks(args: argparse.Namespace) -> None:
    tasks = read_jsonl(Path(args.input))
    if args.limit > 0:
        tasks = tasks[: args.limit]
    output_path = Path(args.output)
    errors_path = Path(args.errors_output)
    done = set() if args.no_skip else completed_ids(output_path)
    if done:
        tasks = [task for task in tasks if str(task.get("id") or "").strip() not in done]
    model_path = Path(args.models)
    judge_models = load_judge_models(model_path, args.only_judge)
    if not judge_models:
        raise SystemExit("No enabled judge models. Check --models and enabled=true.")
    if args.only_judge:
        wanted = set(args.only_judge)
        tasks = [task for task in tasks if str(task.get("judgeModel") or "") in wanted]
    total = len(tasks)
    print(f"Loaded {total} task(s), {len(done)} skipped existing success record(s).", flush=True)
    lock = threading.Lock()
    ok_count = 0
    error_count = 0
    with concurrent.futures.ThreadPoolExecutor(max_workers=max(1, args.workers)) as executor:
        futures = [
            executor.submit(run_judge_task, task, judge_models, args.max_tokens, args.timeout_seconds)
            for task in tasks
        ]
        for index, future in enumerate(concurrent.futures.as_completed(futures), start=1):
            record, error = future.result()
            if record is not None:
                append_jsonl(output_path, record, lock)
                ok_count += 1
                print(
                    f"[{index}/{total}] OK task={record['id']} outcome={record['outcome']}",
                    flush=True,
                )
            elif error is not None:
                append_jsonl(errors_path, error, lock)
                error_count += 1
                print(
                    f"[{index}/{total}] ERROR judge={error.get('judgeModel')} task={error.get('id')} error={error.get('error')}",
                    flush=True,
                )
    print(f"Done. success={ok_count}, errors={error_count}, output={output_path}, errors={errors_path}", flush=True)


def validate_votes(args: argparse.Namespace) -> None:
    domains_root = Path(args.domains_root)
    domains = args.domain or DEFAULT_DOMAINS
    query_ids: dict[str, set[str]] = {}
    model_ids: dict[str, set[str]] = {}
    for domain in domains:
        query_ids[domain] = set(load_queries(domains_root, domain))
        model_ids[domain] = set(model_ids_for_domain(load_responses(domains_root, domain)))

    errors: list[str] = []
    rows = read_jsonl(Path(args.input))
    tasks = task_lookup(Path(args.tasks))
    seen_ids: set[str] = set()
    for index, row in enumerate(rows, start=1):
        row_id = str(row.get("id") or "").strip()
        task = tasks.get(row_id, {})
        domain = str(row.get("domain") or task.get("domain") or "").strip()
        query_id = str(row.get("queryId") or row.get("questionId") or task.get("queryId") or task.get("questionId") or "").strip()
        judge_model = str(row.get("judgeModel") or task.get("judgeModel") or "").strip()
        left_model = str(row.get("leftModel") or row.get("left", {}).get("modelId") or task.get("leftModel") or task.get("left", {}).get("modelId") or "").strip()
        right_model = str(row.get("rightModel") or row.get("right", {}).get("modelId") or task.get("rightModel") or task.get("right", {}).get("modelId") or "").strip()
        outcome = str(row.get("outcome") or "").strip()
        prefix = f"line {index}"

        if not row_id:
            errors.append(f"{prefix}: missing id")
        elif row_id in seen_ids:
            errors.append(f"{prefix}: duplicate id {row_id}")
        elif row_id not in tasks and not all([domain, query_id, judge_model, left_model, right_model]):
            errors.append(f"{prefix}: unknown task id {row_id}; pass --tasks or include full metadata")
        seen_ids.add(row_id)
        if "score" not in row:
            errors.append(f"{prefix}: missing score")
        if "confidence" not in row:
            errors.append(f"{prefix}: missing confidence")
        if domain not in query_ids:
            errors.append(f"{prefix}: unknown domain {domain}")
            continue
        if query_id not in query_ids[domain]:
            errors.append(f"{prefix}: unknown queryId {query_id} for {domain}")
        for label, model_id in [("judgeModel", judge_model), ("leftModel", left_model), ("rightModel", right_model)]:
            if model_id not in model_ids[domain]:
                errors.append(f"{prefix}: unknown {label} {model_id} for {domain}")
        if left_model == right_model:
            errors.append(f"{prefix}: leftModel and rightModel must differ")
        if outcome not in ALLOWED_OUTCOMES:
            errors.append(f"{prefix}: invalid outcome {outcome}")

    if errors:
        for err in errors:
            print(err)
        raise SystemExit(1)
    print(f"validated {len(rows)} peer vote(s)")


def main() -> None:
    parser = argparse.ArgumentParser(description="Sample and validate model peer evaluation files.")
    subparsers = parser.add_subparsers(dest="command", required=True)

    sample = subparsers.add_parser("sample", help="sample model pair judge tasks")
    sample.add_argument("--domains-root", default="eval-workspace/domains")
    sample.add_argument("--output", default="eval-workspace/model-peer-evals/peer_vote_tasks.jsonl")
    sample.add_argument("--domain", action="append", choices=DEFAULT_DOMAINS)
    sample.add_argument("--pairs-per-judge", type=int, default=20)
    sample.add_argument("--seed", type=int, default=20260512)
    sample.add_argument("--run-id", default="peer-eval-20260512")
    sample.add_argument("--include-self", action="store_true", help="allow the judge model to appear as left/right")
    sample.set_defaults(func=sample_tasks)

    judge = subparsers.add_parser("judge", help="call remote judge models for sampled tasks")
    judge.add_argument("--input", default="eval-workspace/model-peer-evals/peer_vote_tasks.jsonl")
    judge.add_argument("--output", default="eval-workspace/model-peer-evals/peer_votes.jsonl")
    judge.add_argument("--errors-output", default="eval-workspace/model-peer-evals/peer_vote_errors.jsonl")
    judge.add_argument("--models", default="eval-workspace/model-peer-evals/models.local.json")
    judge.add_argument("--workers", type=int, default=7)
    judge.add_argument("--max-tokens", type=int, default=1024)
    judge.add_argument("--timeout-seconds", type=float, default=None)
    judge.add_argument("--limit", type=int, default=0)
    judge.add_argument("--only-judge", action="append", default=[])
    judge.add_argument("--no-skip", action="store_true")
    judge.set_defaults(func=judge_tasks)

    validate = subparsers.add_parser("validate", help="validate completed peer_votes.jsonl")
    validate.add_argument("--domains-root", default="eval-workspace/domains")
    validate.add_argument("--input", default="eval-workspace/model-peer-evals/peer_votes.jsonl")
    validate.add_argument("--tasks", default="eval-workspace/model-peer-evals/peer_vote_tasks.jsonl")
    validate.add_argument("--domain", action="append", choices=DEFAULT_DOMAINS)
    validate.set_defaults(func=validate_votes)

    args = parser.parse_args()
    args.func(args)


if __name__ == "__main__":
    main()
