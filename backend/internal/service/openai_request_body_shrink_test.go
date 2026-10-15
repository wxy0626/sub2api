package service

import (
	"encoding/json"
	"strings"
	"testing"
)

const testInlineImagePrefix = "data:image/png;base64,"

func testInlineImage(fill string, repeat int) string {
	return testInlineImagePrefix + strings.Repeat(fill, repeat)
}

// buildShrinkTestBody 构造 input 为 [message(content), function_call_output(output)] 的最小会话体。
func buildShrinkTestBody(oldest, middle, newest string, extraText string) []byte {
	body := map[string]any{
		"model": "flash",
		"input": []any{
			map[string]any{
				"type": "message",
				"role": "user",
				"content": []any{
					map[string]any{"type": "input_text", "text": extraText},
					map[string]any{"type": "input_image", "image_url": oldest},
				},
			},
			map[string]any{
				"type": "function_call_output",
				"call_id": "call_middle",
				"output": []any{
					map[string]any{"type": "input_image", "image_url": middle},
				},
			},
			map[string]any{
				"type": "message",
				"role": "user",
				"content": []any{
					map[string]any{"type": "input_image", "image_url": newest},
				},
			},
		},
	}
	raw, err := marshalOpenAIUpstreamJSON(body)
	if err != nil {
		panic(err)
	}
	return raw
}

func imageURLAtInput(t *testing.T, body []byte, itemIndex int, path ...int) string {
	t.Helper()
	var decoded map[string]any
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	items := decoded["input"].([]any)
	item := items[itemIndex].(map[string]any)
	containerKey := "content"
	if _, ok := item["content"]; !ok {
		containerKey = "output"
	}
	container := item[containerKey].([]any)
	part := container[path[0]].(map[string]any)
	return part["image_url"].(string)
}

func TestShrinkOpenAIRequestBodyInlineImages_UnderLimitUntouched(t *testing.T) {
	body := buildShrinkTestBody(
		testInlineImage("A", 512),
		testInlineImage("B", 512),
		testInlineImage("C", 512),
		"keep me",
	)
	limit := int64(len(body) + 1024)

	got, replaced, changed, err := shrinkOpenAIRequestBodyInlineImages(body, limit)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if changed || replaced != 0 {
		t.Fatalf("expected untouched body, changed=%v replaced=%d", changed, replaced)
	}
	if string(got) != string(body) {
		t.Fatalf("expected identical body bytes")
	}
}

func TestShrinkOpenAIRequestBodyInlineImages_DisabledByZeroLimit(t *testing.T) {
	body := buildShrinkTestBody(testInlineImage("A", 4096), testInlineImage("B", 4096), testInlineImage("C", 4096), "text")

	got, replaced, changed, err := shrinkOpenAIRequestBodyInlineImages(body, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if changed || replaced != 0 || string(got) != string(body) {
		t.Fatalf("limit<=0 must disable trimming")
	}
}

func TestShrinkOpenAIRequestBodyInlineImages_TrimsOldestFirst(t *testing.T) {
	oldest := testInlineImage("A", 8192)
	middle := testInlineImage("B", 8192)
	newest := testInlineImage("C", 8192)
	body := buildShrinkTestBody(oldest, middle, newest, "任务目标保持不变")
	// 需要削减的字节数超过单张图片的净节省量，因此最旧的两张必须被替换。
	limit := int64(len(body) - len(oldest) - 100)

	got, replaced, changed, err := shrinkOpenAIRequestBodyInlineImages(body, limit)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !changed {
		t.Fatalf("expected body to be trimmed")
	}
	if replaced != 2 {
		t.Fatalf("expected 2 oldest images replaced, got %d", replaced)
	}
	if int64(len(got)) > limit {
		t.Fatalf("expected trimmed body <= limit, got %d > %d", len(got), limit)
	}
	var decoded map[string]any
	if err := json.Unmarshal(got, &decoded); err != nil {
		t.Fatalf("trimmed body must stay valid JSON: %v", err)
	}
	if !strings.Contains(string(got), "任务目标保持不变") {
		t.Fatalf("non-image content must be preserved")
	}
	if gotURL := imageURLAtInput(t, got, 0, 1); gotURL != openAIShrunkImagePlaceholderURL {
		t.Fatalf("oldest image should be replaced, got prefix %q", gotURL[:min(24, len(gotURL))])
	}
	if gotURL := imageURLAtInput(t, got, 1, 0); gotURL != openAIShrunkImagePlaceholderURL {
		t.Fatalf("function_call_output image should be replaced too")
	}
	if gotURL := imageURLAtInput(t, got, 2, 0); gotURL != newest {
		t.Fatalf("newest image must be kept intact")
	}
}

func TestShrinkOpenAIRequestBodyInlineImages_KeepsRemoteAndFileReferences(t *testing.T) {
	body, err := marshalOpenAIUpstreamJSON(map[string]any{
		"model": "flash",
		"input": []any{
			map[string]any{
				"type": "message",
				"role": "user",
				"content": []any{
					map[string]any{"type": "input_image", "image_url": "https://example.com/a.png"},
					map[string]any{"type": "input_image", "file_id": "file-123"},
					map[string]any{"type": "input_text", "text": strings.Repeat("x", 512)},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	limit := int64(len(body) - 64)

	got, replaced, changed, err := shrinkOpenAIRequestBodyInlineImages(body, limit)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if changed || replaced != 0 {
		t.Fatalf("non data-URL references must not be trimmed")
	}
	if string(got) != string(body) {
		t.Fatalf("body should be untouched")
	}
}

func TestShrinkOpenAIRequestBodyInlineImages_NoImagesIsBestEffortNoop(t *testing.T) {
	body := []byte(`{"model":"flash","input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"` +
		strings.Repeat("y", 2048) + `"}]}]}`)

	got, replaced, changed, err := shrinkOpenAIRequestBodyInlineImages(body, 128)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if changed || replaced != 0 {
		t.Fatalf("image-free body has nothing to trim")
	}
	if string(got) != string(body) {
		t.Fatalf("body should be returned unchanged")
	}
}

func TestOpenAIUpstreamRequestBodyLimit_ReadsConfig(t *testing.T) {
	var nilService *OpenAIGatewayService
	if limit := nilService.openAIUpstreamRequestBodyLimit(); limit != 0 {
		t.Fatalf("nil service must disable trimming, got %d", limit)
	}
}

// Responses→Chat 降级后的真实出站体是 Chat Completions 形状（messages[].content），
// 必须与 Responses 形状一样被裁剪，否则 DeepSeek 这类上游仍然会 413。
func TestShrinkOpenAIRequestBodyInlineImages_ChatCompletionsShape(t *testing.T) {
	oldest := testInlineImage("A", 8192)
	newest := testInlineImage("C", 8192)
	body, err := marshalOpenAIUpstreamJSON(map[string]any{
		"model": "deepseek-v4.1-flash",
		"messages": []any{
			map[string]any{
				"role": "user",
				"content": []any{
					map[string]any{"type": "text", "text": "先看这张"},
					map[string]any{"type": "image_url", "image_url": map[string]any{"url": oldest}},
				},
			},
			map[string]any{
				"role": "user",
				"content": []any{
					map[string]any{"type": "image_url", "image_url": map[string]any{"url": newest}},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	// 需要削减的字节数小于单张图片的净节省量，因此只替换最旧的一张。
	limit := int64(len(body) - 8000)

	got, replaced, changed, err := shrinkOpenAIRequestBodyInlineImages(body, limit)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !changed || replaced != 1 {
		t.Fatalf("expected 1 chat image replaced, changed=%v replaced=%d", changed, replaced)
	}
	if int64(len(got)) > limit {
		t.Fatalf("expected trimmed body <= limit, got %d > %d", len(got), limit)
	}
	var decoded map[string]any
	if err := json.Unmarshal(got, &decoded); err != nil {
		t.Fatalf("trimmed chat body must stay valid JSON: %v", err)
	}
	messages := decoded["messages"].([]any)
	firstContent := messages[0].(map[string]any)["content"].([]any)
	if gotURL := firstContent[1].(map[string]any)["image_url"].(map[string]any)["url"].(string); gotURL != openAIShrunkImagePlaceholderURL {
		t.Fatalf("oldest chat image should be replaced")
	}
	secondContent := messages[1].(map[string]any)["content"].([]any)
	if gotURL := secondContent[0].(map[string]any)["image_url"].(map[string]any)["url"].(string); gotURL != newest {
		t.Fatalf("newest chat image must be kept intact")
	}
}
