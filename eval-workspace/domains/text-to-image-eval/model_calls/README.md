# Model Calls

这里放本领域的模型调用配置与脚本。

## 快速使用

1. 复制配置：

```bash
cp models.example.json models.local.json
```

2. 将 `models.local.json` 中需要调用的模型设为 `"enabled": true`，并在本地环境变量里注入 API Key。

3. 执行：

```bash
python3 call_openai_compatible.py --models models.local.json
```

脚本会读取同级领域目录下的：

- `raw_queries.jsonl`
- `prompts/system.md`
- `prompts/user_template.md`

并将模型回答写入：

- `../responses/<model-id>.jsonl`

不要提交包含真实 API Key 的 `models.local.json`。
