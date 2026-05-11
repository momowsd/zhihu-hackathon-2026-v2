#!/usr/bin/env python3
"""Call Zhihu OpenAI-compatible model gateway for this evaluation domain."""

from __future__ import annotations

import argparse
import datetime as dt
import json
import os
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


def build_messages(
    model: dict[str, Any],
    system_prompt: str,
    user_prompt: str,
) -> list[dict[str, Any]]:
    messages: list[dict[str, Any]] = []
    if model.get("includeSystemPrompt", True):
        messages.append({"role": "system", "content": system_prompt})

    # 保持与本地已跑通脚本一致：user content 使用 text part 数组。
    messages.append(
        {
            "role": "user",
            "content": [
                {"type": "text", "text": user_prompt},
            ],
        }
    )
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
    request_body: dict[str, Any] = {
        "model": model["model"],
        "messages": messages,
    }
    if "temperature" in model:
        request_body["temperature"] = model["temperature"]
    if "maxTokens" in model:
        request_body["max_tokens"] = model["maxTokens"]

    return client.chat.completions.create(**request_body)


def merge_defaults(config: dict[str, Any]) -> list[dict[str, Any]]:
    defaults = config.get("defaults", {})
    models: list[dict[str, Any]] = []
    for model in config.get("models", []):
        merged = {**defaults, **model}
        models.append(merged)
    return models


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--models", default="models.local.json", help="model config JSON path")
    args = parser.parse_args()

    model_path = Path(args.models)
    if not model_path.is_absolute():
        model_path = SCRIPT_DIR / model_path

    config = json.loads(model_path.read_text(encoding="utf-8"))
    models = [m for m in merge_defaults(config) if m.get("enabled")]
    if not models:
        print("No enabled models. Set enabled=true in your model config.")
        return 0

    system_prompt = read_text(DOMAIN_DIR / "prompts" / "system.md")
    user_template = read_text(DOMAIN_DIR / "prompts" / "user_template.md")
    queries = read_jsonl(DOMAIN_DIR / "raw_queries.jsonl")
    RESPONSES_DIR.mkdir(parents=True, exist_ok=True)

    now = dt.datetime.now(dt.timezone.utc).isoformat()
    for model in models:
        output_path = RESPONSES_DIR / f"{model['id']}.jsonl"
        with output_path.open("a", encoding="utf-8") as output:
            for query in queries:
                user_prompt = user_template.replace("{{query}}", query["query"])
                messages = build_messages(model, system_prompt, user_prompt)
                record: dict[str, Any] = {
                    "queryId": query["id"],
                    "modelId": model["id"],
                    "model": model["model"],
                    "calledAt": now,
                    "provider": model.get("provider"),
                    "baseUrl": model.get("baseUrl"),
                    "request": {"messages": messages},
                    "answer": "",
                    "rawResponse": None,
                    "error": None,
                }
                try:
                    raw = call_model(model, messages)
                    record["rawResponse"] = to_jsonable(raw)
                    record["answer"] = extract_answer(raw)
                except Exception as exc:  # Keep batch output auditable even when one call fails.
                    record["error"] = str(exc)
                output.write(json.dumps(record, ensure_ascii=False) + "\n")
        print(f"Wrote {output_path}")

    return 0


if __name__ == "__main__":
    raise SystemExit(main())
