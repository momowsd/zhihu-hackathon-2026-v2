#!/usr/bin/env python3
"""Call Zhihu OpenAI-compatible model gateway for this evaluation domain."""

from __future__ import annotations

import argparse
import concurrent.futures
import datetime as dt
import json
import os
import threading
import time
from pathlib import Path
from typing import Any

from openai import OpenAI


SCRIPT_DIR = Path(__file__).resolve().parent
DOMAIN_DIR = SCRIPT_DIR.parent
RESPONSES_DIR = DOMAIN_DIR / "responses"


def read_text(path: Path) -> str:
    return path.read_text(encoding="utf-8").strip()


def read_jsonl(path: Path) -> list[dict[str, Any]]:
    rows: list[dict[str, Any]] = []
    for line_no, line in enumerate(path.read_text(encoding="utf-8").splitlines(), start=1):
        line = line.strip()
        if not line:
            continue
        try:
            rows.append(json.loads(line))
        except json.JSONDecodeError as exc:
            raise ValueError(f"{path}:{line_no} is not valid JSONL") from exc
    return rows


def to_jsonable(value: Any) -> Any:
    if hasattr(value, "model_dump"):
        return value.model_dump(mode="json")
    if hasattr(value, "dict"):
        return value.dict()
    return value


def extract_answer(payload: Any) -> str:
    choices = getattr(payload, "choices", None) or []
    if not choices:
        return ""
    message = getattr(choices[0], "message", None)
    content = getattr(message, "content", None)
    return content if isinstance(content, str) else ""


def build_messages(model: dict[str, Any], system_prompt: str, user_prompt: str) -> list[dict[str, Any]]:
    messages: list[dict[str, Any]] = []
    if model.get("includeSystemPrompt", True):
        messages.append({"role": "system", "content": system_prompt})
    messages.append({"role": "user", "content": [{"type": "text", "text": user_prompt}]})
    return messages


def call_model(model: dict[str, Any], messages: list[dict[str, Any]]) -> Any:
    api_key = os.getenv(str(model["apiKeyEnv"]))
    if not api_key:
        raise RuntimeError(f"Missing env var: {model['apiKeyEnv']}")

    client = OpenAI(
        base_url=str(model["baseUrl"]),
        api_key=api_key,
        timeout=float(model.get("timeoutSeconds", 60 * 20)),
    )
    request_body: dict[str, Any] = {"model": model["model"], "messages": messages}
    if "temperature" in model:
        request_body["temperature"] = model["temperature"]
    if "maxTokens" in model:
        request_body["max_tokens"] = model["maxTokens"]
    return client.chat.completions.create(**request_body)


def load_completed_query_ids(output_path: Path, retry_errors: bool) -> set[str]:
    completed: set[str] = set()
    if not output_path.exists():
        return completed
    for line_no, line in enumerate(output_path.read_text(encoding="utf-8").splitlines(), start=1):
        line = line.strip()
        if not line:
            continue
        try:
            row = json.loads(line)
        except json.JSONDecodeError:
            print(f"[warn] Skip invalid JSONL line: {output_path}:{line_no}", flush=True)
            continue
        query_id = row.get("queryId")
        if not isinstance(query_id, str) or not query_id:
            continue
        if retry_errors and row.get("error"):
            continue
        completed.add(query_id)
    return completed


def write_jsonl(path: Path, record: dict[str, Any]) -> None:
    with path.open("a", encoding="utf-8") as output:
        output.write(json.dumps(record, ensure_ascii=False) + "\n")
        output.flush()


def run_one_call(
    *,
    model: dict[str, Any],
    query: dict[str, Any],
    system_prompt: str,
    user_template: str,
    max_tokens: int | None,
    timeout_seconds: float | None,
) -> dict[str, Any]:
    started_at = time.monotonic()
    user_prompt = user_template.replace("{{query}}", query["query"])
    messages = build_messages(model, system_prompt, user_prompt)
    effective_model = dict(model)
    if max_tokens is not None:
        effective_model["maxTokens"] = max_tokens
    if timeout_seconds is not None:
        effective_model["timeoutSeconds"] = timeout_seconds
    record: dict[str, Any] = {
        "queryId": query["id"],
        "modelId": model["id"],
        "model": model["model"],
        "calledAt": dt.datetime.now(dt.timezone.utc).isoformat(),
        "provider": model.get("provider"),
        "baseUrl": model.get("baseUrl"),
        "request": {"messages": messages},
        "answer": "",
        "rawResponse": None,
        "error": None,
        "elapsedSeconds": None,
    }
    try:
        raw = call_model(effective_model, messages)
        record["rawResponse"] = to_jsonable(raw)
        record["answer"] = extract_answer(raw)
    except Exception as exc:
        record["error"] = str(exc)
    finally:
        record["elapsedSeconds"] = round(time.monotonic() - started_at, 3)
    return record


def merge_defaults(config: dict[str, Any]) -> list[dict[str, Any]]:
    defaults = config.get("defaults", {})
    return [{**defaults, **model} for model in config.get("models", [])]


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--models", default="models.local.json", help="model config JSON path")
    parser.add_argument("--workers", type=int, default=7, help="parallel model workers; queries run sequentially per model")
    parser.add_argument("--max-tokens", type=int, default=None, help="override max output tokens for all models")
    parser.add_argument("--timeout-seconds", type=float, default=None, help="override request timeout seconds")
    parser.add_argument("--limit", type=int, default=0, help="only run first N queries after filtering")
    parser.add_argument("--only-model", action="append", default=[], help="only run model id/name; repeatable")
    parser.add_argument("--only-query-id", action="append", default=[], help="only run query id; repeatable")
    parser.add_argument("--no-skip", action="store_true", help="do not skip existing successful records")
    parser.add_argument("--no-retry-errors", action="store_true", help="skip existing records even when they contain error")
    args = parser.parse_args()

    model_path = Path(args.models)
    if not model_path.is_absolute():
        model_path = SCRIPT_DIR / model_path

    config = json.loads(model_path.read_text(encoding="utf-8"))
    models = [m for m in merge_defaults(config) if m.get("enabled")]
    if args.only_model:
        wanted_models = set(args.only_model)
        models = [
            m
            for m in models
            if str(m.get("id")) in wanted_models
            or str(m.get("name")) in wanted_models
            or str(m.get("model")) in wanted_models
        ]
    if not models:
        print("No enabled models. Set enabled=true in your model config.")
        return 0

    system_prompt = read_text(DOMAIN_DIR / "prompts" / "system.md")
    user_template = read_text(DOMAIN_DIR / "prompts" / "user_template.md")
    queries = read_jsonl(DOMAIN_DIR / "raw_queries.jsonl")
    if args.only_query_id:
        wanted_query_ids = set(args.only_query_id)
        queries = [q for q in queries if str(q.get("id")) in wanted_query_ids]
    if args.limit > 0:
        queries = queries[: args.limit]
    RESPONSES_DIR.mkdir(parents=True, exist_ok=True)

    model_jobs: list[tuple[dict[str, Any], list[dict[str, Any]], Path]] = []
    skipped = 0
    for model in models:
        output_path = RESPONSES_DIR / f"{model['id']}.jsonl"
        completed = set()
        if not args.no_skip:
            completed = load_completed_query_ids(output_path, retry_errors=not args.no_retry_errors)
        pending_queries: list[dict[str, Any]] = []
        for query in queries:
            if query["id"] in completed:
                skipped += 1
                continue
            pending_queries.append(query)
        if pending_queries:
            model_jobs.append((model, pending_queries, output_path))

    total = sum(len(model_queries) for _, model_queries, _ in model_jobs)
    print(
        f"Loaded {len(models)} enabled model(s), {len(queries)} querie(s). "
        f"Queued {total} call(s), skipped {skipped} existing record(s).",
        flush=True,
    )
    if total == 0:
        return 0

    workers = max(1, min(args.workers, len(model_jobs)))
    print(
        f"Running with {workers} model worker(s). "
        "Each model processes its query list sequentially.",
        flush=True,
    )
    done = 0
    progress_lock = threading.Lock()
    started_at = time.monotonic()

    def run_model_queries(model: dict[str, Any], model_queries: list[dict[str, Any]], output_path: Path) -> tuple[str, int]:
        nonlocal done
        print(f"[model-start] {model['id']} queued={len(model_queries)} output={output_path}", flush=True)
        wrote = 0
        for query_index, query in enumerate(model_queries, start=1):
            try:
                record = run_one_call(
                    model=model,
                    query=query,
                    system_prompt=system_prompt,
                    user_template=user_template,
                    max_tokens=args.max_tokens,
                    timeout_seconds=args.timeout_seconds,
                )
            except Exception as exc:
                record = {
                    "queryId": query["id"],
                    "modelId": model["id"],
                    "model": model["model"],
                    "calledAt": dt.datetime.now(dt.timezone.utc).isoformat(),
                    "provider": model.get("provider"),
                    "baseUrl": model.get("baseUrl"),
                    "request": None,
                    "answer": "",
                    "rawResponse": None,
                    "error": f"worker failed: {exc}",
                    "elapsedSeconds": None,
                }
            write_jsonl(output_path, record)
            wrote += 1
            with progress_lock:
                done += 1
                global_done = done
            status = "ERR" if record.get("error") else "OK"
            elapsed = record.get("elapsedSeconds")
            elapsed_label = f"{elapsed:.1f}s" if isinstance(elapsed, (int, float)) else "n/a"
            print(
                f"[{global_done}/{total}] {status} {model['id']} "
                f"query={query['id']} model_progress={query_index}/{len(model_queries)} "
                f"answer_chars={len(record.get('answer') or '')} elapsed={elapsed_label}",
                flush=True,
            )
        return model["id"], wrote

    with concurrent.futures.ThreadPoolExecutor(max_workers=workers) as executor:
        future_to_model = {
            executor.submit(
                run_model_queries,
                model,
                model_queries,
                output_path,
            ): model
            for model, model_queries, output_path in model_jobs
        }
        for future in concurrent.futures.as_completed(future_to_model):
            model = future_to_model[future]
            try:
                model_id, wrote = future.result()
            except Exception as exc:
                print(f"[model-error] {model['id']} failed: {exc}", flush=True)
                continue
            print(f"[model-done] {model_id} wrote={wrote}", flush=True)

    print(f"Done in {time.monotonic() - started_at:.1f}s.", flush=True)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
