package service

import (
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"go.uber.org/zap"
)

// 长会话出站请求体超限时的历史内联图片裁剪。
//
// 背景：Codex 客户端每轮都会把整段会话（含历史截图 data URL）重新发送，
// 长任务很容易累积到数十 MB；部分上游对请求体有硬上限（例如 DeepSeek
// 在 50MB 量级直接返回 413 Payload Too Large），客户端只会看到
// "Request payload is too large"，整条线程从此无法继续。
//
// 这里在请求进入传输层前，按配置上限从最旧的历史截图开始替换成占位图：
// 保留 part 结构与类型不变（仍是合法的 input_image），只把体积换成几个字节，
// 因此对上游完全兼容；最近的截图始终优先保留。

// openAIShrunkImagePlaceholderURL 是 1×1 透明 PNG，用于顶替被裁剪的历史截图。
const openAIShrunkImagePlaceholderURL = "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNkYPhfDwAChwGA60e6kgAAAABJRU5ErkJggg=="

// shrinkPassLimit 限制重复裁剪次数：单轮估算与实际序列化体积可能有极小误差，
// 允许少量收敛重试，避免异常输入导致长时间循环。
const shrinkPassLimit = 4

// openAIUpstreamRequestBodyLimit 返回出站请求体上限（字节），<=0 表示不裁剪。
func (s *OpenAIGatewayService) openAIUpstreamRequestBodyLimit() int64 {
	if s == nil || s.cfg == nil {
		return 0
	}
	limit := s.cfg.Gateway.UpstreamRequestBodyLimit
	if limit <= 0 {
		return 0
	}
	return limit
}

// shrinkOversizeOpenAIRequestBody 在出站前裁剪超限的历史内联图片。
// 未超限、未配置上限或没有可裁剪图片时原样返回。
func (s *OpenAIGatewayService) shrinkOversizeOpenAIRequestBody(account *Account, body []byte) []byte {
	limit := s.openAIUpstreamRequestBodyLimit()
	if limit <= 0 || int64(len(body)) <= limit {
		return body
	}
	shrunk, replaced, changed, err := shrinkOpenAIRequestBodyInlineImages(body, limit)
	if err != nil {
		logger.L().Warn("openai.upstream_request_body_shrink_failed",
			zap.Int64("account_id", openAIAccountIDOrZero(account)),
			zap.Error(err),
		)
		return body
	}
	if !changed {
		return body
	}
	logger.L().Warn("openai.upstream_request_body_shrunk",
		zap.Int64("account_id", openAIAccountIDOrZero(account)),
		zap.String("platform", openAIAccountPlatform(account)),
		zap.Int64("limit_bytes", limit),
		zap.Int("original_bytes", len(body)),
		zap.Int("shrunk_bytes", len(shrunk)),
		zap.Int("replaced_images", replaced),
	)
	return shrunk
}

func openAIAccountIDOrZero(account *Account) int64 {
	if account == nil {
		return 0
	}
	return account.ID
}

func openAIAccountPlatform(account *Account) string {
	if account == nil {
		return ""
	}
	return account.Platform
}

// shrinkOpenAIRequestBodyInlineImages 把超限请求体中最旧的内联图片替换成占位图，
// 直到预估体积落回上限以内；返回新 body、被替换的图片数量以及是否发生变化。
func shrinkOpenAIRequestBodyInlineImages(body []byte, limit int64) ([]byte, int, bool, error) {
	if limit <= 0 || int64(len(body)) <= limit {
		return body, 0, false, nil
	}
	current := body
	replacedTotal := 0
	for pass := 0; pass < shrinkPassLimit; pass++ {
		if int64(len(current)) <= limit {
			break
		}
		var reqBody map[string]any
		if err := decodeOpenAIJSONUseNumber(current, &reqBody); err != nil {
			if replacedTotal > 0 {
				return current, replacedTotal, true, nil
			}
			return body, 0, false, err
		}
		parts := collectOpenAIInlineImageParts(reqBody)
		if len(parts) == 0 {
			break
		}
		need := int64(len(current)) - limit
		saved := int64(0)
		replaced := 0
		for _, part := range parts {
			if saved >= need {
				break
			}
			url, ok := openAIInlineImagePartURL(part)
			if !ok {
				continue
			}
			delta := int64(len(url)) - int64(len(openAIShrunkImagePlaceholderURL))
			if delta <= 0 {
				continue
			}
			setOpenAIInlineImagePartURL(part, openAIShrunkImagePlaceholderURL)
			saved += delta
			replaced++
		}
		if replaced == 0 {
			break
		}
		rebuilt, err := marshalOpenAIUpstreamJSON(reqBody)
		if err != nil {
			if replacedTotal > 0 {
				return current, replacedTotal, true, nil
			}
			return body, 0, false, err
		}
		current = rebuilt
		replacedTotal += replaced
	}
	return current, replacedTotal, replacedTotal > 0, nil
}

// collectOpenAIInlineImageParts 按会话时间顺序收集请求体内的内联图片 part（最旧在前）。
// 同时覆盖两种出站形状：
//   - Responses 形状：input[] 内的 message.content / function_call_output.output；
//   - Chat Completions 形状（Responses→Chat 降级后的真实出站体）：messages[].content。
func collectOpenAIInlineImageParts(reqBody map[string]any) []map[string]any {
	parts := collectOpenAIInlineImagePartsFromItems(reqBody["input"])
	parts = append(parts, collectOpenAIInlineImagePartsFromItems(reqBody["messages"])...)
	return parts
}

func collectOpenAIInlineImagePartsFromItems(value any) []map[string]any {
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	var parts []map[string]any
	for _, rawItem := range items {
		item, ok := rawItem.(map[string]any)
		if !ok {
			continue
		}
		// content 承载 message 的图片；output 承载 function_call_output 的图片。
		for _, key := range []string{"content", "output"} {
			container, ok := item[key].([]any)
			if !ok {
				continue
			}
			for _, rawPart := range container {
				part, ok := rawPart.(map[string]any)
				if !ok {
					continue
				}
				if _, ok := openAIInlineImagePartURL(part); ok {
					parts = append(parts, part)
				}
			}
		}
	}
	return parts
}

// openAIInlineImagePartURL 返回 part 内联图片的 data URL。
// 只处理 data:image 开头且非占位图的 URL，file_id / http(s) 引用不参与裁剪。
func openAIInlineImagePartURL(part map[string]any) (string, bool) {
	switch strings.TrimSpace(stringValue(part["type"])) {
	case "input_image", "image_url", "image":
	default:
		return "", false
	}
	for _, key := range []string{"image_url", "url"} {
		switch value := part[key].(type) {
		case string:
			trimmed := strings.TrimSpace(value)
			if strings.HasPrefix(trimmed, "data:image/") && trimmed != openAIShrunkImagePlaceholderURL {
				return trimmed, true
			}
		case map[string]any:
			nested := strings.TrimSpace(stringValue(value["url"]))
			if strings.HasPrefix(nested, "data:image/") && nested != openAIShrunkImagePlaceholderURL {
				return nested, true
			}
		}
	}
	return "", false
}

// setOpenAIInlineImagePartURL 只覆盖原有字段，保持上游已接受的 part 形状不变。
func setOpenAIInlineImagePartURL(part map[string]any, url string) {
	for _, key := range []string{"image_url", "url"} {
		switch value := part[key].(type) {
		case string:
			part[key] = url
		case map[string]any:
			value["url"] = url
		}
	}
}
