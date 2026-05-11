# Responses

这里保存模型对 `raw_queries.jsonl` 的返回结果。

建议每个模型一个 JSONL 文件：

```text
responses/
  deepseek-v4-pro-baidubce.jsonl
```

每行结构：

```json
{
  "queryId": "rq-0001",
  "modelId": "deepseek-v4-pro-baidubce",
  "model": "deepseek-v4-pro-baidubce",
  "calledAt": "2026-05-11T09:30:00+00:00",
  "request": {
    "messages": []
  },
  "answer": "模型归一化回答文本",
  "rawResponse": {},
  "error": null
}
```

如果调用失败，`answer` 可以为空，`error` 保留失败原因。
