# Evaluation Workspace

这里存放每个评估领域的原始用户 query、prompt、模型调用脚本配置，以及各模型返回结果。

## 目录约定

```text
eval-workspace/
  domains/
    <domain-slug>/
      domain.json              # 领域元信息：中文名、说明、标签
      raw_queries.jsonl        # 原始用户 query，一行一个 JSON
      prompts/
        system.md              # system prompt
        user_template.md       # user prompt 模板，使用 {{query}} 注入 query
      model_calls/
        models.example.json    # 模型调用配置示例，不放真实密钥
        call_openai_compatible.py
      responses/
        README.md              # 返回结果 JSONL 结构说明
        .gitkeep
```

## 数据格式

`raw_queries.jsonl` 每行一个对象：

```json
{"id":"rq-0001","query":"如果一只猫每小时吃 2 条鱼，为什么它三小时后还说自己饿？","tags":["logic","trick"],"source":"manual"}
```

模型输出建议按模型拆文件：

```text
responses/
  gpt-4o-mini.jsonl
  deepseek-chat.jsonl
```

每行结果保留原始请求、原始响应、归一化回答和错误信息，便于之后导入盲评题库或复现实验。

## 新增领域

复制 `domains/ruozhi-eval/`，将目录 slug、`domain.json`、`raw_queries.jsonl` 和 prompts 调整为新领域即可。

## 当前领域

- `domains/ruozhi-eval/`：弱智评估
- `domains/novel-writing-eval/`：小说创作评估
- `domains/humor-eval/`：搞笑评估
- `domains/text-to-image-eval/`：文生图评估
