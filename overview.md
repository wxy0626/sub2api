# GLM Chat 422 修复概览

## 已完成

继续修复 `bizdecipher.com:5555` 的 GLM 调用问题。确认后台 Chat 测试与真实 Chat 调用虽然都使用 `/v1/chat/completions`，但真实流式网关会为了获取计费 usage 自动注入 `stream_options.include_usage=true`，后台测试不会注入该字段。GLM 类 OpenAI 兼容上游可能因此返回 HTTP 422。

已调整 `backend/internal/service/openai_gateway_chat_completions_raw.go`：对 `glm-*` 模型不再自动注入 `stream_options.include_usage`，其余模型保持原有 usage 注入行为。

## 验证

以下测试均通过：

- GLM reasoning_effort 归一化测试
- GLM 流式请求跳过 `stream_options` 测试
- 非 GLM 模型仍注入 `stream_options.include_usage` 测试
- Raw Chat Completions 回归测试

## 相关说明

此前已修复 `/v1/responses` 探测对 GLM 422 拒识的误判。当前这次修复针对默认 Chat 与强制 Chat 均仍报 422 的第二个差异点：真实流式请求额外携带 `stream_options`。

如果部署后仍返回 422，下一步需要读取该次请求的上游原始错误正文和实际 wire body，继续确认是否是 `tools`、`reasoning_effort` 或其他客户端参数不兼容。